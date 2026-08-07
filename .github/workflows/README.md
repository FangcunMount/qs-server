# CI/CD 部署说明（prod compose）

工作流分工：

| 文件 | 用途 | 触发 |
| ---- | ---- | ---- |
| `ci.yml` | 代码质量与可构建性（test / lint / depguard / security advisory / build） | `pull_request` / `push` → `main` |
| `cd.yml` | 生产发布（`workflow_run` 或手动） | `CI` 成功后 / `workflow_dispatch` |
| `sonar.yml` | Mac mini 本地 SonarQube 扫描（不阻断 CD） | `push` → `main` |
| `ping-runner.yml` | ServerA 生产巡检 + ServerD worker 主机巡检 | 每 6 小时 / `workflow_dispatch` |
| `db-ops.yml` | MongoDB 备份 / 恢复、MySQL/MongoDB 状态及 Redis 只读键空间盘点（读 `production` Environment secrets）；传入 `audit_from` 时还会对 Attention 缺口做私密跨 Mongo/MySQL 只读分类；`govern-scale-metadata` 以不可变 manifest 和新 release 治理医学量表分类 | 每日定时备份 / `workflow_dispatch` |
| `attention-reconcile-audit.yml` | 对三副本 Attention dry-run 指标做只读验收；仅在形成新治理清单并获得授权后临时启用生产扫描 | `workflow_dispatch` |
| `compatibility-observation.yml` | 只读查询生产 Prometheus，分类三个公开兼容接口、AnswerSheet 旧幂等 lookup/hit 和 Evaluation 无冻结 admission 回退的活跃命中、完整零窗口或历史覆盖不足；零命中仍须结合指标语义与调用方/数据 Owner 确认 | 每日 / `workflow_dispatch` |

JavaScript Action 必须使用原生 Node 24 主版本：测试覆盖率、扫描报告和三个应用制品统一使用 `actions/upload-artifact@v7`，镜像构建使用 `docker/setup-buildx-action@v4` 与 `docker/login-action@v4`。覆盖率仍由 `go test` 与 `go tool cover` 生成，`coverage.out` 作为保留 14 天的 workflow artifact 上传，缺失时阻断；仓库未启用且长期静默失败的 Codecov 外部上传已退出。`scripts/cd/test-github-action-runtimes.sh` 在 CI 中阻止回退到由 GitHub 强制转译运行的 Node 20 主版本，并阻止重新引入未配置的 Codecov Action。

已移除的 workflow（冗余或失效）：

- `build.yml`：已更名为 `sonar.yml`（原名易与 CI Build 混淆）
- `server-check.yml`：与 `ping-runner` 的 ServerA 检查重复，且含自动 restart 容器等高风险逻辑
- `test-ssh.yml`：手动 SSH 诊断，可由 `ping-runner` `workflow_dispatch` 替代
- `seeddata-runner.yml`：指向不存在的 `tools/seeddata-runner/`（seeddata 为独立仓库）
- `statistics-stale-run-settlement.yml`：一次性精确结算已由 31063053544 完成，31063110773 独立只读对账通过后关闭入口
- `attention-reconcile-apply-audit.yml`：91 条精确清单已由 31113670701 验收，31113758030 独立对账归零后关闭入口

本仓库生产部署直接以仓库内配置文件为准，不再额外注入“升配/降配档位”。流程要点：

1. 镜像：CD 先规划本次受影响服务，只构建并推送需要发布的 `qs-apiserver` / `qs-collection-server` / `qs-worker` 镜像到 GHCR/Docker Hub；目标机按本次 `DEPLOY_SHA` 对应的不可变 tag 拉取。
2. 包含文件：`deploy-package` 会携带 `configs`、`configs/env/config.prod.env` 以及 `docker-compose.prod.yml`。
3. 目标机操作：
   - 备份现有 configs，展开 deploy-package。
   - apiserver 使用单实例 `docker compose up -d`。
   - collection 使用固定 Compose project `qs-collection`、service key `server` 和 `--scale` 启动全部副本，容器名为 `qs-collection-server-N`，并逐实例检查 `/serve-readyz` 与镜像 tag。
   - worker 使用固定 Compose project `qs-worker`、service key `runtime` 和 `--scale` 启动全部副本，容器名为 `qs-worker-runtime-N`。
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

collection 副本不发布固定宿主机 `8082/6060` 端口。Nginx 与 apiserver governance 通过外部 Docker 网络上的稳定服务别名 `qs-collection-server` 访问。worker 保留兼容别名 `qs-worker`，并只在 Swarm overlay `infra-network` 上发布治理专用别名 `qs-worker-governance`；专用别名不得出现在各宿主机互不相通的本地 `qs-network` bridge 上。这些 DNS 别名与用于生成可读容器名的短 service key 分离。Prometheus 依靠每个容器的 scrape labels 分别发现 target。部署和 runner 自检通过 Compose label 枚举并逐容器执行 `/serve-readyz`；它要求首次 control sync 已完成，但允许 Redis 运行期降级。可靠提交/K6 仍检查严格 `/readyz`。多副本日志只写 stdout/stderr，由 Docker 为每个容器独立轮转，禁止多个进程写同一个宿主机日志文件。

apiserver 生产配置对 collection governance 使用 `discovery: dns` 和
`minimum_instances: 2`，对 Docker DNS 返回的两个 IPv4 并发读取
`/governance/resilience` 与 `/governance/redis`。一个副本不可达时返回 partial，
不会把存活副本误报 unavailable。worker governance 使用独立的
`qs-worker-governance`、`discovery: dns` 和 `minimum_instances: 3`。worker 或
apiserver 发布完成后，CD 会从 serverA 的 `qs-apiserver` 容器反向校验 DNS
恰好返回三个 IPv4、两个治理端点均可达且返回三个不同的 `instance_id`；定时
`ping-runner` 重复同一验收。多出陈旧地址、少副本、端点超时或实例重复都会失败。

collection 发布要求 Nginx `>= 1.27.3`。`collect-api` upstream 在块内使用 Docker resolver 和 `server qs-collection-server:8080 resolve`，保持默认轮询，不设置 `ip_hash`、固定权重或主备关系。CD 会备份并原子安装 `/data/apps/nginx-configs/collect.conf`，通过 `nginx -t` 后 reload，再用 `nginx -T`、Docker DNS 地址集合和两个副本的 `/health` 请求指标完成切流验收；失败时恢复原配置并重启暂时停用的旧 collection service。

首次采用短 service key 发布时，CD 会先启动并验证新副本，暂时停止同一 Compose project 下旧 `qs-collection-server` service 以排除旧 DNS 地址，切流验收通过后再精确删除旧 collection / worker service 容器；不会使用固定 `container_name`，也不会无边界执行 `docker compose down` 或全局 orphan 清理。

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
- 自动触发只接受当前 `main` HEAD；为避免三个进程出现版本错位，当前策略会全量发布 `apiserver/collection/worker`，包括仅修改 CD、workflow、文档或测试的提交。过期 SHA 会跳过；手动触发仍按输入选择 `all/apiserver/collection/worker`。
- 远端若本地已有同 tag 镜像，或已从 tarball load，则跳过 registry pull。

Secrets 传递规则：

- GitHub secrets 只在 `cd.yml` 的 `env:` / action `with:` 中读取。
- Makefile 和 `scripts/cd/*` 只通过环境变量接收值，不写 GitHub `${{ secrets.* }}` 表达式。
- 不把 token/password 作为 Make 参数或脚本 CLI 参数传递。
- 生产 `config.prod.env` 只在部署包中生成，日志输出必须脱敏。

## 自托管 Runner（Mac mini，组织级）

`plan` / `docker` / `notify` 仍跑 GitHub-hosted；各仓库的 `deploy-*` 跑在 Mac mini runner group `qlume`（标签 `self-hosted, macOS, ARM64`），替代原 ServerD `QS_DEPLOY_RUNNER=serverd`。`ping-runner` 的 ServerA 巡检和 `db-ops` 使用同一 runner group 中带 `ops` 标签的实例，因为 `SVRA_HOST` 是 GitHub-hosted runner 无法访问的 Tailscale 地址；ServerD worker 主机巡检仍留在 `serverd` runner。

部署链路：

1. GitHub-hosted 构建镜像并推 GHCR / Docker Hub / ACR
2. Mac mini：ACR `docker pull --platform linux/amd64` → 导出 tarball
3. 公网 SCP（`SVRA_PUBLIC_HOST` / `SVRD_PUBLIC_HOST`）→ 目标机 `docker load` + compose up

前置：

- runner group `qlume` 允许本仓库
- runner group `qlume` 至少有一个带 `self-hosted, macOS, ARM64, ops` 标签的在线 runner
- org Variables：`SVRA_PUBLIC_HOST`、`SVRD_PUBLIC_HOST`（已配置）
- org Variable：`SVRA_SSH_FINGERPRINT`（ServerA SSH host key 的 SHA256 指纹）
- org Secret：`SVR_MINI_SSH_KEY`（或回退 `SVRA_SSH_KEY` / `SVRD_SSH_KEY`）
- Mac mini Docker Desktop 可用；隔离 `DOCKER_CONFIG` 避免 keychain 卡住

`SVRA_SSH_FINGERPRINT` 应从 ServerA 控制台或已有可信 SSH 会话读取。`appleboy/ssh-action@v1.2.2` 当前与 ServerA 协商 ECDSA host key，因此这里必须配置 ECDSA 指纹，不能使用 OpenSSH 客户端默认选中的 ED25519 指纹：

```bash
ssh-keygen -lf /etc/ssh/ssh_host_ecdsa_key.pub -E sha256
```

Mac mini ops runner 上线前还应确认 `tailscale ping serverA` 和 `nc -vz 100.85.122.124 22` 成功。DERP 中继会增加时延，但不阻断低流量巡检和远程数据库命令；应继续排查 NAT/UDP 条件以争取直连。

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
# 定档时点（2026-08-06）的 GitHub Actions Runner 正式版；新安装前仍须在
# https://github.com/actions/runner/releases/latest 复核，不要长期照抄旧版本号。
RUNNER_VER=2.336.0
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

不要在 `config.sh` 中使用 `--disableupdate`。既有实例必须逐个滚动升级：一次只停止一个 runner，确认其版本、systemd 状态和组织端 `Idle` 后再处理下一个，始终至少保留两个实例接单。版本与服务状态只读核验：

```bash
for dir in runner1 runner2 runner3; do
  printf '%s\t' "$dir"
  "/opt/actions-runner/$dir/bin/Runner.Listener" --version
  cd "/opt/actions-runner/$dir"
  sudo ./svc.sh status
done
```

升级完成后必须手动运行 `Ping Runner`，确认 ServerD 三个 `Runner.Listener`、Docker、到 ServerA/ServerB 的 SSH 以及三个 worker 容器全部通过。2026-08-06 的生产巡检仍观测到 `2.334.0`，且 GitHub 明确提示其将在 2026-08-10 停止接单，因此升级到不低于 `2.336.0` 是当前定档 P1，而不是可忽略的文档建议。

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

`ping-runner.yml` 的 `ping-worker-host` job 使用 `QS_DEPLOY_RUNNER`（默认 `serverd`）巡检 **ServerD worker 主机**（Docker / worker 容器 / 到 A·B SSH），不再承担 CD deploy runner 职责；CD deploy 已迁到 Mac mini `qlume`。该 job 直接使用 Actions `vars` 与 runner 本地 `.env` 解析代理并写入 `GITHUB_ENV`；它既不 checkout，也不访问 GitHub SSH，因此不再从 `raw.githubusercontent.com` 下载网络脚本，避免把无关外网可用性变成生产主机巡检的前置条件。独立 `setup-runner-network.sh` 仍用于真实 runner 配置场景，其 GitHub SSH 探测具有连接时限。

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
