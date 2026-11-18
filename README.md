# LLM Relay

LLM Relay 是一个轻量级 LLM 网关。它对外提供 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages 三种接口，并可把请求路由到使用上述任一协议的上游服务。

## 主要功能

- OpenAI Chat、OpenAI Responses、Anthropic Messages 三种协议互转
- 普通 JSON 响应和 SSE 流式响应
- 多上游、模型别名、高权重优先路由、自动故障切换及模型冷却
- compatible（尽力兼容并告警）和 strict（拒绝已知有损转换）桥接模式
- 文本、图片、推理内容和工具调用转换
- 多 API Key 轮询、鉴权失败换 Key 及 429、502、503、504 自动重试
- 每个上游独立选择 SOCKS5 或直连出口
- 请求明细与输入、缓存命中、输出 Token 多维统计
- 对外 API Key 鉴权和独立的管理后台密码认证
- 上游模型同步及 `/health` 健康检查

## API

| 客户端协议 | 接口 |
|---|---|
| [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create) | `POST /v1/chat/completions` |
| [OpenAI Responses](https://developers.openai.com/api/reference/resources/responses/methods/create) | `POST /v1/responses` |
| [Anthropic Messages](https://docs.anthropic.com/en/api/messages) | `POST /v1/messages` |
| 模型列表 | `GET /v1/models` |
| 健康检查 | `GET /health` |

三个生成接口均可路由到 `openai`、`openai-responses` 或 `anthropic` 类型的上游。

## 模型路由

模型路由遵循以下规则：

- 默认只对客户端开放“模型映射”：`GET /v1/models` 只返回别名，请求也必须使用别名。
- 模型名是已配置的别名。别名可以配置一个或多个“目标上游 + 实际模型”，默认优先选择权重最高的启用目标，同权重目标通过轮询分担请求。
- 在管理后台模型映射区打开“允许直接使用上游模型”后，网关会额外接受 `custom_models` 中声明的模型名，并按上游顺序轮询负载均衡；别名仍然优先。
- 请求的模型不在允许范围内时，请求会在访问上游前返回 404。

直接模型（仅在开启“允许直接使用上游模型”时可用）会保留客户端显式发送的 `thinking`、`reasoning` 或 `reasoning_effort`。命中模型别名时，只有启用该别名的“思维链”开关才会向上游发送这些推理参数；关闭后该策略会一致应用于三种客户端协议的透传、桥接和 Web Search 回退路径。

显式模型别名的优先级高于按模型名自动选择。核心解析逻辑位于 `backend/internal/routing/resolver.go` 和 `backend/internal/config/`。

多目标映射默认采用 `priority` 策略。下面的配置会优先使用权重为 3 的主上游，故障时先尝试同权重的其他启用目标，再按权重从高到低切换。同权重目标独立轮询，包括故障后进入的备用权重层。切换目标时会重新按实际协议构建请求，适用于三种生成接口及流式请求。权重必须是正整数，`enabled` 为 `false` 的目标会保留权重。全部目标均被禁用时，该映射会保留为停用状态并在管理页提醒。旧版单目标 `target_model/upstream` 配置仍可读取，并会在管理页保存时转换为 `targets`。

```json
{
  "model_alias": {
    "balanced-chat": {
      "targets": [
        { "upstream": "primary", "target_model": "gpt-5.1", "weight": 3, "enabled": true },
        { "upstream": "backup", "target_model": "claude-sonnet-4-5", "weight": 1, "enabled": true }
      ],
      "with_reasoning": true
    }
  }
}
```

管理后台“模型映射”中，点击“添加别名”旁边的“设置”打开“路由与故障切换”弹窗，可以调整选择策略、故障切换开关、每次请求最多尝试目标数、冷却开关、连续故障阈值及冷却时间，点击“保存设置”生效。选择 `weighted_round_robin` 后恢复按别名独立计数的加权周期分配，上述 `3:1` 配置每四次首选分配中主上游占三次，备用上游占一次。发生故障后仍会继续尝试剩余目标，每个目标在一次请求中最多进入一次。单个目标内部继续使用其 `max_retries` 密钥轮换和退避预算。

默认路由配置如下，保存到 SQLite 后可热更新。

```json
{
  "model_routing": {
    "strategy": "priority",
    "disable_failover": false,
    "max_attempts": 0,
    "disable_cooldown": false,
    "failure_threshold": 3,
    "cooldown_seconds": 180
  }
}
```

`max_attempts: 0` 会遍历全部候选目标，正整数限制本次请求尝试的目标总数。默认连续故障阈值为 3 次，冷却时间为 3 分钟（180 秒）。冷却按上游与实际模型隔离，在当前进程内维护。服务重启后清空冷却状态，并根据新请求重新判断上游可用性。不同别名引用同一目标时共用其健康状态。冷却结束后放行一个恢复探测请求，成功后恢复正常路由，失败后重新冷却。修改上游配置后按新配置重新判断健康状态。

| 上游结果 | 请求处理 | 模型冷却 |
|---|---|---|
| 429 | 按原预算换 Key、退避，仍失败则切换下一个目标 | 始终跳过 |
| 401、402、403 鉴权或权限失败 | 在重试预算内尝试其余 Key，仍失败则切换目标 | 达到阈值后冷却 |
| 404、405、410 模型或端点不可用 | 切换目标 | 达到阈值后冷却 |
| 408、5xx、连接失败、响应读取故障 | 按当前目标的重试规则处理，仍失败则切换目标 | 达到阈值后冷却 |
| 409、425 临时拒绝 | 切换目标 | 跳过 |
| 400、422 明确表示模型不存在、停用或不支持 | 切换目标 | 达到阈值后冷却 |
| 400、422 明确表示不支持某项参数或能力 | 切换目标 | 跳过 |
| 请求参数错误、上下文超限、内容策略拒绝 | 返回对应请求错误 | 跳过 |

故障目标的响应会暂存到切换决策完成。全部候选均失败、处于冷却或已达到尝试目标上限时，对外统一返回符合客户端协议的 `503 upstream_unavailable` 和 `Retry-After`。关闭自动故障切换后保留当前目标的原始错误响应。客户端取消会立即终止后续尝试。

流式请求在收到首个上游字节后开始回传，首字节前断流可以切换目标。已经交付内容的流继续遵循下文的同模型重试与重放规则，跨模型切换适用于输出开始前。输出开始后的读取故障会影响该目标后续请求的冷却状态。

管理后台保存上游配置时，会把从已保存 `custom_models` 中删除的模型同步移出对应上游的模型映射；某个别名因此没有任何剩余目标时，该别名也会删除。多上游表格中的“同步模型”会立即打开带搜索的同步弹窗并并行拉取各上游目录：同步成功渠道中上游已不存在的模型及其关联映射会自动删除，同步失败渠道保持原样；上游新增模型按渠道提供全选和取消全选，并在管理员确认后保存。模型同步既支持填写 API 基址，也兼容直接填写完整的 `/v1/models` 地址；未带版本路径的 OpenAI 兼容基址会依次尝试 `/models` 和 `/v1/models`。

上游配置支持“批量添加”：每行填写 `URL | Key`，可选第三列名称 `URL | Key | 名称`。默认逐行识别竖线、逗号、Tab 或空格，也可指定自定义分隔符；接口类型统一选择。导入会预览名称和地址、隐藏完整 Key、标出错误行，并按 URL、Key 和协议跳过重复项。每批最多 500 个，加入列表后点击“保存上游”生效，再同步模型。

上游的“测试模型”提供多轮对话，支持 OpenAI Chat、OpenAI Responses 和 Anthropic Messages。可通过快捷问题检查能力、上下文记忆、推理与代码生成，并在对话中继续追问、切换模型、复制回答或全屏查看。请求自动携带最近 20 轮完整问答，总上下文不超过 32768 字；停止、失败或连接中断的回答不会加入后续上下文。对话仅保留在当前弹窗中，点击“新对话”清空上下文，测试不计入使用记录。模型映射每行的“测试模型”复用同一对话窗口，按别名和权重解析实际上游模型，并在弹窗中显示本次命中的上游与实际模型。

使用记录和上游配置的筛选支持单项清除、再次点击已选选项取消、清空全部，以及可移除的已选条件标签；搜索框可直接清空。主页“模型 × 上游”和“按日期 + 上游 + 模型”统计分别展示请求模型（下游）与实际模型（上游）：同一别名路由到不同实际模型时独立计数，合计保持一致。

使用记录的“模型路由”列会显示本次请求实际使用的上游密钥序号，例如 `KEY 2/3` 表示该上游配置了 3 个密钥、这次由第 2 个应答（含 429 后轮换到其他密钥重试的结果）；未记录密钥序号的旧数据不显示该标记。

管理后台的“实时日志”页通过 SSE 实时同步服务器控制台输出，最多保留最近 600 行并自动重放，支持暂停/继续自动滚动、清空视图和手动重连。日志页只在打开时建立连接，切走后自动断开。

## 协议桥接

每个请求由 `decideProtocolBridge` 根据客户端协议和上游 `api_type` 选择真实执行路径：

| 客户端 \ 上游 | OpenAI Chat | Anthropic Messages | OpenAI Responses |
|---|---|---|---|
| OpenAI Chat | passthrough | pairwise | pairwise |
| Anthropic Messages | pairwise | passthrough | pairwise |
| OpenAI Responses | pairwise | pairwise | passthrough |

- `passthrough`：客户端与上游协议相同，尽量保留原生字段和事件，只进行模型、鉴权及必要清洗。
- `pairwise`：两个不同协议之间使用直接请求、响应和流转换器，包括 Anthropic Messages 与 OpenAI Responses。
- `pivot`：仅在必须运行网关自有工具循环等场景使用 Chat 中间表示；中间协议无法表达的字段会产生桥接告警。

流式回程统一由 `dispatchClientStream` 分发。非流式请求继续由各入口 Handler 调用现有转换函数处理。

流式上游每次等待数据的空闲时限为 120 秒，覆盖响应头已返回但迟迟没有正文、输出中途停滞和错误正文未结束的情况；持续输出的长回答没有总时长限制。读取超时会取消该次上游请求，并记录 `[上游流读取超时]` 日志，包含上游、模型、密钥槽位和出口。收到协议结束事件后，透传和转换会及时收尾，无需等待上游关闭连接；Chat 的 `finish_reason` 后仍会保留独立的用量分块，直到 `[DONE]` 或流结束。

上游连接以 `unexpected EOF` 断开时，会先处理已接收的最后一帧，包括缺少末尾空行的事件。Chat 转换仅在已经收到有效 `finish_reason` 时正常收尾，保留尾部用量；缺少结束信号、结束事件 JSON 被截断或读取超时仍按失败处理。真实断流错误包含上游、模型、密钥槽位、出口和已接收字节数，便于排查上游服务或代理连接。

HTTP 200 后的正文读取失败也会使用上游的 `max_retries` 预算（默认重试 3 次，未显式设置时至少覆盖全部密钥，设为 `0` 禁用），与连接失败和状态码重试共用次数，并支持退避、密钥轮换和客户端取消。同协议透传仅在尚未交付字节时重试，保持原始流不变。Chat→Anthropic/Responses 支持中途断流后重发原请求：先逐字节核对已交付的文字、推理、工具名和工具参数，忽略变化的请求/工具 ID 以及分块边界，匹配后只发送新增内容，核对期间会随重放数据定期发送心跳。重新生成的内容不一致时明确报错；收到结束原因后不再重新生成。为限制内存，核对记录最多保留 8 MiB，包含无法核对的扩展字段时也不执行中途重放。重试可能产生额外上游计费，用量按客户端请求记录一次，采用完成尝试返回的用量。

流式日志附带请求标识；断流错误同时记录总耗时 `elapsed`、距最后数据的间隔 `idle`、HTTP 协议、预期正文长度、自动解压状态和本地取消原因 `context`，可据此区分总时限、输出停滞、正文截断与本地取消。

响应会附带以下可观测头：

- `X-Llm2api-Bridge-Path`：`passthrough`、`pairwise` 或 `pivot`
- `X-Llm2api-Bridge-Client`
- `X-Llm2api-Bridge-Upstream`
- `X-Llm2api-Bridge-Mode`：`passthrough`、`compatible` 或 `strict`
- `X-Llm2api-Warning`、`X-Llm2api-Warning-Count`：检测到兼容性损失时返回

### 桥接模式

- `compatible`：尽可能完成请求；对已知的有损字段给出告警。
- `strict`：在调用上游前拒绝已知的有损转换。

模式可以在上游配置中设置，也可通过请求头 `X-Llm2api-Bridge-Mode` 指定 `strict`。请求头严格模式优先。

JSON Schema 输出会在 Chat 的 `response_format`、Responses 的 `text.format` 和 Anthropic 的 `output_config.format` 之间转换。Anthropic 的 `output_config.effort` 单独控制推理，不会触发结构化输出降级告警；关闭别名推理时仍保留输出格式。无 Schema 的 `json_object`、显式 `strict:false` 等无法等价转入 Anthropic 的设置仍会告警或被 strict 模式拒绝。具体模型的 Schema 支持范围由上游验证，详见[协议审查记录](backend/PROTOCOL_REVIEW.md)。

协议转换不要求额外维护供应商能力列表。网关按客户端协议与上游 `api_type` 自动选择同协议透传或直接转换；Responses 映射到 Chat 时会先发送当前 Chat 标准字段。如果兼容上游以 400/422 拒绝 `verbosity` 或 Prompt Cache 提示字段，compatible 模式会自动移除这些可选字段并重试一次，同时返回信息级桥接告警。

### Hosted Web Search 自动协商

上游不再配置 `hosted_web_search`。Responses 的 `web_search` 和 Anthropic 的 `web_search_*` 版本工具都由网关自动选择执行路径：

- 首次请求优先使用上游原生 hosted search；Chat 上游自动映射为 `web_search_options`。
- 上游以 400/422 明确返回“不支持 web search”时，compatible 模式会切换到网关搜索执行器。对于 Claude Code WebSearch 这类通过 `tool_choice` 明确要求必须搜索的请求，HTTP 200 终态若没有 `web_search_call`、搜索结果、引用或搜索计数，也会被识别为上游忽略了搜索并自动回退。
- 不支持判断按上游地址、API 类型和模型缓存 30 分钟；后续请求直接使用网关搜索，不再重复无效的原生尝试。流式强制搜索仅在能力未知时缓冲首个响应做终态判断，确认原生支持后恢复直接流式转发。
- 401、403、429、5xx 等鉴权、限流和服务错误不会触发降级，避免掩盖真实故障。
- strict 模式不执行原生搜索到网关搜索的语义降级。
- 未启用全局搜索执行器时，原生搜索失败会原样返回，不会静默生成一份没有搜索依据的答案。

旧 SQLite 配置记录中的 `hosted_web_search` 字段会在一次性关系化迁移时忽略。

网关回退是独立的受控工具循环：模型先调用内部搜索 function，网关执行搜索、把结果作为 tool message 注入，然后再次调用模型。客户端自己的 function/custom 工具不会由网关执行。流式请求会在搜索和模型闭环完成后输出合法的 Responses SSE 事件；原生上游流式请求仍保持透传。

Anthropic `/v1/messages` 的自动回退会返回 `server_tool_use`、`web_search_tool_result` 和最终文本块，并在 `usage.server_tool_use.web_search_requests` 中返回实际搜索次数。Responses 或 Chat 上游的搜索调用、引用和计数也会自动映射到该字段，避免客户端把已执行的搜索显示为 0 次。Responses 与 Anthropic 的搜索调用/结果均可在下一轮请求中重建为成对的 Chat 工具历史。

全局配置支持 SearXNG、DuckDuckGo Lite 和 Tavily：

```json
{
  "web_search": {
    "enabled": true,
    "provider": "searxng",
    "searxng_mode": "auto",
    "fallback_provider": "duckduckgo",
    "max_results": 6,
    "timeout_seconds": 10,
    "max_tool_rounds": 2,
    "max_result_bytes": 65536
  }
}
```

SearXNG 支持三种实例模式：

- `auto`：从 [searx.space](https://searx.space) 官方实时目录读取公共实例，按 HTTP/TLS 等级、搜索成功率、搜索延迟、可用率和隐私指标评分；失败实例冷却 10 分钟，并记住最近成功实例。目录缓存 15 分钟，可在管理页手动刷新。`base_url` 可选填为目录不可用时的备用实例。默认的 `fallback_provider: "duckduckgo"` 会给 SearXNG 一个 1 秒优先窗口，随后使用 DuckDuckGo Lite，避免公共实例的 429 或超时耗尽整个搜索时限；设为 `none` 可关闭此兜底。
- `selected`：在管理页查看官方目录中当前 HTTP 可用的公共实例，并按延迟、月可用率、等级及“自动候选/仅手动”标记选择；结果保存到 `base_url`，运行时固定使用该实例。
- `custom`：不读取公共目录，直接使用手工填写的 `base_url`，适合自建 SearXNG。

目录默认使用 `https://searx.space/data/instances.json`；高级部署可通过 `searxng_directory_url` 覆盖。公共实例可能限制自动化或 JSON 输出，自动模式会在实际搜索失败时故障切换。

自动搜索失败日志仅记录尝试数、429 数、超时数和其他错误数，不记录查询字符串或带查询参数的 URL。整体请求超时后立即停止枚举剩余实例。

一次 hosted search 已经失败后，网关会在当前请求中移除该内部工具，避免模型再次消耗完整搜索超时；Fetch 等客户端自有工具仍可继续使用。搜索服务的 429 不会改变模型上游使用的 SOCKS5 出口。

DuckDuckGo 可将 `provider` 改为 `duckduckgo`，默认使用 Crush 同款的 `https://lite.duckduckgo.com/lite/` 公共 HTML 搜索，无需 API Key；也可以仅作为 SearXNG 自动模式的兜底。该实现使用浏览器请求头、解析标题/URL/摘要，并还原 DuckDuckGo 跳转链接。

Tavily 可将 `provider` 改为 `tavily`，`base_url` 可留空使用官方端点，并设置 `api_key`。密钥支持 `env:TAVILY_API_KEY` 或环境变量展开形式。搜索返回内容会被限制数量、总大小和超时时间，并作为不可信外部数据提示给模型；网关不会自动抓取搜索结果中的 URL。

## 代码结构

后端采用 Go 项目常见的 `internal` 分层：顶层 `backend` 只保留稳定启动入口，业务实现按领域职责拆分为多个内部包：

```text
backend/
  run.go                         对外稳定启动入口
  internal/
    app/                         应用组装与进程生命周期
    domain/                      跨模块领域类型
    protocol/{chat,anthropic,responses}/
                                 三种协议的数据结构
    config/                      配置读取、规范化、保存与运行时应用
    auth/                        管理会话和对外 API 鉴权
    routing/                     模型别名与上游路由解析
    catalog/                     模型目录、缓存与同步
    bridge/{convert,stream}/     协议决策、转换与 SSE 状态机
    gateway/                     三种对外协议的业务 Handler
    upstream/                    上游调用、重试、透传和错误映射
    netproxy/                    HTTP 客户端与 SOCKS5 出口管理
    websearch/                   搜索 Provider 与 hosted-search 工具循环
    stats/                       请求明细与多维 Token 统计
    storage/                     纯 Go SQLite 数据库初始化
    sse/                         SSE 输出基础设施
    httpapi/{public,admin,middleware}/
                                 路由注册、管理 API 与中间件
  tests/gateway/                 Gateway 独立集成与协议矩阵测试
```

更详细的后端文件约定见 `backend/STRUCTURE.md`。

前端使用原生 HTML、CSS 和 JavaScript，无需构建工具；资源在编译时嵌入可执行文件。

## 运行

项目使用 `go.mod` 指定的 Go 版本。启动示例：

```bash
go run . -password "管理密码" -api-key "对外 API 密钥"
```

默认地址：

- 管理后台：<http://localhost:8000/>
- API Base URL：<http://localhost:8000/v1>
- 健康检查：<http://localhost:8000/health>

首次启动会在当前目录生成纯 Go 驱动的 SQLite 数据库 `llmrelay.db`，配置和调用明细、统计数据统一保存在其中。配置已经使用关系表保存：上游（含独立 SOCKS5 配置）、上游密钥、模型白名单、模型别名及目标、推理强度映射、API 密钥、SOCKS5 和 Web Search 分别存储，并通过外键、唯一约束和排序字段关联。旧 SQLite 数据库中的 `app_config` JSON 记录会在首次加载时事务迁移到这些表，迁移成功后删除旧表；应用不再读取、写入或导入 `config.json`，也不再提供 `-config` 参数。数据库路径通过 `-db <数据库路径>` 指定。旧版 `stats.json` 仅作为统计数据的独立一次性导入来源。

管理后台的“API 密钥”页可创建、停用、删除和复制多个对外 API Key。旧版本通过 `-api-key`、`LLMGATEWAYGO_API_KEY` 或兼容环境变量 `LLM2API_API_KEY` 设置的密钥，会在首次启动时自动迁移为“默认密钥”；这些参数仍可用于兼容启动脚本。


## 发布

推送符合 `v*` 的 Git tag 后，GitHub Actions 会自动交叉编译并创建 Release：

```bash
# 推荐使用附注 tag 写更新说明
git tag -a v1.0.0 -m "首个正式版本
- 支持多协议路由
- 管理后台与 API 鉴权"

git push origin v1.0.0
```

Release 产物包含：

- Linux / macOS / Windows 的 amd64、arm64 二进制压缩包
- `SHA256SUMS.txt` 校验文件
- 更新说明（tag 附注 + 相对上一版本的提交摘要 + GitHub 自动生成的变更记录）

## 安全建议

默认管理密码是 `123456`，没有创建 API 密钥时对外 API 不启用鉴权。公网部署前应显式设置安全的 `-password`，并在管理后台创建 API 密钥后通过 HTTPS 反向代理访问。

管理后台会话与 `/v1` API Key 是两套独立鉴权：登录管理后台不会自动获得 `/v1` API Key 权限。

## 测试

```bash
go test ./...
go vet ./...
node --test frontend/tests/admin.test.cjs
```

测试覆盖协议 3×3 矩阵、流式转换、工具调用、模型路由、鉴权、重试、代理和 Token 统计等核心路径。
