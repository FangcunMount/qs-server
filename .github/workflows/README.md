# CICD 部署说明（prod compose）

本仓库 CI 在部署阶段改为使用 `build/docker/docker-compose.prod.yml`，流程要点：

1. 镜像：从 GHCR/Docker Hub 拉取 `qs-apiserver` / `qs-collection-server` / `qs-worker`。
2. 包含文件：`deploy-package` 会携带 `configs`、`configs/env/config.prod.env` 以及 `docker-compose.prod.yml`。
3. 目标机操作：
   - 备份现有 configs，展开 deploy-package。
   - 使用 `docker compose -f /tmp/deploy-package/docker-compose.prod.yml up -d <service>` 启动指定服务。
4. 网络与挂载：依赖外部网络 `qs-network`，日志与 TLS 路径为 `/data/logs/qs-server/<svc>` 与 `/data/ssl/...`。
5. 资源限额：compose 文件内设定 CPU/内存（apiserver 0.25C/384M，collection 0.20C/256M，worker 0.15C/256M）。

如需本地开发，请使用 `build/docker/docker-compose.dev.yml`。
