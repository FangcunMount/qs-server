# CI/CD 部署说明（prod compose）

工作流分工：

| 文件 | 用途 | 触发 |
| ---- | ---- | ---- |
| `ci.yml` | 代码质量与可构建性（test / lint / depguard / security advisory / build） | `pull_request` / `push` → `main` |
| `cd.yml` | 生产发布（`workflow_run` 或手动） | `CI` 成功后 / `workflow_dispatch` |
| `ping-runner.yml` | 生产 ServerA + ServerD runner 健康自检 | 每 6 小时 / `workflow_dispatch` |
| `db-ops.yml` | MongoDB 备份 / 恢复 / 状态 | 每日定时备份 / `workflow_dispatch` |

已移除的 workflow（冗余或失效）：

- `server-check.yml`：与 `ping-runner` 的 ServerA 检查重复，且含自动 restart 容器等高风险逻辑
- `test-ssh.yml`：手动 SSH 诊断，可由 `ping-runner` `workflow_dispatch` 替代
- `seeddata-runner.yml`：指向不存在的 `tools/seeddata-runner/`（seeddata 为独立仓库）

本仓库生产部署直接以仓库内配置文件为准，不再额外注入“升配/降配档位”。流程要点：

1. 镜像：CD 先规划本次受影响服务，只构建并推送需要发布的 `qs-apiserver` / `qs-collection-server` / `qs-worker` 镜像到 GHCR/Docker Hub；目标机按本次 `DEPLOY_SHA` 对应的不可变 tag 拉取。
2. 包含文件：`deploy-package` 会携带 `configs`、`configs/env/config.prod.env` 以及 `docker-compose.prod.yml`。
3. 目标机操作：
   - 备份现有 configs，展开 deploy-package。
   - apiserver 使用单实例 `docker compose up -d`。
   - collection 使用固定 Compose project `qs-collection` 和 `--scale` 启动全部副本，并逐实例检查 `/readyz` 与镜像 tag。
4. 资源配额：直接维护在 `build/docker/docker-compose.prod.yml`（serverA 4C/8G：apiserver + collection x2 同机；collection 两副本共享原总预算）。
5. 服务内部并发/连接池：直接维护在 `configs/apiserver.prod.yaml`、`configs/collection-server.prod.yaml`、`configs/worker.prod.yaml`。
6. Collection 副本数：workflow_dispatch 可填写 `collection_replicas`，留空时读取仓库变量 `QS_COLLECTION_REPLICAS`，缺失时默认 `2`。
7. Worker 副本数：workflow_dispatch 可填写 `worker_replicas`，留空时读取仓库变量 `QS_WORKER_REPLICAS`，缺失时默认 `3`。

生产拓扑（2026-06）：

| 主机 | 规格 | 组件 |
| ---- | ---- | ---- |
| serverA | 4C/8G | nginx、qs-apiserver、qs-collection-server x2 |
| serverB | 2C/2G | IAM（`iam-apiserver`） |
| serverD | 4C/4G | qs-worker |

`deploy-collection` 与 `deploy-apiserver` 均 SSH 到 `SVRA_*`；访问 serverB 上的 `iam-apiserver:9090` 依赖 Swarm overlay `infra-network` 跨机 DNS，**不要** `extra_hosts` 到宿主机 Tailscale IP（serverB 宿主机 9090 常被 mihomo 占用）。

collection 副本不发布固定宿主机 `8082/6060` 端口。Nginx 与 apiserver governance 通过外部 Docker 网络上的稳定服务别名 `qs-collection-server` 访问；Prometheus 依靠每个容器的 scrape labels 分别发现 target。部署和 runner 自检通过 Compose label 枚举并逐容器执行 `/readyz`。多副本日志只写 stdout/stderr，由 Docker 为每个容器独立轮转，禁止多个进程写同一个宿主机日志文件。

将 `QS_COLLECTION_REPLICAS` 改为 `1` 可回滚副本数；若使用 workflow_dispatch 临时覆盖为 `1`，应同步更新仓库变量，否则定时 `ping-runner` 会按期望值 `2` 报警。Compose 扩容只提供进程级冗余，同机 serverA 故障仍会同时影响两个副本。

CD 本地入口：

- `make cd-plan`
- `make cd-image SERVICE=apiserver DEPLOY_REF=main DEPLOY_SHA=<sha>`
- `make cd-package SERVICE=apiserver`
- `make cd-export-image SERVICE=apiserver DEPLOY_SHA=<sha>`
- `make cd-remote-deploy SERVICE=apiserver IMAGE_TAG=<sha>`
- `make cd-validate SERVICE=apiserver`

镜像构建与拉取：

- `cd-image` 默认使用 GHCR registry cache：`ghcr.io/fangcunmount/<image>:buildcache`。
- 生产部署默认走 **tarball 直传（阿里云 ACR 模式）**：`docker` job 构建推 GHCR/Docker Hub 后 **同步 push ACR**；ServerD 从 **国内 ACR** `docker pull`（秒级～分钟级）→ `save | gzip` → SCP → 目标机 `docker load`。GHCR/Docker Hub 仍作备份。
- 手动部署或未上传 tarball 时，`DEPLOY_IMAGE_SOURCE=auto|registry` 会 fallback 到 registry pull；此时默认优先 Docker Hub（`DEPLOY_PULL_REGISTRY=dockerhub`），再回退 GHCR。
- 自动触发时，CD 脚本、workflow、文档、测试等非运行时变更不会触发生产服务发布；手动触发仍按输入选择 `all/apiserver/collection/worker`。
- 远端若本地已有同 tag 镜像，或已从 tarball load，则跳过 registry pull。

Secrets 传递规则：

- GitHub secrets 只在 `cd.yml` 的 `env:` / action `with:` 中读取。
- Makefile 和 `scripts/cd/*` 只通过环境变量接收值，不写 GitHub `${{ secrets.* }}` 表达式。
- 不把 token/password 作为 Make 参数或脚本 CLI 参数传递。
- 生产 `config.prod.env` 只在部署包中生成，日志输出必须脱敏。

## 自托管 Runner（Mac mini，组织级）

`plan` / `docker` / `notify` 仍跑 GitHub-hosted；各仓库的 `deploy-*` 跑在 Mac mini runner group `qlume`（标签 `self-hosted, macOS, ARM64`），替代原 ServerD `QS_DEPLOY_RUNNER=serverd`。

部署链路：

1. GitHub-hosted 构建镜像并推 GHCR / Docker Hub / ACR
2. Mac mini：ACR `docker pull --platform linux/amd64` → 导出 tarball
3. 公网 SCP（`SVRA_PUBLIC_HOST` / `SVRD_PUBLIC_HOST`）→ 目标机 `docker load` + compose up

前置：

- runner group `qlume` 允许本仓库
- org Variables：`SVRA_PUBLIC_HOST`、`SVRD_PUBLIC_HOST`（已配置）
- org Secret：`SVR_MINI_SSH_KEY`（或回退 `SVRA_SSH_KEY` / `SVRD_SSH_KEY`）
- Mac mini Docker Desktop 可用；隔离 `DOCKER_CONFIG` 避免 keychain 卡住

目标主机：

| Job | 目标 | 公网 Variable | hostname 校验 |
| --- | --- | --- | --- |
| deploy-apiserver | serverA | `SVRA_PUBLIC_HOST` | `serverA` |
| deploy-collection | serverA | `SVRA_PUBLIC_HOST` | `serverA` |
| deploy-worker | serverD | `SVRD_PUBLIC_HOST` | `serverD`（不区分大小写） |

> `IAM_GRPC_HOST` 仍用 `SVRB_HOST`（Tailscale），供 worker 机上 compose 解析集群内 IAM，不要改成公网 IP。

### 1.1 阿里云 ACR（组织 Secrets）

1. 开通 [容器镜像服务 ACR 个人版](https://cr.console.aliyun.com/)，创建**命名空间**（如 `fangcunmount`）
2. 访问凭证 → 设置 **固定密码**
3. 组织 **Settings → Secrets** 添加：

| Secret | 示例 |
| ------ | ---- |
| `ALIYUN_ACR_REGISTRY` | 个人版用**公网**地址，如 `crpi-xxx.cn-beijing.personal.cr.aliyuncs.com`（概览页复制；不是 `registry.cn-*.aliyuncs.com`） |
| `ALIYUN_ACR_NAMESPACE` | `fangcunmount` |
| `ALIYUN_ACR_USERNAME` | 访问凭证页用户名（如 `clack`） |
| `ALIYUN_ACR_PASSWORD` | 访问凭证 → **设置固定密码** |

4.组织 Variable：`QS_DEPLOY_EXPORT_REGISTRY=acr`（或删除该变量，默认已是 `acr`）

ServerD 验证：

```bash
echo "<ACR_PASSWORD>" | docker login crpi-xxx.cn-beijing.personal.cr.aliyuncs.com -u clack --password-stdin
docker pull crpi-xxx.cn-beijing.personal.cr.aliyuncs.com/fangcunmount/qs-apiserver:<sha>
```

**GitHub Deploy Key（只读）**：仓库 Settings → Deploy keys → Add → 勾选只读；私钥存 Secrets `QS_SERVER_DEPLOY_KEY`。自托管 job 用 **SSH checkout**（`git@github.com` → `ssh.github.com:443` → Mihomo CONNECT）。

### 2. 获取 Registration Token（一次性）

这是 **Runner 注册专用 token**，不是 Personal Access Token（PAT），也**不用**单独去 Developer settings 申请。

1. 打开组织：`https://github.com/organizations/fangcunmount/settings/actions/runners`
   - 或：**fangcunmount** → **Settings** → **Actions** → **Runners**
2. 点 **New runner** → **New self-hosted runner**
3. 选 **Linux** / **x64**，页面会显示 `./config.sh --url https://github.com/fangcunmount --token XXXXX`
4. 复制其中的 `XXXXX` 作为 `<RUNNER_TOKEN>`（约 **1 小时**内有效，过期重新点 **New self-hosted runner** 再取）

注册完成后 runner 长期在线，**日常 CD 不需要**再保存或使用这个 token。

### 3. 在 ServerD 安装 Runner（组织级，多实例并行）

目录建议 `/opt/actions-runner/runner{1,2,3}/`，标签均为 `serverd`（deploy job 并行）：

```bash
RUNNER_VER=2.334.0
TARBALL=/tmp/actions-runner-linux-x64-${RUNNER_VER}.tar.gz
curl -fsSL -o "$TARBALL" -L \
  "https://github.com/actions/runner/releases/download/v${RUNNER_VER}/actions-runner-linux-x64-${RUNNER_VER}.tar.gz"

install_runner() {
  local name=$1 dir=$2 token=$3
  mkdir -p "/opt/actions-runner/$dir"
  tar xzf "$TARBALL" -C "/opt/actions-runner/$dir"
  cd "/opt/actions-runner/$dir"
  ./config.sh --url https://github.com/fangcunmount --token "$token" \
    --name "$name" --labels serverd --unattended
  cp /opt/actions-runner/runner1/.env .env 2>/dev/null || cp scripts/cd/runner-dotenv.example .env
  sudo ./svc.sh install deploy && sudo ./svc.sh start
}

install_runner serverD-runner1 runner1 <TOKEN1>
install_runner serverD-runner2 runner2 <TOKEN2>
install_runner serverD-runner3 runner3 <TOKEN3>
```

注意 `--url` 是 **组织地址** `https://github.com/fangcunmount`；`svc.sh` 须在各自目录内执行：`cd /opt/actions-runner/runner1 && sudo ./svc.sh status`。

成功后，组织 **Settings → Actions → Runners** 应出现 3 个带 `serverd` 标签的 runner（Idle）。

**组织 Runner 可见范围**（组织 Settings → Actions → Runners → 该 runner → Repository access）：

- 推荐 **All repositories**，或
- **Selected repositories** 仅勾选需要走 ServerD 部署的仓库

### 4. 各仓库配置

每个要用 ServerD 部署的仓库单独配置（仓库或组织 Variables 均可）：

| Variable | 值 | 说明 |
| -------- | -- | ---- |
| `QS_DEPLOY_RUNNER` | `serverd` | deploy job 的 `runs-on` |
| `QS_DEPLOY_EXPORT_REGISTRY` | `acr` | ServerD 从阿里云 ACR pull；`ghcr`/`dockerhub` 为回退 |
| `QS_DEPLOY_HTTP_PROXY` | `http://127.0.0.1:7890` | HTTP(S) 工具走 Mihomo |
| `QS_DEPLOY_ALL_PROXY` | `socks5://127.0.0.1:7891` | 可选 SOCKS 代理 |
| `QS_DEPLOY_NO_PROXY` | `127.0.0.1,localhost,内网` | 生产 SSH/SCP 不走代理 |

| Secret | 说明 |
| ------ | ---- |
| `QS_SERVER_DEPLOY_KEY` | GitHub Deploy Key 私钥（SSH checkout） |

组织级 Variable 可设一次、全仓库生效；仓库级 Variable 可覆盖组织默认值。

**Environment 放行**：各仓库 **Settings → Environments → `production`** → 允许 self-hosted runner（否则 deploy job 会 Pending）。

`ping-runner.yml` 的 `ping-serverd` job 同样使用 `QS_DEPLOY_RUNNER`（默认 `serverd`），每 6 小时自检 runner 服务、Docker/部署工具、到 A/B 的 SSH 连通性。

自托管 runner 上 **不用** `appleboy/ssh-action`；生产 SSH/SCP 走原生 `setup-runner-ssh.sh`，GitHub 拉代码走 **SSH + Mihomo 代理**（`setup-runner-network.sh`）。

### 5. 多项目共用说明

| 问题 | 答案 |
| ---- | ---- |
| 多个项目要多个目录吗？ | **不需要**；组织级一个 `/opt/actions-runner` 即可 |
| 多个项目同时 CD？ | 单 runner **同时只跑一个 job**；同机多实例（`runner1/2/3`，同标签 `serverd`）可并行 deploy |
| secrets 怎么隔离？ | 仍按**仓库 / Environment** 注入；runner 只是执行机，不共享业务 secrets |

### 6. 流量路径（ServerD 模式）

```text
GitHub 触发 CD
  → docker job（GitHub-hosted）构建推 GHCR + Docker Hub + 同步 push ACR
  → deploy job（ServerD runner ×3 并行）
       → docker pull ACR（国内直连，NO_PROXY .aliyuncs.com）→ save tarball
       → SCP → ServerA / ServerB / ServerD（Tailscale 内网）
       → remote-deploy.sh（docker load + compose up）
```

ACR 镜像 tag：`registry.cn-<region>.aliyuncs.com/<namespace>/qs-<service>:<DEPLOY_SHA>`。

如需本地开发，请使用 `build/docker/docker-compose.dev.yml`。
