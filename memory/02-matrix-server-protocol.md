# 真实 matrix server 协议细节与坑

端点: `http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message`(k8s 内网,本机可达 192.168.203.224)
鉴权: URL query 参数 `?sk=<MATRIX_SK>&source=<source>`,不是 Header。
协议: 标准 JSON-RPC 2.0 over HTTP POST(streamable HTTP 的简化形态),单次请求单次响应,无 SSE。

## 协议行为(实测)

- `initialize` 响应: `protocolVersion: "2025-03-26"`, `serverInfo.name: "matrix-mcp-server"` v1.0.0,
  capabilities 仅 `tools.listChanged: false`。
- `tools/call` 响应: `result.content = [{"type":"text","text":"<JSON 字符串>"}]`,
  真正的返回数据是 **text 里的 JSON 字符串**(如 `{"available_voices":[...]}` 或 `{"data":[...]}`)。
- 批量工具(web_search 等)返回形状是 `{"data":[...]}`,不是 `{"results":[...]}` —— 注意别搞混。
- 真实 server 是 Python 实现,`content[0].text` 里的 JSON 由 server 侧 `json.dumps` 生成。

## 坑(必须记住)

1. **`arguments: null` 直接崩溃**: 传 `"arguments": null` → `-32603 'NoneType' object has no attribute 'keys'`。
   无参工具(get_voice_list 等)也必须传 `"arguments": {}`。proxy.go 已处理。
2. **source 白名单**: 仅 `openclaw` 和 `hermes` 允许调用工具,其他 source → `-32603 Source 'xxx' is not allowed`。
   测试/脚本里 source 必须用 `hermes`。
3. **token 位置**: `/root/.config/crush/matrix.env` → `MATRIX_SK`(600 权限)。不要写进任何 git 文件。
4. **HTTP 状态码**: 正常返回 200;错误时 JSON-RPC error 字段,HTTP 层仍是 200,解析时两个都要看。

## deploy 工具实测对比(2026-08-18,本地 replica vs 真实 server)

真实 server 行为(已全部对齐,2026-08-18):

1. **部署产物强制注入 "Created by MiniMax Agent" 悬浮球(~4.6KB)**: 每个**被服务的 .html 文件**(index.html、about.html、sub/page.html 全部)在 `</body>` 前注入;非 html 文件不动。**本地 replica 未对齐此项**(--inject 是自研扩展,用户明确不做,仍只重写 index.html)。
2. **website_id 是 431 前缀 + 12 位随机数**(431840818266354、431900120703430、431897721405594,非单调)→ 本地已改随机(原为 base+计数器)。
3. **website_url 无尾斜杠**: `https://<11位site-id>.space.mcode.cn`(固定域名、https、真实公网)→ 本地已去尾斜杠:`http://<id>.localhost`。
4. **成功结果带 `display_data`**({website_id, website_url})→ 本地经 `matrix/envelope.go` 在 HTTP 层注入(go-sdk 表达不了)。
5. **isError 语义**: 缺 dist → isError=false(正常 result);dist 在 workspace 外 → isError=true → 本地 `ToolError.IsError` 已逐类对齐(缺 dist 用 softToolError)。
6. **"required" 不强制**: 缺 dist_dir 时默认 `/workspace/dist` → 本地 deploy 注册时去掉 required,tools/list 响应由 envelope.go 补回(在线对比 22/22 不受影响)。
7. **project_name 完全不校验**(`../evil` 也能部署成功)→ 本地已删校验。
8. **SPA fallback**: 缺失的**无扩展名**路径(.git/HEAD、/plain-missing)返回注入后的 index.html(200);带扩展名的缺失路径(.js)返回 404;`/definitely-missing/` 目录形式 404。无 index.html 的站点根路径 404(不是目录列表)→ 本地 site.go 已对齐(无列表、显式 /index.html 直接 200 不 301)。
9. **.git/node_modules 均不上传**(node_modules/junk.js → 404,.git/config 返回的是 fallback 的 index.html),与本地 ignoredDirs 一致。
10. **每次部署新 URL 且旧 URL 长期有效**(append-only,实测 5 个旧站点全部存活)。
11. **content[0].text 的 JSON 格式**: Python json.dumps(冒号后带空格、键序 website_id/website_url/screenshot_url)→ 本地 `pyJSON` + `deploySuccess` 已对齐。
12. 无 index.html 不产生 warning(与本地一致);screenshot_url 恒为空字符串。

## schema 保真验证方法

```python
# 抓真实 tools/list → 与 matrix/schema.json 逐项对比 name/description/inputSchema
# 上次对比(2026-08-18): 22/22, DIFFS: 0
```

对比脚本逻辑: 两边都按 name 建 dict,检查缺失/多余/描述差异/inputSchema 排序后 JSON 差异。
线上工具变更时: 重抓 → 覆盖 schema.json → `go test ./...` 全绿即可。

## 调用真实 server 的最小脚本

```python
import json, urllib.request
url = 'http://matrix-mcp-server.weaver.svc.cluster.local:8080/mcp/message?sk=<SK>&source=hermes'
body = json.dumps({'jsonrpc':'2.0','id':1,'method':'tools/list','params':{}}).encode()
r = urllib.request.Request(url, data=body, headers={'Content-Type':'application/json'})
print(urllib.request.urlopen(r, timeout=15).read().decode())
```

(sk 从 `/root/.config/crush/matrix.env` 的 `MATRIX_SK` 读取;禁止明文入库)
