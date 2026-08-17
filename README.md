# inject-proxy

Go monorepo with two components.

## htmlinject — HTML 注入反向代理

转发请求到上游,响应时把注入片段插到 `</body>` 前。基于 HTML5 tokenizer
(而非字符串匹配),正确处理 gzip/zlib 压缩与 charset。

```sh
go run ./cmd/inject-proxy --upstream http://127.0.0.1:8000 --port 8080 --inject ./inject-ball.html
```

注入核心抽在 [`rewrite`](rewrite) 包(幂等、tokenizer 级重写),`htmlinject`
与 `matrix --inject-html`(重写静态站点 index.html)共用同一实现。

## matrix — MiniMax matrix MCP 复刻

基于 [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)
对真实 matrix MCP server 的高保真复刻:22 个工具,name/description/inputSchema
与线上 100% 一致(嵌入 `matrix/schema.json` 逐字节核对)。

```sh
# mock 模式(离线确定性响应),HTTP 监听 :8080($PORT 优先)
go run ./cmd/matrix --mode mock

# proxy 模式(转发到真实 matrix server)
MATRIX_URL=http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message \
MATRIX_SK=sk_... go run ./cmd/matrix

# 本地 deploy + 站点托管:同一进程按 Host 路由,部署出的站点
# 以 http://<site-id>.localhost/ 访问,MCP 仍在监听地址上
go run ./cmd/matrix --mode mock --data-dir ./data --workspace-dir /workspace --domain localhost
```

详见 [`matrix/README.md`](matrix/README.md)。

## 测试

```sh
go test ./...
```
