# AGENTS.md

本仓库是 HTML 注入代理 + MiniMax matrix MCP server 高保真复刻（Go）。
主代码在 `htmlinject`（根包）、`matrix/`（子包）、`rewrite/`、`cmd/matrix`（单一入口）。

`memory/` 是本仓库的持久记忆目录，以 git submodule 挂载，远端即附属 wiki 仓库
`btwiuse/matrix.wiki`。所有调研、架构决策、协议细节、可复用经验按主题
沉淀到 `memory/` 下（一个主题一个文件），并维护 `memory/Home.md` 索引
（session 启动时通过 `.crushrc` 的 `option context-path memory/Home.md` 自动加载）。

更新 `memory/` 内容后，运行 `scripts/sync-wiki.sh` 将其同步到 wiki 仓，
再 `git submodule update --remote memory` 推进本地 submodule 指针。
不要直接修改 `memory/` 下的 submodule 内部 git 历史。

详细布局见 `memory/04-repo-layout.md`，测试策略见 `memory/05-testing-verification.md`。
