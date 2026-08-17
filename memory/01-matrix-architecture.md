# matrix 复刻架构 (matrix/)

高保真复刻 MiniMax matrix MCP server,基于 modelcontextprotocol/go-sdk。
目标:22 个工具的 name/description/inputSchema 与线上**逐字节一致**,行为通过 proxy 转发保持真实。

## 分层

```
┌─ cmd/matrix/main.go   唯一入口 (cobra CLI): --mode auto|proxy|mock,
│                       streamable HTTP(默认 :8080 / $PORT),单一监听端口
│
├─ matrix/server.go      go-sdk server 组装: LoadSpecs() 读嵌入 schema.json,
│                        registerAll() 用 mcp.AddTool 注册全部 22 工具
│
├─ matrix/router.go      Router: 按 Host 路由 — 站点命名空间(<site>.<domain> 与
│                        apex 的 "/")走 SiteHandler,其余(监听地址、apex 非根路径、
│                        未知 Host)走 MCP 端点
│
├─ matrix/handler.go     Handler 接口(22 方法,每工具一个)+ Output = []byte(原始 JSON)
│
├─ matrix/types.go       22 个工具的输入结构体(json tag 与 schema 属性一一对应)
│
├─ matrix/proxy.go       ProxyHandler: 把 tools/call 转发到真实 server(高保真行为)
├─ matrix/mock.go        MockHandler: 确定性本地响应(离线/测试用,输出形状仿真实)
├─ matrix/deploy.go      LocalDeploy: 本地发布实现(复制到 data-dir/<随机 site-id>)
├─ matrix/site.go        SiteHandler: 子域静态服务(<site>.<domain>/ → data-dir/<site>),
│                        可选 index.html 重写(复用 rewrite 包,见 04 关键约定 4)
│
└─ matrix/schema.json    真实 server tools/list 的逐字抓取(go:embed 嵌入)
```

deploy 与站点托管在同一个进程:`--data-dir` 开启后 router 按 Host 派发,
MCP 端点与站点共用监听端口(详见 `matrix/README.md` 的 Site hosting 一节)。
无 --data-dir 时纯 MCP(与旧行为一致,deploy 转发/模拟)。

## 关键设计决策

1. **schema 来源**: `schema.json`(线上抓取)而非 Go 反射 → tools/list 与真实 100% 一致。
   若线上工具变更,重新抓取覆盖此文件即可(验证方法见 02)。
2. **Handler 是接缝**: 复刻/真实行为切换只换 Handler 实现,server 层不动。
   `-mode auto` 规则: 有 `--url`+`--token` → proxy,否则 mock。
3. **输出透传**: 真实 server 的返回是 `content[0].text` 里的 JSON 字符串,
   复刻用 `Output = []byte` 原样透传,不做结构解析(结构解析放 client 侧)。
4. **注册机制**: `register[In any]` 泛型 helper + `reg1` 一行绑定
   (Handler 方法指针 → 工具名,编译期保证 22 方法存在)。新增工具 = schema.json + types.go + handler.go + reg1 + proxy/mock 方法。
5. **errors**: 工具执行错误经 `fmt.Errorf("%s: %w", spec.Name, err)` 返回,
   go-sdk 自动打成 tool error(IsError=true)而非 protocol error。

## 入口参数

```sh
go run ./cmd/matrix --mode mock                                  # 离线确定性,HTTP :8080($PORT 优先)
MATRIX_URL=... MATRIX_SK=... go run ./cmd/matrix               # auto→proxy
go run ./cmd/matrix --data-dir ./data --domain localhost        # + 站点托管(Host 路由)
```

详见 `matrix/README.md`。
