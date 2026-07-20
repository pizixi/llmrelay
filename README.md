# LLM Relay

LLM Relay 是一个轻量级 LLM 网关。它对外提供 OpenAI Chat Completions、OpenAI Responses 和 Anthropic Messages 三种接口，并可把请求路由到使用上述任一协议的上游服务。

## 主要功能

- OpenAI Chat、OpenAI Responses、Anthropic Messages 三种协议互转
- 普通 JSON 响应和 SSE 流式响应
- 多上游、默认上游、模型别名和按模型指定上游
- compatible（尽力兼容并告警）和 strict（拒绝已知有损转换）桥接模式
- 文本、图片、推理内容和工具调用转换
- 多 API Key 轮询及 429、502、503、504 自动重试
- SOCKS5 固定或轮询出口，遇到 429 时可切换出口
- 请求量与 Token 统计
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

- 模型名是已配置的别名；别名可以配置一个或多个“目标上游 + 实际模型”，并用正整数权重分配请求。
- 没有匹配别名时，模型名原样发送到默认上游。
- 默认上游的 `custom_models` 非空时，它是直接模型的允许列表，不在列表中的模型会在访问上游前返回 404。
- 默认上游的 `custom_models` 为空时不限制模型名，由上游判断模型是否存在及当前密钥是否有权访问。

直接模型会保留客户端显式发送的 `thinking`、`reasoning` 或 `reasoning_effort`。命中模型别名时，只有启用该别名的“思维链”开关才会向上游发送这些推理参数；关闭后该策略会一致应用于三种客户端协议的透传、桥接和 Web Search 回退路径。

显式模型别名的优先级高于默认上游模型。核心解析逻辑位于 `backend/internal/routing/resolver.go` 和 `backend/internal/config/`。

多目标映射使用按别名独立计数的加权周期分配。例如下面的 `3:1` 配置每四个请求会将三个请求路由到主上游、一个请求路由到备用上游。权重为 `0` 的目标会保留在配置中但不参与轮询；每个映射至少需要一个权重大于 `0` 的目标。旧版单目标 `target_model/upstream` 配置仍可读取，并会在管理页保存时转换为 `targets`：

```json
{
  "model_alias": {
    "balanced-chat": {
      "targets": [
        { "upstream": "primary", "target_model": "gpt-5.1", "weight": 3 },
        { "upstream": "backup", "target_model": "claude-sonnet-4-5", "weight": 1 }
      ],
      "with_reasoning": true
    }
  }
}
```

管理后台保存上游配置时，会把从已保存 `custom_models` 中删除的模型同步移出对应上游的模型映射；某个别名因此没有任何剩余目标时，该别名也会删除。多上游表格中的“同步模型”会立即打开带搜索的同步弹窗并并行拉取各上游目录：同步成功渠道中上游已不存在的模型及其关联映射会自动删除，同步失败渠道保持原样；上游新增模型按渠道提供全选和取消全选，并在管理员确认后保存。

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

协议转换不要求额外维护供应商能力列表。网关按客户端协议与上游 `api_type` 自动选择同协议透传或直接转换；Responses 映射到 Chat 时会先发送当前 Chat 标准字段。如果兼容上游以 400/422 拒绝 `verbosity` 或 Prompt Cache 提示字段，compatible 模式会自动移除这些可选字段并重试一次，同时返回信息级桥接告警。

### Hosted Web Search 自动协商

上游不再配置 `hosted_web_search`。Responses 的 `web_search` 和 Anthropic 的 `web_search_*` 版本工具都由网关自动选择执行路径：

- 首次请求优先使用上游原生 hosted search；Chat 上游自动映射为 `web_search_options`。
- 上游以 400/422 明确返回“不支持 web search”时，compatible 模式会切换到网关搜索执行器。对于 Claude Code WebSearch 这类通过 `tool_choice` 明确要求必须搜索的请求，HTTP 200 终态若没有 `web_search_call`、搜索结果、引用或搜索计数，也会被识别为上游忽略了搜索并自动回退。
- 不支持判断按上游地址、API 类型和模型缓存 30 分钟；后续请求直接使用网关搜索，不再重复无效的原生尝试。流式强制搜索仅在能力未知时缓冲首个响应做终态判断，确认原生支持后恢复直接流式转发。
- 401、403、429、5xx 等鉴权、限流和服务错误不会触发降级，避免掩盖真实故障。
- strict 模式不执行原生搜索到网关搜索的语义降级。
- 未启用全局搜索执行器时，原生搜索失败会原样返回，不会静默生成一份没有搜索依据的答案。

旧配置文件中的 `hosted_web_search` 字段会在读取时被忽略，保存后自动移除，无需迁移。

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
    stats/                       请求量与 Token 统计
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

首次启动会在当前目录生成 `config.json`。统计数据保存在 `stats.json`。

对外 API Key 也可通过 `LLMGATEWAYGO_API_KEY` 或兼容环境变量 `LLM2API_API_KEY` 设置。


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

默认管理密码是 `123456`，对外 API 默认不启用鉴权。公网部署前应显式设置安全的 `-password` 和 `-api-key`，并通过 HTTPS 反向代理访问。

管理后台会话与 `/v1` API Key 是两套独立鉴权：登录管理后台不会自动获得 `/v1` API Key 权限。

## 测试

```bash
go test ./...
go vet ./...
```

测试覆盖协议 3×3 矩阵、流式转换、工具调用、模型路由、鉴权、重试、代理和 Token 统计等核心路径。
