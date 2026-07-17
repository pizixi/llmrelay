# LLM Relay 后端结构

后端采用 Go 项目常见的 `internal` 组织方式。`backend` 包只暴露稳定的 `Run` 启动入口，具体实现放在 `internal` 下，避免业务包被项目外部直接依赖，也让协议、基础设施和 HTTP 接入层的职责边界保持清晰。

```text
backend/
  run.go                              # 对外稳定启动入口
  internal/
    app/
      app.go                          # 依赖组装、启动参数和进程生命周期

    domain/
      types.go                        # 配置及跨模块共享领域类型
    protocol/
      chat/types.go                   # OpenAI Chat 数据结构
      anthropic/types.go              # Anthropic Messages 数据结构
      responses/types.go              # OpenAI Responses 数据结构

    config/
      config.go                       # 配置读取、规范化、保存与运行时状态
      store.go                        # 配置状态访问
      compat.go                       # 迁移后的内部依赖适配
    auth/                             # 管理会话和对外 API 鉴权
    routing/resolver.go               # 模型别名及上游路由解析
    catalog/                          # 模型目录、缓存与上游模型同步
    stats/                            # 请求量和 Token 统计
    netproxy/                         # HTTP 客户端、SOCKS5 出口与轮换
    sse/writer.go                     # SSE 写出基础设施

    bridge/
      decision.go                     # 客户端协议到上游协议的路径决策
      warning.go                      # 桥接损失告警
      convert/                        # 三种协议的请求和响应转换
      stream/                         # 三种协议间的 SSE 状态机

    upstream/                         # 上游请求、密钥轮询、重试与错误映射
    websearch/                        # 搜索 Provider、目录选择和工具执行循环
    gateway/                          # Chat、Anthropic、Responses 业务 Handler

    httpapi/
      server.go                       # HTTP 路由总装
      public/                         # 对外模型 API 和健康检查
      admin/                          # 管理 API
      middleware/                     # HTTP 鉴权中间件

  tests/
    gateway/                          # Gateway 集成、协议矩阵和回归测试
```

## 依赖方向

```text
main -> backend.Run -> internal/app -> internal/httpapi
                                      -> internal/gateway
                                      -> internal/config/auth/catalog/stats

httpapi -> gateway -> routing/bridge/upstream/websearch
bridge  -> protocol/domain
基础设施包 -> domain
```

- `domain` 和 `protocol` 位于依赖底层，不依赖 Handler。
- `bridge` 只负责协议决策、转换和流状态机，不注册 HTTP 路由。
- `gateway` 编排一次模型请求的完整业务流程。
- `httpapi` 只负责路由、管理接口和中间件装配。
- `app` 是唯一的进程组装入口，不承载协议业务。
- `tests/gateway` 是独立测试包；测试专用适配层只在 `go test` 时编译，不扩大生产包的公开 API。

## 协议桥接矩阵

| 客户端 \ 上游 | OpenAI Chat | Anthropic Messages | OpenAI Responses |
|---|---|---|---|
| OpenAI Chat | passthrough | anthropic→chat | responses→chat |
| Anthropic Messages | chat→anthropic | passthrough | responses→anthropic |
| OpenAI Responses | chat→responses | anthropic→responses | passthrough |

Anthropic 与 Responses 之间继续使用直接成对转换；只有网关自有 hosted-search 工具循环等场景才使用 Chat 中间表示。迁移仅改变代码归属和依赖组织，不改变转换规则、SSE 事件顺序、Header、重试、搜索回退或统计语义。

## 新增代码约定

1. 新协议类型放入 `internal/protocol/<protocol>`。
2. 请求和响应映射放入 `internal/bridge/convert`，流式状态机放入 `internal/bridge/stream`。
3. 对外请求流程放入 `internal/gateway`，路由注册放入 `internal/httpapi`。
4. 上游网络行为放入 `internal/upstream`，SOCKS5 和客户端复用放入 `internal/netproxy`。
5. 跨模块共享类型优先放入 `internal/domain`；仅单包使用的类型保留在所属包，避免继续扩大公共面。
6. 注释沿用中文，协议规范中的正式字段名和事件名保持原文。

## 验证

```bash
go test ./...
go vet ./...
```

Gateway 测试覆盖三种客户端协议与三种上游协议的 3×3 矩阵，以及非流式转换、SSE、工具调用、模型路由、鉴权、重试、代理和 Token 统计。
