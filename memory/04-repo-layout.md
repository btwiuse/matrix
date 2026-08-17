# 仓库布局与约定

Module: `github.com/gearshell/inject-proxy`(go 1.27rc3)
Git: 本仓库,main 分支;身份用 `Crush <crush@local>`(仓库级,勿动 global config)

## 布局

```
/workspace/html-inject-proxy/
├── main.go               # htmlinject 包: HTML 注入反向代理库(tokenizer 版)
├── main_test.go          # htmlinject 测试(含用户回归测试)
├── server.js             # Node 版参考实现(旧)
├── inject-ball.html      # 注入片段示例
├── cmd/
│   ├── inject-proxy/     # cobra CLI: --upstream --port --inject/--inject-html
│   ├── matrix/           # matrix MCP 复刻入口 (cobra CLI): --mode --url --token --http
│   └── deploy-server/    # cobra CLI: 二级域名 site server (--data-dir --domain --http)
├── matrix/               # matrix 复刻库(独立包,非根包!)
│   ├── schema.json       # 真实 server tools/list 抓取(go:embed)
│   ├── types.go handler.go server.go proxy.go mock.go deploy.go
│   └── server_test.go deploy_test.go
├── test/                 # 手工测试文件
├── memory/               # 本知识库(见 README.md)
└── README.md             # 仓库总览
```

## 关键约定

1. **根包 = htmlinject**;matrix 代码必须在 `matrix/` 子包(Go 单目录单包)。
   新增组件同样:库代码放子目录,CLI 放 `cmd/<name>/main.go`。
2. **新增 matrix 工具五步**: schema.json(重新抓取) → types.go(结构体) →
   handler.go(接口方法) → server.go(regXxx) → proxy.go + mock.go(实现)。
3. 所有 .go 文件无注释要求(用户偏好代码自解释),中文注释可留但别废话。
4. **本地 deploy(与正式版完全对齐,2026-08-18 实测)**: `matrix/deploy.go` 的 `LocalDeploy` 包装任意
   Handler,只覆写 Deploy。对齐点:① dist_dir 必须位于 workspace(默认 /workspace,`-workspace-dir`
   可配)子目录,workspace 本身也不行,报错措辞逐字照抄正式版;② 缺 dist_dir 默认 `<workspace>/dist`;
   ③ 成功输出形状 `{"website_id","website_url","screenshot_url"}`,`website_url` 暂为 `/data/<project>/`
   占位(http server 以后做);④ 错误走 `ToolError` → content 为 JSON 文本 + isError=true(register 已支持),
   与正式版一致;⑤ 无 index.html 不报 warning(实测正式版无 warning,尽管 schema 描述说有);
   ⑥ 每次部署 website_id 递增(基准 431000000000000),同项目覆盖旧版本(本地无 CDN 版本化)。
   跳过 .git/node_modules/符号链接。
4. token/密钥一律不进 git:`matrix.env` 在 `/root/.config/crush/`,600 权限。
5. 提交信息: 主题行 ≤72 字符,解释 why;署名 `Crush <crush@local>`。

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
