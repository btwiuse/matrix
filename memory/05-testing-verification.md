# 测试与验证现状

## 测试套件 (`go test ./...` 全绿)

| 测试 | 覆盖路径 | 状态 |
|---|---|---|
| `TestToolsList` | HTTP(子进程): tools/list 22 工具 + 每 schema 是 object + 与嵌入 schema 对齐 | PASS |
| `TestCallToolAllTools` | HTTP: table-driven 遍历全部 22 工具,最小合法参数各调一次,断言输出可解析 + 期望键/子串 | PASS |
| `TestCallToolBatchWebSearch` | HTTP: 调用 + JSON 输出含 `results` 键(mock) | PASS |
| `TestCallToolGetVoiceList` | HTTP: 无参工具调用 + `available_voices` 键 | PASS |
| `TestCallToolRejectsInvalidInput` | HTTP: 缺 required 字段 → IsError 而非 panic | PASS |
| `TestProxyHandlerForwardsToRealServer` | proxy: 转发到真实 server 取回真实 voices(不可达时 SKIP) | PASS |
| `TestProxyForwardsLowSideEffectTools` | proxy: get_voice_list / images_list / synthesize_speech 走真实 server,校验输出子串(不可达时 SKIP) | PASS |
| `TestHTTPTransport` | HTTP: 起子进程 `-http` → go-sdk StreamableClientTransport 连接 → list+call(mock) | PASS |
| `TestHTTPTransportProxyMode` | HTTP+proxy: `-mode proxy` 起子进程,list 22 工具 + get_voice_list 转发真实 server(不可达时 SKIP) | PASS |
| `TestLocalDeployCopiesAssets` | LocalDeploy: dist 树完整拷贝到 data/<site-id>,输出 website_id/website_url/screenshot_url(与正式版同形状) | PASS |
| `TestLocalDeployProjectNameDoesNotDetermineLocation` | project_name/dist basename 不决定发布位置(随机 site-id,同正式版) | PASS |
| `TestLocalDeployRejectsPathTraversal` | project_name `../evil`/绝对路径/含分隔符 → tool error,且无文件逃逸 | PASS |
| `TestLocalDeployRejectsDistOutsideWorkspace` | dist_dir 在 workspace 外 → 措辞同正式版的 tool error(`under <ws>/` 尾斜杠) | PASS |
| `TestLocalDeployRejectsWorkspaceItself` | workspace 根本身不可用 → tool error | PASS |
| `TestLocalDeployMissingDist` | dist 不存在 → `{"error":"dist directory does not exist","message":"...built files. Error: <stat 错误>"}`(message 含正式版同款 "Error:" 详情) | PASS |
| `TestLocalDeployDefaultsDistDirToWorkspaceDist` | 缺 dist_dir → 默认 `<workspace>/dist` | PASS |
| `TestLocalDeploySkipsDevDirsAndSymlinks` | node_modules/.git 不拷贝;指向树外符号链接跳过 | PASS |
| `TestLocalDeployNoIndexHTMLSucceedsWithoutWarning` | 无 index.html → 成功且无 warning(对齐实测) | PASS |
| `TestLocalDeployFreshWebsiteIDPerDeployment` | 每次部署 website_id 递增 | PASS |
| `TestLocalDeployKeepsPreviousReleases` | 重复部署 → 每次新随机目录/URL,旧发布保留(对齐正式版每次新子域) | PASS |
| `TestLocalDeployAbsoluteURLWithDomain` | 配置 Domain 后 website_url = http://<site-id>.<domain>/ | PASS |
| `TestLocalDeployOtherToolsDelegate` | 其余工具经内嵌 Handler 委托,不受影响 | PASS |
| rewrite 包: `TestInjectIdempotent` / `TestInjectFallbackAppend` / `TestInjectIgnoresLiteralTags` / `TestInjectPreservesFormat` | 注入核心(幂等、兜底追加、字面 `</body>` 不误插、逐字节保真),从根包移入 rewrite 包 | PASS |
| `TestSiteHandlerRewritesIndexHTML` | SiteHandler + injector: `/` 与 `/sub/` 的 index.html 被重写,snippet 在 `</body>` 前,非 HTML 资产不动 | PASS |
| `TestSiteHandlerRewriteIsIdempotent` | 二次请求不重复注入,两次响应一致 | PASS |
| `TestSiteHandlerNoRewriteWithoutInjector` | 未配 injector 时原样服务(旧行为) | PASS |
| `TestSiteHandlerRewriteLeavesPlainPagesAndListingsAlone` | about.html 不重写;无 index.html 的目录仍走 FileServer 列表;显式 `/index.html` 仍是重定向 | PASS |
| `TestSiteHandlerRewriteHeadRequest` | HEAD: 200, Content-Length = 重写后长度,空 body | PASS |
| `TestRouterDispatch` | Router 单测: 子域/apex 列表 → 站点;监听地址/apex 非根路径/未知 Host → MCP;大小写与端口不敏感 | PASS |
| `TestHTTPHostRouting` | e2e 合并后单进程: 经 MCP deploy → 按 website_url 子域取回站点(含注入 snippet)、apex 列表、站点子域上的 MCP 路径 404 | PASS |
| `TestLoadInjection` | `--inject` 文件优先于 `--inject-html`;文件缺失报错 | PASS |

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
