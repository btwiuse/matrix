# 测试与验证现状

## 测试套件 (`go test ./...` 全绿)

| 测试 | 覆盖路径 | 状态 |
|---|---|---|
| `TestToolsList` | stdio: tools/list 22 工具 + 每 schema 是 object + 与嵌入 schema 对齐 | PASS |
| `TestCallToolAllTools` | stdio: table-driven 遍历全部 22 工具,最小合法参数各调一次,断言输出可解析 + 期望键/子串 | PASS |
| `TestCallToolBatchWebSearch` | stdio: 调用 + JSON 输出含 `results` 键(mock) | PASS |
| `TestCallToolGetVoiceList` | stdio: 无参工具调用 + `available_voices` 键 | PASS |
| `TestCallToolRejectsInvalidInput` | stdio: 缺 required 字段 → IsError 而非 panic | PASS |
| `TestProxyHandlerForwardsToRealServer` | proxy: 转发到真实 server 取回真实 voices(不可达时 SKIP) | PASS |
| `TestProxyForwardsLowSideEffectTools` | proxy: get_voice_list / images_list / synthesize_speech 走真实 server,校验输出子串(不可达时 SKIP) | PASS |
| `TestHTTPTransport` | HTTP: 起子进程 `-http` → go-sdk StreamableClientTransport 连接 → list+call(mock) | PASS |
| `TestHTTPTransportProxyMode` | HTTP+proxy: `-mode proxy` 起子进程,list 22 工具 + get_voice_list 转发真实 server(不可达时 SKIP) | PASS |
| `TestLocalDeployCopiesAssets` | LocalDeploy: dist 树完整拷贝到 data/<project>,输出 status/url/files 正确 | PASS |
| `TestLocalDeployDefaultsProjectNameToDistBasename` | project_name 缺省 → 用 dist 目录 basename | PASS |
| `TestLocalDeployRejectsPathTraversal` | project_name `../evil`/绝对路径/含分隔符 → error,且无文件逃逸 | PASS |
| `TestLocalDeploySkipsDevDirsAndSymlinks` | node_modules/.git 不拷贝;指向树外符号链接跳过 | PASS |
| `TestLocalDeployWarnsWithoutIndexHTML` | 无 index.html → warning 字段而非 error | PASS |
| `TestLocalDeployMissingDist` | dist_dir 不存在 → tool error | PASS |
| `TestLocalDeployReplacesPreviousRelease` | 重复部署同项目 → 旧文件清理(发布语义) | PASS |
| `TestLocalDeployOtherToolsDelegate` | 其余工具经内嵌 Handler 委托,不受影响 | PASS |

## 真实 server 对比验证(2026-08-18)

- tools/list 实时对比: 22/22, name/description/inputSchema **DIFFS: 0**。
- 输出形状对比发现并修正 2 处 mock 与真实 server 不一致:
  1. **images_list 返回 markdown 而非 JSON**(`# Total Images: N` + `## Image <file>` 列表),
     mock 已改为镜像该格式,并支持 start/number 分页。
  2. **synthesize_speech 无 `status` 键**,真实形状为
     `{output_file, url, url_clean, url_visible, message: "Speech synthesis completed"}`,
     mock 已改为镜像。
- 方法见 `02-matrix-server-protocol.md`。

## 已知覆盖缺口(尚未补)

1. **昂贵工具有副作用的 e2e 未验**: gen_videos / deploy / deploy_html_presentation /
   batch_text_to_music / images_search_and_download(会真实下载)/ batch_web_search(耗配额)等
   只验了 mock 路径与参数解析,未走真实 server 全量 e2e(避免副作用)。
2. **image_synthesize 等带文件参数的工具**: 未用真实文件走真实 server(需先在 server workspace 有文件)。

## 运行命令

```sh
go test ./...            # 全部
go test ./matrix/ -v     # 只看 matrix,含 PASS 明细
```

注意: 依赖真实 server 的 proxy 测试(TestProxy* / TestHTTPTransportProxyMode)
在 `matrix-mcp-server.weaver.svc.cluster.local:8080` 不可达时自动 SKIP,不影响 CI 绿。
