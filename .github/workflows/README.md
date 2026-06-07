# CI/CD 部署说明（prod compose）

工作流分工：

1. `CI` (`ci.yml`)：只做代码质量与可构建性验证，运行在 `pull_request -> main` 和 `push -> main`，不读取生产 secrets，不发布镜像。
2. `Production Deploy` (`cd.yml`)：在 `CI` 的 `main` 成功后通过 `workflow_run` 自动触发，也支持手动 `workflow_dispatch`，并绑定 `production` environment 做人工审批。
3. `cd.yml` 只负责编排、权限、审批和 secrets 注入；具体发布动作走 Makefile 入口和 `scripts/cd/*`。
4. 运维类 workflow（健康检查、数据库操作、SSH 测试）独立保留，不作为 CI/CD 主链路的一部分。

本仓库生产部署直接以仓库内配置文件为准，不再额外注入“升配/降配档位”。流程要点：

1. 镜像：CD 先规划本次受影响服务，只构建并推送需要发布的 `qs-apiserver` / `qs-collection-server` / `qs-worker` 镜像到 GHCR/Docker Hub；目标机按本次 `DEPLOY_SHA` 对应的不可变 tag 拉取。
2. 包含文件：`deploy-package` 会携带 `configs`、`configs/env/config.prod.env` 以及 `docker-compose.prod.yml`。
3. 目标机操作：
   - 备份现有 configs，展开 deploy-package。
   - 使用 `docker compose -f /tmp/deploy-package/docker-compose.prod.yml up -d <service>` 启动指定服务。
4. 资源配额：直接维护在 `build/docker/docker-compose.prod.yml`。
5. 服务内部并发/连接池：直接维护在 `configs/apiserver.prod.yaml`、`configs/collection-server.prod.yaml`、`configs/worker.prod.yaml`。
6. Worker 副本数：不再硬编码；workflow_dispatch 可填写 `worker_replicas`，留空时读取仓库变量 `QS_WORKER_REPLICAS`，缺失时默认 `3`。

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

## 自托管 Runner（ServerD，组织级）

在 **GitHub 组织 `fangcunmount`** 注册 **一台** ServerD runner，组织内多个仓库可共用（无需每个项目一个 `/opt/actions-runner` 目录）。

`plan` / `docker` / `notify` 仍跑 GitHub-hosted；各仓库的 `deploy-*` 通过变量 `QS_DEPLOY_RUNNER=serverd` 切到该 runner。

### 1. ServerD 前置依赖

- Docker（daemon + **containerd** 均需配置 HTTP 代理，见下方「Docker daemon 代理」）
- `git`、`make`、`gzip`、`openssh-clients`（`ssh`/`scp`/`nc`）
- Mihomo 代理（默认 `127.0.0.1:7890` HTTP / `7891` SOCKS5）
- 到 ServerA/ServerB/ServerD 的 SSH（内网直连，不走 GitHub 代理）

```bash
# 每个 runner 实例各一份 .env（进程级代理，供拉 action / git 等）
for d in runner1 runner2 runner3; do
  cp scripts/cd/runner-dotenv.example /opt/actions-runner/$d/.env
  chown deploy:deploy /opt/actions-runner/$d/.env
done
```

**Docker daemon 代理**（`docker pull` / `docker login` 走 daemon，不是 shell 代理）：

```bash
sudo tee /etc/systemd/system/docker.service.d/http-proxy.conf <<'EOF'
[Service]
Environment="HTTP_PROXY=http://127.0.0.1:7890"
Environment="HTTPS_PROXY=http://127.0.0.1:7890"
Environment="NO_PROXY=127.0.0.1,localhost,100.64.0.0/10,.aliyuncs.com"
EOF
sudo mkdir -p /etc/systemd/system/containerd.service.d
sudo cp /etc/systemd/system/docker.service.d/http-proxy.conf \
  /etc/systemd/system/containerd.service.d/http-proxy.conf
sudo systemctl daemon-reload && sudo systemctl restart containerd docker

# 验证 ACR 直连（不走代理，期望 401 或 200）
curl -sS -o /dev/null -w "acr:%{http_code}\n" https://<ALIYUN_ACR_REGISTRY>/v2/
```

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

4. 组织 Variable：`QS_DEPLOY_EXPORT_REGISTRY=acr`（或删除该变量，默认已是 `acr`）

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

```
GitHub 触发 CD
  → docker job（GitHub-hosted）构建推 GHCR + Docker Hub + 同步 push ACR
  → deploy job（ServerD runner ×3 并行）
       → docker pull ACR（国内直连，NO_PROXY .aliyuncs.com）→ save tarball
       → SCP → ServerA / ServerB / ServerD（Tailscale 内网）
       → remote-deploy.sh（docker load + compose up）
```

ACR 镜像 tag：`registry.cn-<region>.aliyuncs.com/<namespace>/qs-<service>:<DEPLOY_SHA>`。

如需本地开发，请使用 `build/docker/docker-compose.dev.yml`。
