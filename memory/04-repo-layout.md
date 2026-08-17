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
│   └── matrix/           # matrix MCP 复刻入口: -mode -url -token -http
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
4. **本地 deploy**: `matrix/deploy.go` 的 `LocalDeploy` 包装任意 Handler,只覆写 Deploy,
   把 dist 拷贝到 `data-dir/<project>`(project_name 缺省用 dist 目录名;跳过 .git/node_modules/符号链接;
   无 index.html 给 warning 不报错;每次部署清空旧版本)。main.go `-data-dir` 开启,url 暂返回
   `/data/<project>/` 占位,http server 以后做。
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
