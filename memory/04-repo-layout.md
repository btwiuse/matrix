# 仓库布局与约定

Module: `github.com/gearshell/inject-proxy`(go 1.27rc3)
Git: 本仓库,main 分支;身份用 `Crush <crush@local>`(仓库级,勿动 global config)

## 布局

```
/workspace/html-inject-proxy/
├── htmlinject.go          # htmlinject 包: HTML 注入反向代理库(tokenizer 版;命名避开 main 惯例)
├── htmlinject_test.go     # htmlinject 测试(含用户回归测试)
├── server.js             # Node 版参考实现(旧)
├── inject-ball.html      # 注入片段示例
├── rewrite/              # rewrite 包: HTML 注入核心(幂等 tokenizer 重写)
│   ├── rewrite.go        #   Injector: New(injection) + Inject(html) (string, bool)
│   └── rewrite_test.go   #   注入测试(从根包移入)
├── cmd/
│   ├── inject-proxy/     # cobra CLI: --upstream --port --inject/--inject-html
│   └── matrix/           # 唯一入口 (cobra CLI): MCP + 站点托管,单一 HTTP 监听
├── matrix/               # matrix 复刻库(独立包,非根包!)
│   ├── schema.json       # 真实 server tools/list 抓取(go:embed)
│   ├── types.go handler.go server.go proxy.go mock.go deploy.go site.go router.go envelope.go
│   └── server_test.go deploy_test.go site_test.go router_test.go envelope_test.go
├── test/                 # 手工测试文件
├── memory/               # 本知识库(见 README.md)
└── README.md             # 仓库总览
```

## 关键约定

1. **根包 = htmlinject**;matrix 代码必须在 `matrix/` 子包(Go 单目录单包)。
   新增组件同样:库代码放子目录,CLI 放 `cmd/<name>/main.go`。
2. **新增 matrix 工具五步**: schema.json(重新抓取) → types.go(结构体) →
   handler.go(接口方法) → server.go(reg1) → proxy.go + mock.go(实现)。
3. 所有 .go 文件无注释要求(用户偏好代码自解释),中文注释可留但别废话。
4. **单进程 = MCP + 站点托管**: 无 stdio,唯一 transport 是 streamable HTTP
   (`--http` 默认 `$PORT` 或 `:8080`)。`--data-dir` 开启本地 deploy + Host 路由
   (`matrix/router.go`): 站点命名空间 = apex 的 "/" + `<site>.<domain>` 子域,
   其余(监听地址 127.0.0.1、apex 非根路径如 /mcp、未知 Host)全走 MCP 端点。
   `cmd/deploy-server` 已删除(2026-08-18 合并),flags 并入 matrix:
   `--data-dir --domain --inject --inject-html`。
   注意: 未给 MCP 保留 `mcp.<domain>` 子域 —— go-sdk 的 DNS rebinding 防护在
   loopback 监听下会 403 非 loopback Host(见 03)。
4. **本地 deploy(与正式版完全对齐,2026-08-18 实测)**: `matrix/deploy.go` 的 `LocalDeploy` 包装任意
   Handler,只覆写 Deploy。对齐点:① dist_dir 必须位于 workspace(默认 /workspace,`-workspace-dir`
   可配)子目录,workspace 本身也不行,报错措辞逐字照抄正式版;② 缺 dist_dir **不强制 required**(本地
   schema 注册时去掉 required,`envelope.go` 在 tools/list 响应里补回),默认 `<workspace>/dist`;
   ③ 成功输出 Python 风格 JSON(冒号后带空格,键序 website_id/website_url/screenshot_url),
   `website_url` = `http://<site-id>.<domain>`(**无尾斜杠**,同正式版 `https://<id>.space.mcode.cn`);
   ④ `website_id` = 431 前缀 + 12 位**随机**(同正式版,非递增);⑤ 错误走 `ToolError` → content 为
   JSON 文本(同样 Python 风格);**isError 逐类对齐**: 缺 dist = false(softToolError),路径越界 = true;
   ⑥ 无 index.html 不报 warning(实测正式版无 warning,尽管 schema 描述说有);⑦ 每次部署新随机
   site-id(12 位,append-only,旧发布保留);⑧ 措辞对齐: workspace 拒绝文案 `under <ws>/`(尾斜杠,
   逐字同正式版);缺 dist 的 message 追加 `Error: <os.Stat 错误>` 详情(正式版是 file gateway 404
   详情,本地用等价物);⑨ project_name 完全不校验(`../evil` 也能部署,同正式版);⑩ 成功结果额外
   `display_data {website_id, website_url}` + 显式 isError:false(go-sdk 表达不了,
   `envelope.go` 在 HTTP 层注入,SSE/纯 JSON 两种 framing 都处理);⑪ 站点托管对齐正式版 gateway:
   无目录列表(无 index.html 的目录 404)、显式 /index.html 直接 200(不 301)、缺失的无扩展名路径
   SPA fallback 到 index.html、带扩展名缺失路径 404。跳过 .git/node_modules/符号链接。
4. **注入核心 = rewrite 包**(根包 htmlinject 的 Injector 是薄包装:内嵌 *rewrite.Injector + verbose);
   `matrix --inject/--inject-html` 通过 `NewSiteHandlerWithInjector` 复用同一实现:
   SiteHandler.injector 非 nil 时,目录 index.html(含 SPA fallback 与显式 /index.html)被重写,
   其余普通文件原样服务。
5. token/密钥一律不进 git:`matrix.env` 在 `/root/.config/crush/`,600 权限。
6. 提交信息: 主题行 ≤72 字符,解释 why;署名 `Crush <crush@local>`。

## 历史提交(脉络)

```
2cb6466 初始: Node + Go 双实现 HTML 注入代理
d4c9206 Go 代理重构成库 + cobra CLI + 测试套件
ef10757 加固: 上游误配置容错
d0295fd 回归测试: 上游怪癖
0131475 HTML5 tokenizer 注入(替代字符串匹配)
638117a matrix MCP 复刻并入本仓库(本知识库的来源任务)
```

## 环境事实

- 工作区 /workspace;ufo term 相关(RELAY=...?persist=1)见 `.hermes` 记忆,与本仓库无关。
- 本机可达真实 matrix server(192.168.203.224),token 见 crush matrix.env。
