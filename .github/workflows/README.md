# CICD 部署说明（prod compose）

本仓库生产部署直接以仓库内配置文件为准，不再额外注入“升配/降配档位”。流程要点：

1. 镜像：从 GHCR/Docker Hub 拉取 `qs-apiserver` / `qs-collection-server` / `qs-worker`。
2. 包含文件：`deploy-package` 会携带 `configs`、`configs/env/config.prod.env` 以及 `docker-compose.prod.yml`。
3. 目标机操作：
   - 备份现有 configs，展开 deploy-package。
   - 使用 `docker compose -f /tmp/deploy-package/docker-compose.prod.yml up -d <service>` 启动指定服务。
4. 资源配额：直接维护在 `build/docker/docker-compose.prod.yml`。
5. 服务内部并发/连接池：直接维护在 `configs/apiserver.prod.yaml`、`configs/collection-server.prod.yaml`、`configs/worker.prod.yaml`。
6. Worker 副本数：不再硬编码；workflow_dispatch 可填写 `worker_replicas`，留空时读取仓库变量 `QS_WORKER_REPLICAS`，缺失时默认 `1`。

如需本地开发，请使用 `build/docker/docker-compose.dev.yml`。
