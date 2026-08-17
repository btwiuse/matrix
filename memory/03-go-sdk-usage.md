# go-sdk 用法要点 (modelcontextprotocol/go-sdk v1.7.0)

官方 SDK: `github.com/modelcontextprotocol/go-sdk`,主包 `mcp`。
本文记录本项目实际用到的 API 与踩坑,避免下次重新考古。

## Server 侧

```go
server := mcp.NewServer(&mcp.Implementation{Name: "x", Version: "0.1.0"}, nil)
```

### 注册工具(泛型版,推荐)

```go
// ToolHandlerFor[In, Out]: func(ctx, req *CallToolRequest, in In) (result *CallToolResult, out Out, err error)
mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, SayHi)

func SayHi(ctx context.Context, _ *mcp.CallToolRequest, in Input) (*mcp.CallToolResult, Output, error) {
    return nil, Output{...}, nil   // 返回 out;或返回自建 result 忽略 out
}
```

- In 类型提供默认 inputSchema(jsonschema struct tag 可加描述),也可在 Tool.InputSchema 显式覆盖
  (本项目用嵌入 schema.json 覆盖,保证 fidelity)。
- In 自动从 req.Params.Arguments 解组 + 按 schema 校验,**校验失败在进 handler 前拒绝**。
- Out 非 `any` 时提供 outputSchema;Out=any 则无输出 schema。
- **err 返回 = tool error**(IsError=true,打进 content),不是 protocol error —— 语义上正好符合"工具调用失败"。
- `Tool.InputSchema` 必须非 nil 且 type=object,否则 panic;可用 `json.RawMessage` 直接传原始 schema。
- 工具名合法性: a-z A-Z 0-9 `_-.` ,≤128 字符。

### 非泛型版本

`server.AddTool(t *Tool, h ToolHandler)` — handler 自己解 args(jsonschema 校验也自己做),
flexibility 高但样板多。泛型版足够时优先泛型。

## Transport

```go
// stdio(默认)
server.Run(ctx, &mcp.StdioTransport{})

// streamable HTTP
h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
    &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
http.ListenAndServe(":8080", h)
```

- `StreamableHTTPOptions.Stateless: true` = 无 session,GET/DELETE 返回 405,适合无状态部署。
- `JSONResponse: true` 让响应走 application/json(默认 text/event-stream)。

## Client 侧(测试用)

```go
client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, nil)

// stdio: 连到子进程
session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command("go", "run", "...")}, nil)

// HTTP: 连到端点
session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
    Endpoint: "http://127.0.0.1:8080",
    DisableStandaloneSSE: true, // 不需要 server 主动推送时省一条 SSE 连接
}, nil)

tools, _ := session.ListTools(ctx, nil)                       // tools.Tools []*Tool
res, _ := session.CallTool(ctx, &mcp.CallToolParams{Name: "x", Arguments: map[string]any{...}})
// res.Content[0] 断言成 *mcp.TextContent 取 .Text;res.IsError 判断工具错误
```

## 坑

1. `mcp.TextContent` 的 MarshalJSON 是**指针接收者** → 存进 `[]mcp.Content` 必须用 `&mcp.TextContent{}`,
   否则编译错 "does not implement mcp.Content"。
2. CommandTransport 里 `exec.CommandContext` 的 ctx 超时会杀掉子进程 — 测试记得给足 timeout。
3. client 默认会开 SSE 长连接;测试纯请求-响应用 `DisableStandaloneSSE: true` 更快更稳。
4. SchemaCache 可用于无状态部署复用反射结果(本项目没用到)。

## 版本

- v1.7.0: 支持 MCP spec 2026-07-28,向下兼容 2025-03-26(真实 matrix server 用的版本)。
- 需要新能力先查 `go list -m -versions github.com/modelcontextprotocol/go-sdk`。
