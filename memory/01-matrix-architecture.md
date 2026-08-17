# matrix 复刻架构 (matrix/)

高保真复刻 MiniMax matrix MCP server,基于 modelcontextprotocol/go-sdk。
目标:22 个工具的 name/description/inputSchema 与线上**逐字节一致**,行为通过 proxy 转发保持真实。

## 分层

```
┌─ cmd/matrix/main.go   入口 (cobra CLI): --mode auto|proxy|mock, stdio 或 streamable HTTP
│
├─ matrix/server.go      go-sdk server 组装: LoadSpecs() 读嵌入 schema.json,
│                        registerAll() 用 mcp.AddTool 注册全部 22 工具
│
├─ matrix/handler.go     Handler 接口(22 方法,每工具一个)+ Output = []byte(原始 JSON)
│
├─ matrix/types.go       22 个工具的输入结构体(json tag 与 schema 属性一一对应)
│
├─ matrix/proxy.go       ProxyHandler: 把 tools/call 转发到真实 server(高保真行为)
├─ matrix/mock.go        MockHandler: 确定性本地响应(离线/测试用,输出形状仿真实)
│
└─ matrix/schema.json    真实 server tools/list 的逐字抓取(go:embed 嵌入)
```

## 关键设计决策

1. **schema 来源**: `schema.json`(线上抓取)而非 Go 反射 → tools/list 与真实 100% 一致。
   若线上工具变更,重新抓取覆盖此文件即可(验证方法见 02)。
2. **Handler 是接缝**: 复刻/真实行为切换只换 Handler 实现,server 层不动。
   `-mode auto` 规则: 有 `--url`+`--token` → proxy,否则 mock。
3. **输出透传**: 真实 server 的返回是 `content[0].text` 里的 JSON 字符串,
   复刻用 `Output = []byte` 原样透传,不做结构解析(结构解析放 client 侧)。
4. **注册机制**: `register[In any]` 泛型 helper + 22 个 `regXxx` 函数显式绑定
   类型→Handler 方法。新增工具 = schema.json + types.go + handler.go + regXxx + proxy/mock 方法。
5. **errors**: 工具执行错误经 `fmt.Errorf("%s: %w", spec.Name, err)` 返回,
   go-sdk 自动打成 tool error(IsError=true)而非 protocol error。

## 入口参数

```sh
go run ./cmd/matrix --mode mock                                  # 离线确定性
go run ./cmd/matrix --http :8080 --mode mock                     # streamable HTTP
MATRIX_URL=... MATRIX_SK=... go run ./cmd/matrix               # auto→proxy
```

详见 `matrix/README.md`。
