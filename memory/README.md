# Project Memory

本目录沉淀本仓库(以及 matrix MCP 复刻任务)的可复用知识,供后续 session 渐进式读取:
先读本索引,按需打开对应主题文件,避免一次性灌入全部上下文。

## 主题索引

| 文件 | 主题 | 何时读 |
|---|---|---|
| [`01-matrix-architecture.md`](01-matrix-architecture.md) | matrix 复刻的分层架构:接口层 / server 层 / 实现层 (proxy+mock),文件职责 | 改 matrix/ 代码、加新工具、理解调用链 |
| [`02-matrix-server-protocol.md`](02-matrix-server-protocol.md) | 真实 matrix server 的协议细节与坑:source 白名单、arguments=null 崩溃、schema 保真验证方法 | 调试代理转发、对齐线上行为、schema 变更 |
| [`03-go-sdk-usage.md`](03-go-sdk-usage.md) | modelcontextprotocol/go-sdk 关键 API 用法:AddTool 泛型、ToolHandlerFor、transport 选择、鉴权 | 写 MCP server/client、改注册逻辑 |
| [`04-repo-layout.md`](04-repo-layout.md) | 本仓库布局:htmlinject(根包) + matrix(子包) + 两个 cmd;迁移历史与命名约定 | 新增文件/命令、理解包边界、提交 |
| [`05-testing-verification.md`](05-testing-verification.md) | 测试策略与现状:覆盖的工具、验证方法(含真实 server 对比)、**已知覆盖缺口** | 跑测试、补测试、验收前检查 |

## 速查

- 真实 matrix server: `http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message`,token 在 `/root/.config/crush/matrix.env` (`MATRIX_SK`),**source 仅白名单 `openclaw`/`hermes`** (详见 02)
- 复刻 server 支持 `-mode auto|proxy|mock` + stdio/HTTP,README 见 `matrix/README.md`
- 测试: `go test ./...`(全绿);22 个工具中仅 3 个有调用级测试覆盖(详见 05)
- 合并后提交号 `638117a`,module `github.com/gearshell/inject-proxy`
