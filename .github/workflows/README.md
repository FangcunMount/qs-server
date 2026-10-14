# GitHub Actions 与交付契约

本页只维护 workflow 的稳定职责、输入、安全边界和本地验证入口。目标主机、实际副本数、runner 版本、某次部署 SHA、探针结果和数据扫描数量属于时点证据，统一记录在[基础设施生产证据台账](../../docs/00-总览/10-基础设施生产证据台账.md)。

## 1. Workflow 责任

| Workflow | 稳定职责 | 状态写入边界 |
| --- | --- | --- |
| `ci.yml` | 单测、静态/架构/配置契约、一次性 Mongo Replica Set 上的事务/schema 集成契约、文档与 API 同步、构建 | 只证明该 run 的 checkout；不证明部署 |
| `sonar.yml` | SonarQube 分析 | 不替代 CI、部署或运行验收 |
| `cd.yml` | 计划服务、构建/交付镜像、生成部署包、按依赖顺序部署、逐实例验证 | 成功结果必须记录 exact SHA、image digest、effective config 与环境 |
| `ping-runner.yml` | 周期/手动执行目标环境连通与服务探针 | 只证明该时点、该检查项；不证明业务全链和数据一致性 |
| `db-ops.yml` | 受保护的备份、恢复与只读数据盘点入口 | 写操作/恢复需要独立授权、备份和复验；不输出凭据 |
| `attention-reconcile-audit.yml` | 受控 Attention reconciliation 审计 | 不得把 dry-run 自动升级为 apply |
| `compatibility-observation.yml` | 只读观察仍在用的 Evaluation legacy binding 指标 | 无指标、零值、无命中和证据缺失必须区分 |
| `authz-production-matrix.yml` | 使用生产主体执行只读 IAM AuthZ v3 精确 12 项矩阵 | 覆盖角色与 origin、缺属性、错误属性类型及 force_retry；IAM 快照复核角色，不写角色或业务数据 |
| `authz-production-matrix-provision.yml` | 显式创建隔离的 evaluator 与 plan manager 矩阵主体 | 仅手工触发并要求固定确认词；幂等创建无登录凭据/联系方式/外部身份的 IAM 用户及单一角色 assignment，不修改现有用户 |

R4 源码退出候选移除两个专用退役工作流及其打包、恢复夹具和临时运行观察检查。CI 保留标准事务证据、当前 MQ、退休 ID 防护、迁移与正常启动保护；普通构建和部署保留原交付入口。四对象的实际生产验收和材料零剩余仍需独立回执，见[退役工具退出说明](../../scripts/database/compatibility-retirement.md)。

删除、重命名或改变触发关系时，必须同步 `scripts/cd` 契约测试和文档门禁。历史 run 只进入证据台账，不回写本页。

## 2. 交付链

```text
CI for exact SHA
  -> plan-services
  -> build image + metadata/digest
  -> prepare-package (configs + env + scripts)
  -> target dependency/certificate preflight
  -> remote-deploy
  -> per-instance health/readiness/governance
  -> dated evidence ledger entry
```

配置、Secret、镜像、Compose 和网络 canonical 边界见[配置与部署](../../docs/03-基础设施/config-deployment/README.md)，部署执行步骤见[部署与端口 runbook](../../docs/04-接口与运维/06-部署与端口.md)。

## 3. 输入与 Secret

- workflow input/Variable 只表达服务计划、环境和非敏感参数；数据库、Redis、JWT、委托 key、AI Provider API key、registry token、SSH key 与 TLS private key 必须来自受保护 Secret/目标主机。
- `scripts/cd/prepare-package.sh` 在每个部署包内生成独立的 `config.prod.env`；日志只允许显示变量名和脱敏 endpoint，不得输出值。
- CI 的公开镜像读取使用仓库级 `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` Secrets，token 只需读取权限。服务容器在步骤执行前通过 `services.credentials` 登录；步骤内启动容器或构建镜像的 job 使用登录 Action，并在 job 结束时退出登录。未配置凭据的运行仍执行相同测试并匿名拉取，可能受到下载限流；凭据不进入构建参数或生产部署包。
- QS 不再注入模型 API Key；模型凭据由 qs-ai 的部署管理，不得进入 QS 部署包、镜像 build args 或 GitHub Actions 日志。
- 自动部署必须校验目标 SHA 没有被更新提交取代；手动部署仍须记录调用者、输入、environment 和审批。
- self-hosted runner 的网络、SSH、Docker 权限和工具版本属于环境前置条件，不能写死为当前事实。

两个有界只读入口只接受手工派发，必须在 checkout 前提交 `governance_confirm=<完整 GitHub SHA>` 且 `backup_name` 为空；不匹配时不进入携带数据库 Secret 的读取步骤。它们使用 production Environment 和 Mac mini 自托管 runner，但 source SHA 只标识工具源码，不是生产应用部署身份。

| operation / database | 只读入口及连接来源 | 证据边界 |
| --- | --- | --- |
| `metadata-readiness-admin` / `mysql` | 固定 `root@47.94.204.124:22` 接收器，现有 `MYSQL_METADATA_ADMIN_*`；六成员静态 Linux 包使用独立 `--metadata-readiness` 模式 | 绑定读取前后数据库身份、限定目录及 readiness 读取；原五成员权限 profile 保持独立 |
| `metadata-visibility-admin-profile` / `mongodb` | 原 Mongo 日常备份的 `MONGO_BACKUP_*` / `MONGODB_*` 候选配置；Mac mini 原生 reader 使用独占 loopback SSH tunnel，认证库固定 `admin` | 仅观测固定 qs / rs0 / member / migration / UUID 目标的身份、FCV、TTL monitor 和认证元数据；不执行日常备份脚本，不回退其它连接 |

`complete` 只证明本次已声明的有界读取和 owned 连接释放。metadata count、digest、权限及 FCV 不证明对象无业务责任、完整 namespace 导出、备份对可恢复或停写 RPO=0，也不授权停写、恢复、DDL 或删除。`system.profile` 的业务恢复排除范围另须保留定义、索引和计数回执，生产集合继续保留。两个 reader 均不初始化业务容器、迁移 runner、消息消费者或发送端点；现用 backup、validate、restore、status 和 scheduled job 使用各自原入口。

Mongo 最多执行 10 个固定逻辑只读查询，异常时至多释放一个本 reader 拥有的 cursor；driver 握手不计为业务查询。`ttl_monitor.known=false` 表示未知，不能把零值 `enabled=false` 解释为已关闭。TTL 参数仅为本次观测，不扫描 TTL 业务数据、不读取 currentOp、不执行关闭动作，也不证明未来持续状态或维护冻结。

Mongo 的最终传输回执增加固定 `failure_stage`，区分 SSH keyscan、指纹、路由、隧道、自有监听者、native reader 和释放阶段；native reader 的 JSON 协议保持原样。成功必须为 `none`，未知阶段拒绝；失败不输出命令、连接地址或 stderr。MySQL readiness 对服务端 3024／1317、权限拒绝及 Go context deadline／cancel 使用不同固定错误类别，其余继续为 `query_failed`；不会根据运行时长猜测原因、调整读取预算或自动重试。失败回执不出具完整测量结论。

实际总耗时超限时，仅最终 `reading_budget_exceeded`／`total_budget` 失败回执可以保留耗时和已证明的释放字段，三个完成标记必须全部为 false；native 回执及所有成功回执仍受原45秒上限约束。该诊断例外不延长执行时间或把未证释放标记设为true。

MySQL未知typed服务端错误使用 `query_mysql_error` 并保留可选uint16号码；仅该类型实际出现时复制Number，旧回执无该字段仍有效。原样的网络错误、InvalidConn、BadConn及失败EOF各自分类，InvalidConn不推定为timeout，正常EOF不失败。不得输出Message、SQLState、网络Op／Addr，释放失败及部分测量规则保持原样。

CI 为两个 reader 各建独立 fixture：MySQL 8.0.36、认证 MongoDB 7.0.31 rs0。必须设置 required 标记，精确 selector 的 JSON 事件必须实际 RUN/PASS；任意 SKIP、缺失 selector 或依赖未就绪均失败。Mongo fixture 使用固定官方 digest、owned network/container label 和唯一 127.0.0.1 随机端口，不复用业务 runtime fixture。

## 4. 本地非缓存验证

```bash
go test -count=1 ./internal/pkg/configcontract
go test -race -count=1 ./scripts/oneoff/mysql_metadata_profile
python3 -B scripts/dbops/test-mysql-metadata-profile.py
python3 -B scripts/dbops/test-mysql-metadata-profile-ssh.py
python3 -B scripts/dbops/test-receipt-transport.py
python3 -B scripts/dbops/test-mysql-metadata-readiness.py
python3 -B scripts/dbops/test-mongodb-metadata-profile.py
python3 -B scripts/testing/test-m4-metadata-readiness-native-proof.py
# 仅在各自独占 fixture 和 required 身份变量已绑定后执行：
make test-metadata-readiness-mysql-native
make test-metadata-readiness-mongodb-native
bash scripts/cd/test-github-action-runtimes.sh
bash scripts/cd/test-plan-services.sh
bash scripts/cd/test-prepare-package.sh
bash scripts/cd/test-production-compose-network-exposure.sh
bash scripts/cd/test-wait-worker-readiness.sh
bash scripts/cd/test-verify-worker-dependencies.sh
bash scripts/cd/test-verify-worker-governance.sh
bash scripts/cd/test-verify-authz-production-matrix.sh
bash scripts/cd/test-provision-authz-production-matrix-evaluator.sh
```

这些命令不读取生产 Secret，也不证明目标主机可达。真实部署必须另存 workflow URL/run ID、exact source/deployed SHA、image digest、effective-config hash、逐实例检查、limitations 和失效期。

## 5. 安全与恢复边界

- 不在 workflow 中无边界执行 `docker compose down`、全局 orphan 清理、volume prune 或自动重启所有容器。
- 只删除本 workflow/project 精确拥有且已被新实例替代的资源；先验证新实例，再切流，再清理旧资源。
- migration dirty、数据不兼容、证书身份错误、依赖不可达或 probe 语义不符时应停止部署，不用重试掩盖。
- 回滚旧镜像前先判断已落地 schema/event/config 是否向后兼容；one-off 写入不能靠镜像回滚恢复。
- `/healthz`、`/readyz`、`/serve-readyz` 和 governance 语义不同，验收按[健康探针 canonical 文档](../../docs/03-基础设施/observability/20-健康探针、治理与持久证据.md)解释。

## 6. 生产证据要求

workflow 成功后若未写入证据台账，只能称为“run 成功”，不能称为当前生产已签署。每条记录至少绑定：

- `observed_at`、environment、owner；
- source/deployed SHA 和 image digest；
- effective config hash 或具名 unknown limitation；
- workflow/命令、结果和原始 evidence reference；
- 探针/查询覆盖、limitations、`expires_on`、`supersedes`。
