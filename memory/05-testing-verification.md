# 测试与验证现状

## 测试套件 (`go test ./...` 全绿)

| 测试 | 覆盖路径 | 状态 |
|---|---|---|
| `TestToolsList` | stdio: tools/list 22 工具 + 每 schema 是 object + 与嵌入 schema 对齐 | PASS |
| `TestCallToolBatchWebSearch` | stdio: 调用 + JSON 输出含 `results` 键(mock) | PASS |
| `TestCallToolGetVoiceList` | stdio: 无参工具调用 + `available_voices` 键 | PASS |
| `TestCallToolRejectsInvalidInput` | stdio: 缺 required 字段 → IsError 而非 panic | PASS |
| `TestProxyHandlerForwardsToRealServer` | proxy: 转发到真实 server 取回真实 voices(不可达时 SKIP) | PASS |
| `TestHTTPTransport` | HTTP: 起子进程 `-http` → go-sdk StreamableClientTransport 连接 → list+call | PASS |

## 真实 server 对比验证

- tools/list 实时对比(2026-08-18): 22/22, name/description/inputSchema **DIFFS: 0**。
- 方法见 `02-matrix-server-protocol.md`。

## 已知覆盖缺口(尚未补)

1. **工具覆盖不全**: 22 个工具仅 3 个有调用级测试(batch_web_search / get_voice_list / deploy-非法输入)。
   其余 19 个(images_*、audios_*、videos_*、gen_videos、music、speech 族等)只验证了注册,
   没有调用路径测试 → 参数解析 bug 可能漏检。
2. **proxy 全工具转发未验**: 只转发了 get_voice_list;其余 21 个工具经 proxy 到真实 server
   的参数兼容性未知(真实 server 对某些参数组合可能报错)。
3. **HTTP 模式仅 mock**: TestHTTPTransport 用 `-mode mock`;proxy 模式走 HTTP 未测。

## 补测试建议

- 加一个 table-driven 测试: 遍历全部 22 工具,用 mock handler 各调一次,断言返回 JSON 可解析 + 无 IsError。
- proxy 补测: `images_list`、`synthesize_speech`(带 voice)等低副作用工具走真实 server。
- 注意: 真实 server 调用有副作用(生成文件/部署),不要对昂贵工具(gen_videos、deploy)做全量 e2e。

## 运行命令

```sh
go test ./...            # 全部
go test ./matrix/ -v     # 只看 matrix,含 PASS 明细
```
