P. 代码分析报告

本批实现可验证的 **SSH probe 来源门禁**、一次挑战持久消费原语及 root 目录维护窗口预算。生产尚未安装；一次消费尚未接入生产固定入口，完整写入隔离、生产 executor 和 DROP 能力保持 false。单凭 workflow disable、production 限定 main、共享 SSH key 或暂停变量，均不能证明旧 main SHA 的重跑已被拒绝。

## 分析目标与范围

追踪当前 source-only catalog 的 12 个入口、仍被 GitHub 列为 active 的 9 个历史工作流，以及部署/数据库维护的 SSH、直接数据库连接和本地执行旁路。只新增 `internal/apiserver/maintenance/compatibilityretirementfence/` 和本报告；不改当前 workflow、主 retirement Python、业务服务、迁移或其他维护包。

## 入口与调用路径

| 当前入口 | 已读源码事实与关键位置 | 隔离边界 |
| --- | --- | --- |
| `cd.yml` | `cd.yml:310`、`:421`、`:546` 选择 mini/ServerA/ServerD SSH key；`:332`、`:443`、`:571` 传已有 sudo 凭据；`runner-upload-and-deploy.sh:119` 可直接本地执行；`bootstrap/database.go:513`、`:567` 会按配置运行迁移 | SSH、目标机本地 runner、Docker/sudo、服务启动迁移都需独立控制 |
| `db-ops.yml` | `db-ops.yml:93` 起多个 production 作业；`:216` 起元数据工具，后续 restore/cleanup 作业运行 sudo Docker；直接传 SQL/Mongo credentials | 不能把 reviewed readonly profile 推广到全 workflow；旧 ref 可重新进入旧脚本 |
| `authz-production-matrix-provision.yml` | `:14` 共用 production-deploy；`:26` production；`:35` ServerA SSH key；调用既有容器内 provisioner | IAM 主体/角色写入与 Docker exec 要阻断 |
| `authz-production-matrix.yml` | `:11` 独立并发组；`:22` production；`:31` ServerA SSH key | 本次审查的 DB 读取是只读，旧脚本执行权限仍须控制 |
| `compatibility-observation.yml` | `:17` 独立组；`:28` production；`:37` ServerA SSH key；读取指标 | 指标读取不证明 SSH credential 的全历史权限被限制 |
| `attention-reconcile-audit.yml` | `:31` production；`:47` mini→ServerD→ServerA fallback key；`:71` 远程 bash stdin | 当前 source 只读指标，但共享账号/key 可运行其他旧脚本 |
| `m6-qs03-gap-readonly.yml` | `:37` production；上传二进制、容器内读取 AnswerSheet/SQL | 上传与 sudo Docker 能力仍需阻断旧版本 |
| `m5-authz-outage-preflight.yml` | ServerA job 未使用 production environment；SSH+sudo Docker 读取配置；ServerD 可本地 runner 执行 | production branch policy 无法覆盖 repo Secret 与本地执行 |
| `m5-authz-ephemeral-postcheck.yml` | 无 production environment；ServerA/ServerD SSH 读取配置/NSQ | 包括备用 key、直接 host 路径 |
| `ping-runner.yml` | 定时；无 production environment；ServerA/ServerB SSH，ServerD 健康观测；`setup-runner-ssh.sh:67` 也可跳过 SSH | 常规只读检查不等于凭据不可用于写入 |
| `reliable-messaging-m6-qs-image-handoff.yml` | 当前 harness 使用 disposable DB/NSQ 和镜像；发布镜像分支取得 registry credentials | Registry 与 runner 资源是另一边界，不能由 DB readonly 推导全历史无权限 |
| `compatibility-retirement.yml` | `:59` contents/actions read，无 id-token write；`:65` production-deploy；`:233` production；`:310–321` 为数据库 prepare/history 模式传 SQL/Mongo credentials；原 validate 仅约束当前源码 | 现有入口尚不能取得所需 OIDC；即使新源码 validate 正确，旧 workflow revision 的 rerun 也不能由它代为拒绝 |

历史元数据和每个最后实际 run 对应 YAML 已通过只读 GitHub API 获取，原始文件和 API JSON 仅保存在私有 0600 证据目录，正文不进入本报告。该已保存只读快照中，9 个对象的 metadata 均为 active；这不是隔离失败或生产写入的单独证明。

| workflow ID | 实际名称 | 最后实际 source SHA |
| --- | --- | --- |
| 368629149 | M5 AuthZ rollback image availability | `feba1f905ac1b0e58f29bb78a21c0e056deeafa1` |
| 372424442 | M6 Task-compatible API candidate | `15a4ef80c8606a9f04341d80c08907f556728620` |
| 373553659 | M4 current recovery pressure isolation | `4d648907e63c192b5c0bb97f3f51d29804b7439c` |
| 373620355 | M4 Mongo sustained primary loss isolation | `a81114d2c39f30a48519cdd01c2c41d8eac9bfeb` |
| 373624776 | M4 actual SQL Confirm native successful Outcome isolation | `53626cdd1d3b57c41584abdb7c8ebf5b0fcd655d` |
| 373634423 | M4 sustained actual Confirm and native successful Outcomes | `9a9c51588dfeac5b62e6b55a380a2ae7e53b44fa` |
| 373838613 | M4 pressure version handoff (isolated) | `d5266c2223a3864bde049ab7b46f5c2ae14e25f5` |
| 373874329 | M4 Mongo commit PUB timing isolation | `5ad94c5ef3eb5c578e606cb30d140d467ebc4e40` |
| 373875487 | M4 MySQL transaction rejection pressure isolation | `1152c577fa8e568bee6364e3a21602a5fd97469f` |

这些最后实际 YAML 都声明 GitHub-hosted `ubuntu-latest`；可定位当前历史 source 的 disposable harness。除 registry/Actions 相关 Secret 外，本次这些 YAML 未观察到 SSH 或直接 SQL/Mongo Secret 名称。它们不等于每个更早 revision 都已核验，也不等于组织/repo Secret 访问政策已拒绝。因此 adapter 不自动把这些 ID 排除：完整 workflow ID 集合必须独立批准，任一其他非终态 run 仍阻断 probe。

## 当前职责、依赖与实施 API

- `ReadRootPolicy(path, approvedRawSHA)`：严格 root owner、不可由 group/other 写、所有父路径 root protected、NOFOLLOW、regular/single-link、原字节 SHA 和 exact policy JSON；普通部署用户只能读取已批准公开策略。
- `AuthorizeProbe(ctx, borrowedHTTPClient, policy, SSH_ORIGINAL_COMMAND, fixedAuthenticatedKey, stdinOIDC, actualUID, now)`：只接受固定 probe grammar；源码 SHA/op/request/current run/attempt/check-run/actor/repo/owner IDs、SSH key 指纹和 UID 必须精确匹配。借用 HTTP client 只发固定 GET，不关闭其 transport；响应 body 属于本次读取并关闭。
- GitHub OIDC 用固定 issuer/JWKS 和标准 RSA RS256 验签。必须有批准 audience/subject、workflow SHA、event SHA/ref、repository/owner、run/attempt、check-run、actor、production/self-hosted 与有效时间；缺失字段、重复 JSON key、算法变化、伪造、过期或历史 main SHA 均拒绝。JWKS、API 读取失败不放行。主 executable 的 read-only API credential 仅传 `api.github.com`，不发给 issuer 或其他 host，拒 redirect。
- 两次独立 REST 完整分页读取 repository identity、current main、精确 run attempt、全部 workflows、queued/in_progress/waiting/pending/requested 和精确 attempt jobs；固定 body/page/total deadline，超 API 可证明的 1000 results 上限明确拒绝。只有批准 job/runner 可以正在执行，其他非终态都阻断。两次 raw response 摘要必须相等。这是两次观测，不是平台原子 queue freeze。
- `ConsumeProbeChallenge(ctx, fixedRootDir, approvedPolicy, opaquePermit)`：生产 API 要求实际 euid 0、root 所有的 0700 目录与受保护的完整祖先路径。只接受 `AuthorizeProbe` 返回且尚未过期的真实不透明 permit；按相同完整 policy/source/op/run/request/challenge 绑定，通过相对 dirfd 的 NOFOLLOW/NONBLOCK、O_EXCL、文件与目录 fsync、inode/owner/single-link/原字节复读记录 intent/result。成功、半写、中断或未知状态均占用原挑战，禁止覆盖、采用或换审批重用；文件操作后的实际过期再次拒绝。普通用户测试 seam 不是 root 安装证明，目录必须由后续独立管理通道固定绑定。
- `StartMaintenanceWindow` / `OpenMaintenanceWindow`：公共 Linux API 要求真实 euid 0、root 所有的 0700 目录及受保护祖先。固定四目标/source/op/manifest/原 run 绑定；用真实 `CLOCK_BOOTTIME` 与 procfs `boot_id`、目录 flock、O_EXCL primary/commit seal 和文件/目录 fsync 保存原起点。`ForwardContext` 上限 1,200 秒，共享窗口 1,800 秒；`RecoveryContext` 从第一次恢复保存 `min(原起点+1800秒,首次恢复起点+600秒)`，Close/Open 不能重置。25ms 时钟监控是协作取消，不是 OS 硬超时；其他平台拒绝公共生产入口。零值 Close 不释放任何 FD，副本在 mutex 前拒绝使用或释放原 lease；Diagnostic 只返回 `BudgetOnly`，不授予写入或恢复权限。
- `PrepareSSH`：为所有独立批准 raw public keys 生成 `restrict,command=...` 行，固定 policy path/hash、每 key 的真实 fingerprint、read-token path；保留原 from/expiry/verification 限制，证书/环境/未支持 options 阻断。完整原 bytes 只在 opaque plan 中；`RestoreOriginal` 要求当前 bytes 恰好等于已准备 restricted bytes，拒覆盖并发修改。
- `VerifyEffectiveSSHD`：只验证宿主实际 `sshd -T` 投影；独立 root authorized file、无旁路 CA/AuthorizedKeysCommand/password/hostbased/GSSAPI、禁 user env/rc/forwarding/TTY，ForceCommand 不能覆盖 per-key forced command。每个 account/client Match context 都须实际核验，单次 projection 不证明全局。
- 包内 `internal/apiserver/maintenance/compatibilityretirementfence/cmd/qs-retirement-fence` 是可编译真实普通用户 forced-command probe。策略 hash 和 key 来源必须来自 root 安装的固定 key line，不能来自客户端 env。JWT 仅 stdin；45 秒 executable deadline 由宿主关闭自己的 stdin；不执行客户端 shell、DB、CAS、DDL 或生产 executor。

## 行为、契约与不变量

`SSHProbeInvocationVerified` 只表示该 probe 的来源/当前平台读取验证通过；`ProductionInstalled`、`WholeSystemWriterFenceProven`、`MutationBackendEnabled`、`DropReady` 始终 false。原 probe 回执继续保留 `ReplayProtectionUnproven=true`；单独的 `ConsumeProbeChallenge` 只记录本次 probe 挑战的持久消费，permit 不可用于生产写入。外部调用者不能通过任意 imported complete/count/boolean 生成 permit。

按 GitHub 官方语义，rerun 使用原事件的 SHA/ref，production 分支规则匹配 ref，所以 main-only 不排除旧 main SHA。[GitHub rerun 文档](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/re-run-workflows-and-jobs)，[Environment 规则](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments)。本适配层同时核验 signed workflow SHA 和独立 current-main/run API。[OIDC primary reference](https://docs.github.com/en/actions/reference/security/oidc)，[issuer discovery（含 sha/RS256）](https://token.actions.githubusercontent.com/.well-known/openid-configuration)。

SSH forced command 本身不禁止 forwarding，key 的 command 也可被 server ForceCommand 覆盖；这就是严格禁旁路与独立 root effective config 验证的原因。[OpenSSH sshd_config](https://man.openbsd.org/sshd_config.5)，[authorized_keys restrictions](https://man.openbsd.org/sshd.8)。

## 测试、可观测性与分析指标

入口与契约：绿，固定 grammar/signature/identity/platform双读取可直接测试。历史源与平台 queue 覆盖：黄，9 实际最后 source 已读取，仍不证明全部历史 revision 或平台原子暂停。普通用户权限：绿，本批 owned network-none/noports Linux 容器中，root:root 0644 policy、root:1234 0640 token、root 0555 binary，真实 UID/GID 1234 能读不可写；会通过文件/credential 层后因网络隔离明确拒绝远端读取。历史 shell 在此之前拒绝。真实 root-only 0600 token 明确拒绝，没有以 root-only 成功冒充部署用户可用。完整生产写隔离：红，以下缺口尚未完成；可由真实独立源、账号/session、网络和服务核验消除。

本地真实 `/bin/sh`→独立 Go 子进程→loopback HTTP 的 synthetic 签名/平台 fixture，覆盖批准 probe、旧 shell/scp/SFTP/注入、旧 main SHA、历史 rerun、排队/待审批/等待与错误隐私；另有完整跨页 catalog、重复/缺失结果、跨 repo/actor/key/UID、有效时间、配置并发变化、受保护路径和原字节 rollback 测试。它们不是实际 GitHub OIDC 或生产 sshd 安装的验收。

修复前源码 `2a89d5577ddfb6598b54eda6f1ec22670da1deb2` 的五包竞态/覆盖率组合回归有 711 条测试记录，其中 710 条通过、1 条并发挑战错误分类失败；独立实际 Linux euid 0 调用还在受保护路径校验中拒绝了公共入口的根锚点 `/`。普通用户测试 seam 的通过不能代替真正 root 公共入口的通过，文档门禁通过也不能覆盖这些失败。后续修复必须分别绑定新源码、公共入口原生结果和完整并发回归；生产 root 安装与完整写隔离仍未证明。

提交 `b3c546f0bdd5d79b4d69d42130a813f6937e1b9d` 保留完整祖先路径、所有权和持久结果校验，允许精确的 `/` 根锚点。打开目录时发现同挑战的既有保留记录，只把该竞态归为拒绝重用，不接受变化目录、不采用状态或授予执行权。该提交的完整五包竞态/覆盖率回归为 713 条测试及 5 条包级结果全部通过、0 失败、0 跳过；三个针对用例各重复 20 次，共 60 条测试全部通过。独立真实 Linux arm64 euid 0 公共 API 原生测试已终态通过：非 race、非 coverage，1 个父测试、0 子测试、0 失败、0 跳过。其 `2a89` 加两个精确库文件覆盖版本与 `b3c546` 的 4,026 个跟踪文件逐字节等价，额外私有原生夹具单独声明。真实签名检查与两次完整 synthetic 本地 API 快照产生 permit，再调用公共 `ConsumeProbeChallenge`；两个状态文件均为 0600 单链接，拒绝 0702 目录、symlink、重放和错误 source，独立 CID/volume 清理剩余为 0。安全回执 `qs-fence-linux-root-public-api-native-20261009.json` 的 SHA256 为 `afab6c0c43b158aaa406409b215a0192bc0417738af003c0d9cbc68d17a16cd6`。它不证明真实 GitHub origin、生产 root 安装、executor、完整写隔离、CAS 或 DROP；旧 `4b402` 的 root 拒绝证据保持。

提交 `d252d11fd80bf42ea23d9eb5c5d2a92f8eb57cc2` 已纳入维护窗口四文件。独立实际 Linux arm64 root 用例 `TestMaintenanceWindowActualLinuxRootLease` 真实终态为 1 个父测试通过、0 子测试、0 失败、0 跳过，非 race、非 coverage；`b7e8b905` 完整 archive 加精确四文件与该提交的 4,030 个跟踪文件逐字节等价，额外 metadata-only 私有 harness 单独绑定。真实 euid 0、CLOCK_BOOTTIME、procfs、root 0700 tmpfs、互斥 flock 和五个 root 0600 单链接文件经过读取核验；Start→返回 forward cancel→OpenBusy→首次 Recovery→Close/Open 保留原起点/恢复截止时间。该用例没有取消 parent context，不能记录为原生 parent-cancel 证明。安全回执 `qs-maintenance-window-linux-root-native-20261009.json` 的 SHA256 为 `22f06e68fa71c41e54ca110c13311c634c51f13524db5890c3aaf74fb3b67b8e`；容器墙钟约 0.307 秒，独立两次 CID/volume 读回均为 0。此次只证明预算组件公共入口；不是完整业务链在 600 秒内恢复的演练，不证明生产安装、真实 GitHub origin、全写者隔离、executor、CAS、DROP 或部署许可，生产能力保持 false。

## 主要风险、缺口及最小下一步

1. **管理通道未证明**：源码只显示现有 deploy SSH/sudo Docker/install 使用；没有读取实际 authorized-key fingerprints、effective sshd configs、sudoers 或独立管理连接。不能猜同一账号临时限制后仍能自行恢复。需独立 root 管理通道，先保存所有原 file bytes/mode/owner/unset policy 状态及 hash，再 native 验证配置、安装前后和精确 rollback，禁止 sudo 执行上传目录的可写解释器。
2. **完整身份与权限**：root 批准全部实际 SSH key/account/host 集合和 policy IDs；每个认证来源及 Match context 封住旁路。已建立 SSH sessions/local runner 不受新 key restriction 追溯影响，必须另行实际 drain 与本地执行隔离。 key forced command 仍由账号 login shell -c 启动；restrict 禁止 ~/.ssh/rc，却不能保证 Bash 不先读取用户可写 ~/.bashrc。须核对真实 shell/hash、所有启动文件和父路径、loader 环境；必要时另行受控暂时绑定不会读取用户启动文件的可信 shell，精确保留/恢复原账号配置，不能自行 chmod 用户 home 或仅加 key restriction 就宣称完整隔离。[GNU Bash startup primary reference](https://www.gnu.org/software/bash/manual/html_node/Bash-Startup-Files)。
3. **直接 DB/外部写者**：当前主工具能直接获得生产 DB credentials；业务 API/Collection/Worker、scheduler、host cron/manual、其他 repo/qs-ai/IAM、外部管理员连接仍可绕 SSH。需实际账号 grants/current sessions/network writer scope、服务停止/连接释放和独立复读，不改业务账号密码/权限扩大，也不把 Docker stopped 与网络所有写者静止混为一谈。
4. **GitHub 许可与 queue**：当前 workflow 无 id-token:write，生产源码集成尚未做。read-only host token 的实际权限、environment/repo/org/reusable workflows 与 Secrets/protection原状态需实际读取；非终态作业多次完整分页和 installed gate 的真实拒绝实验缺一不可。
5. **重放保护与 executor**：挑战原子一次消费、本批私有持久结果和中断未知语义已实现并进行本地文件及并发进程测试；生产 root 目录的固定安装、当前 SSH 来源与实际 executor 接线未完成。所有许可仍仅 probe；只有源码、安装、生产 origin、队列、写者与一次消费闭环通过后，才可另行接固定受保护 executor。不得先打开 mutation capability。
6. **恢复原状态**：源 adapter 只保留最小临时 original bytes 并拒绝冲突；宿主必须实际 atomic/CAS 安装和恢复、验收后清理所有本批原始 assets。没有永久备份或绕过变更状态的 blanket rollback。
