# 三协议转换审查与优化记录

核查日期：2026-11-04。文档依据为当日实际读取的在线官方文档；开源参照固定到下面列出的修订，避免后续 `main` 更新后无法复核。

## 实现与审查范围

网关以 Chat Completions、Responses、Anthropic Messages 为三个协议顶点。同协议走原生透传，六个跨协议方向使用成对转换器，只有网关自有搜索工具循环等场景经过 Chat 中间表示。本轮保留这个结构：直接转换能够保留更多消息、工具和推理信息。

核查入口为 `bridge/decision.go`、`gateway/handler_*.go`，请求与 JSON 响应位于 `bridge/convert/`，流式响应位于 `bridge/stream/` 及 `upstream.ResponsesStreamToChatHandler`，同时检查了协议类型、SSE 解析器和用量统计。

下面六项已依次实现并验证。协议方向均按数据实际流动方向描述。

## 1. 统一缓存用量语义

**原问题**：Anthropic 的 `input_tokens` 仅包含未缓存输入，Chat 的 `prompt_tokens` 和 Responses 的 `input_tokens` 包含全部输入。直接复制导致少算，反向复制后再保留缓存字段则会重复计算。直接转换使用的 `json.Number` 未被通用用量读取函数识别，还可能得到 0。Chat 响应清洗会删除缓存和推理用量明细；转为 Anthropic 流时，最终输入与缓存用量没有回传。

**官方依据**：[Anthropic 缓存统计](https://platform.claude.com/docs/en/build-with-claude/prompt-caching#tracking-cache-performance) 给出公式：

```text
全部输入 = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
```

[Chat API](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create) 定义 `prompt_tokens_details.cached_tokens`、`cache_write_tokens`，且总量为输入加输出。[Anthropic SSE](https://platform.claude.com/docs/en/build-with-claude/streaming) 明确 `message_delta.usage` 是累积快照。

**开源参照**：LiteLLM 的 `AnthropicConfig.calculate_usage` 将缓存读写加入总输入；CLIProxyAPI 的 `claudeUsageTokens.Merge` 按已出现字段更新快照。

**改动**：通过 `convert/usage.go` 集中换算，在转入 OpenAI 时加上缓存、转入 Anthropic 时扣除缓存；识别 `json.Number`，保留用量明细；流式终态回传完整用量。统计层区分原生 Anthropic 与已转换的 OpenAI 用量，避免二次累加。

**验证**：`TestCacheUsageAcrossProtocolMatrix` 覆盖 3×3×2 共 18 组。使用普通输入 7、缓存读 100、缓存写 10、输出 5 的固定语料，同时断言客户端用量、网关统计与单次请求计数；输入总量应为 117、总 Token 为 122。流中起始与最终快照不同，Chat 用量位于 `finish_reason` 之后。

## 2. 保留函数工具严格度及正确识别 custom

**原问题**：Chat → Anthropic、Anthropic → Chat、Responses → Anthropic 会遗漏部分 `strict`。Anthropic 的显式 `type: "custom"` 函数工具被当作不支持的服务端工具，连带影响 strict 桥接模式的能力判断。Chat/Anthropic 函数工具省略 `strict` 后直接转入 Responses，也可能改变可选参数约束。

**官方依据**：[Anthropic Messages](https://platform.claude.com/docs/en/api/messages/create) 的普通工具类型允许 `"custom"`，其 `strict` 控制工具名和输入的 Schema 校验。[OpenAI 函数调用指南](https://developers.openai.com/api/docs/guides/function-calling#strict-mode) 说明 Chat 默认不严格，而 Responses 在省略 `strict` 时会尝试归一化为严格 Schema，不能归一化时才回退。

**开源参照**：CLIProxyAPI 的 Chat → Claude 工具转换分别处理显式 `strict: true` 与 `strict: false`，没有通过删除布尔字段表达关闭。

**改动**：Anthropic 工具结构增加 `*bool`，六向转换保留显式 true/false；Chat、Anthropic 函数工具进入 Responses 时，源端未设置严格模式则显式发送 false。Anthropic `custom` 走函数工具路径，能力检测不再与 OpenAI 自由格式 `custom` 混淆。

Responses 自身省略 `strict` 时的服务器端 Schema 归一化取决于该 Schema，跨协议转换保留省略状态，不声称已复现该服务器算法。

**验证**：`TestFunctionToolStrictAcrossProtocolMatrix` 覆盖 3×3×3 共 27 组，分别使用省略、false、true，启用 strict 桥接模式，并检查工具名称和 `additionalProperties` 约束。

## 3. 保留多模态顺序与工具结果

**原问题**：Anthropic → Chat 把图片、文件和文本分别收集后重排；Chat → Anthropic 会把结构化工具结果编码成 JSON 字符串；Anthropic → Responses 的数组型错误结果丢失 `is_error` 的错误标记；Responses 的纯 base64 `file_data` 被当作 URL。

**官方依据**：[Messages 内容定义](https://platform.claude.com/docs/en/api/messages/create) 和 [Responses 输入定义](https://developers.openai.com/api/reference/resources/responses/methods/create) 均区分有序内容数组、图片、文件及工具输出，字符串化不能保留这些类型语义。

**开源参照**：CLIProxyAPI 在遍历 Claude 内容块时按顺序追加 `contentItems`，在 Chat → Claude 时单独转换 `tool_result.content`。

**改动**：按原始顺序构造 Chat 内容；工具结果继续转换为 Anthropic 内容块；为数组型错误结果加入文本错误标记；文件转换复用同一数据源解析，区分 URL、data URL 和纯 base64。

**验证**：`convert/content_test.go` 检查“文字→图片→文字→PDF→文字”的往返一致性、结构化工具图片、错误结果和三种文件数据源。

Chat 官方工具消息的内容类型比 Responses/Anthropic 少。此处修复的是现有兼容路径的内容保留；某个 Chat 上游是否接受工具结果中的媒体仍由该上游决定。

## 4. 补齐拒答与内容分段生命周期

**原问题**：Chat `message.refusal`、Responses `response.refusal.delta` 在部分方向被吞掉或当作未知内容报错。Chat 非流式响应内容被限定为字符串，无法接受内容数组。Responses → Anthropic 只按 item ID 记录文本块，多个 `content_index` 会合并；过滤终态也可能错误变成长度截断。

**官方依据**：[Chat API](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create) 定义独立 refusal 字段和 refusal 内容块；[Responses 流事件](https://developers.openai.com/api/reference/resources/responses/streaming-events) 定义 refusal 增量、完成事件及内容分段索引；[Anthropic SSE](https://platform.claude.com/docs/en/build-with-claude/streaming) 要求每个内容块具有独立开始、增量、停止生命周期。

**改动**：Chat/Responses 拒答正文与历史消息不再丢失；进入 Anthropic 时表达为文本并标记拒答终态，进入 Chat 时保留 refusal。Responses → Anthropic 按消息和内容索引分别建块，处理 `.done` 补齐正文且避免重复输出；`content_filter` 终态保留其含义。

**验证**：`refusal_bridge_test.go` 检查拒答、过滤终态、多个内容分段的独立开闭、增量和完成快照的一致性；`content_test.go` 检查拒答历史及数组型 Chat 响应。

## 5. 六个流转换方向统一使用完整 SSE 事件

**原问题**：六个转换器分别使用按行读取，假定每个 `data:` 行就是完整 JSON。合法的多行 `data:` 会全部解析失败；仅 CR 换行也无法正常消费。旧的逐行逻辑与原生透传使用的事件观察器行为不一致。

**改动**：增加 `sse.Reader`，复用原有 Parser 的事件边界规则，统一处理 LF、CRLF、CR、注释、多行 `data:`、事件名和 EOF 尾事件。读取错误继续交给转换器处理。终止事件后及时返回，Chat 仍读取 `finish_reason` 后的最终用量和 `[DONE]`。

转换器的单事件缓存上限为 64 MiB，超限返回明确错误；原生透传继续使用自己的旁路观察与原始字节转发逻辑。不会用观察器的 1 MiB 上限静默丢弃较大的工具参数事件。

**验证**：新增测试先复现六个跨协议方向的失败，再通过 `TestSSEFramingAcrossProtocolMatrix` 检查 3×3×3 共 27 组，包括多行 JSON、注释、省略 `data:` 后空格和无结束换行的 EOF。`sse/reader_test.go` 检查逐字节输入、大事件、缓存超限和原始读取错误。已有终态不等待 EOF、读取超时及独立用量分块的测试继续通过。

## 6. 统一生成上限与手动思考预算

**原问题**：Responses → Anthropic 的手动思考映射可以生成 `budget_tokens >= max_tokens` 的无效请求；Chat 路径会在过小的显式上限下提高客户端上限。两条 Chat 请求转换都优先读取已弃用的 `max_tokens`。当前 Claude Opus 5、Haiku 5.5 也未进入已有的自适应思考匹配。

**官方依据**：[Messages API](https://platform.claude.com/docs/en/api/messages/create) 要求手动思考预算至少 1024 且小于 `max_tokens`；[Thinking 指南](https://platform.claude.com/docs/en/build-with-claude/thinking#configuring-thinking) 列出当前模型对 adaptive/manual 的支持，其中 Claude 5.5 拒绝手动预算；[Chat API](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create) 将 `max_tokens` 标为弃用，使用 `max_completion_tokens`。

**改动**：两条 Anthropic 请求转换复用预算校验；显式输出上限不再提高。无法容纳最小手动预算时，在访问上游前返回带原因的 HTTP 400；未指定上限时沿用按预算生成默认上限的策略。Chat 跨协议转换优先使用 `max_completion_tokens`，缺省时兼容旧字段。补充官方文档已确认的模型家族匹配，不修改原生透传的 API 版本。

**验证**：预算边界覆盖未指定、1024、1025、1536、4096、65536，以及现代字段优先级和 Claude 5.5 自适应映射；网关集成覆盖 Chat/Responses × 流式/非流式四组，断言无效预算返回 400 且上游调用数为 0。

## 开源参照版本

开源实现用于交叉检查工程处理方式，字段定义和约束以官方文档为准；本轮没有复制第三方实现代码。

- CLIProxyAPI，修订 [`01b24f1f95dc9b309765212c52b25b25e005ff01`](https://github.com/router-for-me/CLIProxyAPI/tree/01b24f1f95dc9b309765212c52b25b25e005ff01)。查阅 [Claude ↔ Chat 请求](https://github.com/router-for-me/CLIProxyAPI/blob/01b24f1f95dc9b309765212c52b25b25e005ff01/internal/translator/claude/openai/chat-completions/claude_openai_request.go)、[Claude → Chat 响应及用量](https://github.com/router-for-me/CLIProxyAPI/blob/01b24f1f95dc9b309765212c52b25b25e005ff01/internal/translator/claude/openai/chat-completions/claude_openai_response.go)、[Chat ↔ Claude 请求](https://github.com/router-for-me/CLIProxyAPI/blob/01b24f1f95dc9b309765212c52b25b25e005ff01/internal/translator/openai/claude/openai_claude_request.go)、[Claude ↔ Responses](https://github.com/router-for-me/CLIProxyAPI/tree/01b24f1f95dc9b309765212c52b25b25e005ff01/internal/translator/claude/openai/responses) 和 [Chat ↔ Responses](https://github.com/router-for-me/CLIProxyAPI/tree/01b24f1f95dc9b309765212c52b25b25e005ff01/internal/translator/openai/openai/responses) 的请求/响应转换。
- LiteLLM，修订 [`d5ca88900c4ec558fdf9c7f9084484896221a366`](https://github.com/BerriAI/litellm/tree/d5ca88900c4ec558fdf9c7f9084484896221a366)。查阅 [Anthropic transformation](https://github.com/BerriAI/litellm/blob/d5ca88900c4ec558fdf9c7f9084484896221a366/litellm/llms/anthropic/chat/transformation.py) 的缓存用量处理和 [OpenAI Responses transformation](https://github.com/BerriAI/litellm/blob/d5ca88900c4ec558fdf9c7f9084484896221a366/litellm/llms/openai/responses/transformation.py)。

## 验证方式与适用边界

```bash
go test ./...
go vet ./...
```

本轮新增缓存、工具严格度、SSE 三类矩阵，共 72 组，另有内容往返、拒答、预算边界和事件读取回归。所有网络验证使用本地模拟上游，不依赖真实供应商密钥，因此不能替代各供应商及模型的线上兼容性验收。

协议仍存在无法逐字段等价表达的部分，例如供应商绑定的加密推理、服务器托管工具、自由格式工具语法和服务端 Schema 归一化。本轮只对上文列明的转换行为作出保证；既有 compatible/strict 的降级告警策略继续适用。
