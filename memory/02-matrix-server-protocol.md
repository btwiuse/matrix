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
