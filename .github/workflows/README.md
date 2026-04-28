# CI/CD 部署说明（prod compose）

工作流分工：

1. `CI` (`ci.yml`)：只做代码质量与可构建性验证，运行在 `pull_request -> main` 和 `push -> main`，不读取生产 secrets，不发布镜像。
2. `Production Deploy` (`cd.yml`)：在 `CI` 的 `main` 成功后通过 `workflow_run` 自动触发，也支持手动 `workflow_dispatch`，并绑定 `production` environment 做人工审批。
3. `cd.yml` 只负责编排、权限、审批和 secrets 注入；具体发布动作走 Makefile 入口和 `scripts/cd/*`。
4. 运维类 workflow（健康检查、数据库操作、SSH 测试）独立保留，不作为 CI/CD 主链路的一部分。

本仓库生产部署直接以仓库内配置文件为准，不再额外注入“升配/降配档位”。流程要点：

1. 镜像：CD 构建并推送 `qs-apiserver` / `qs-collection-server` / `qs-worker` 到 GHCR/Docker Hub，目标机再拉取。
2. 包含文件：`deploy-package` 会携带 `configs`、`configs/env/config.prod.env` 以及 `docker-compose.prod.yml`。
3. 目标机操作：
   - 备份现有 configs，展开 deploy-package。
   - 使用 `docker compose -f /tmp/deploy-package/docker-compose.prod.yml up -d <service>` 启动指定服务。
4. 资源配额：直接维护在 `build/docker/docker-compose.prod.yml`。
5. 服务内部并发/连接池：直接维护在 `configs/apiserver.prod.yaml`、`configs/collection-server.prod.yaml`、`configs/worker.prod.yaml`。
6. Worker 副本数：不再硬编码；workflow_dispatch 可填写 `worker_replicas`，留空时读取仓库变量 `QS_WORKER_REPLICAS`，缺失时默认 `3`。

CD 本地入口：

- `make cd-image SERVICE=apiserver DEPLOY_REF=main DEPLOY_SHA=<sha>`
- `make cd-package SERVICE=apiserver`
- `make cd-remote-deploy SERVICE=apiserver`
- `make cd-validate SERVICE=apiserver`

Secrets 传递规则：

- GitHub secrets 只在 `cd.yml` 的 `env:` / action `with:` 中读取。
- Makefile 和 `scripts/cd/*` 只通过环境变量接收值，不写 GitHub `${{ secrets.* }}` 表达式。
- 不把 token/password 作为 Make 参数或脚本 CLI 参数传递。
- 生产 `config.prod.env` 只在部署包中生成，日志输出必须脱敏。

如需本地开发，请使用 `build/docker/docker-compose.dev.yml`。
