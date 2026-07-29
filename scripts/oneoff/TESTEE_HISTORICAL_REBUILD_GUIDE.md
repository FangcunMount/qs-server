# Testee 历史数据重建分步指导

本文是一次性重建 `2025-01-01..2026-07-27` Testee 历史业务事实的端到端执行手册。
执行顺序固定为：

```text
Staging 全流程演练
→ 生产维护窗口与三库同时间点备份
→ QS MySQL 清零
→ QS MongoDB 清零
→ IAM Testee Profile/ProfileLink 清理
→ 空状态验收
→ seeddata-runner 历史回填
→ Statistics repair/validate/publish
→ K6 验收
→ 关闭历史能力并保存证据
```

本文负责串联完整流程。删除脚本的字段级边界和恢复细节以
[reset_testee_historical_data/README.md](reset_testee_historical_data/README.md) 为准；回填工具的契约以
[HISTORICAL_BACKFILL_RUNBOOK.md](HISTORICAL_BACKFILL_RUNBOOK.md) 为准。

## 0. 先确认本次删除边界

本流程是**全环境级**操作，不支持按 Org、日期或部分 Testee 清理。执行后：

- 删除 QS 中全部 Testee，以及归属于 Testee 的 Entry、Plan、Task、AnswerSheet、Assessment、Outcome、Report、Statistics 和相关事件事实；
- 删除 IAM 中由 QS Testee 导出的 Profile 对应的 ProfileLink/Profile；
- 保留 QS 的医护人员、入口定义、Plan 定义、Questionnaire、Model、Norm、Report Template 和迁移水位；
- 保留 IAM 的 User、LoginIdentity、Credential、TokenAudit、Session/Outbox；
- 保留 MongoDB collection、validator 和索引，仅删除文档。

因此，这不是 IAM User 物理删除方案。若要求连 User/LoginIdentity/Credential 一并删除，立即停止，
另行完成引用审计和数据治理评审，不能扩写或绕过本手册中的 SQL。

以下任一项成立时不得开始：

- 目标数据库中存在必须保留的 Testee；
- 医护 User 与待删 Testee Profile 存在交叉 ProfileLink，且尚未人工处理；
- 不能同时停止三个存储的相关写入；
- QS MySQL、QS MongoDB、IAM MySQL 不能取得同一维护窗口备份；
- 不能完成备份恢复抽查；
- Staging 尚未用同一 revision、schema 和部署拓扑完成演练。

## 1. 固定批次、版本和操作记录

正式批次固定为：

```text
batch_id: hist-20250101-20260727-v1
timezone: Asia/Shanghai
from: 2025-01-01
to: 2026-07-27
days: 573
```

建立维护工单或执行记录，至少保存：

- 操作人、复核人、开始时间、维护窗口和回退决策人；
- 三个仓库 HEAD、构建产物 digest 和实际部署版本；
- QS MySQL、QS MongoDB、IAM MySQL 的库名、schema migration 水位；
- 目标 Org ID、冻结的 Plan、Questionnaire 和 Model 精确版本；
- 三份备份位置、校验值和恢复抽查结果；
- 删除前数量、导出清单 SHA-256、各阶段日志和最终验收报告。

正式批次必须使用一个新的、权限受限的 runner state dir。若该目录已经存在同 batch manifest，先停止：
确认这是应当继续的旧批次，还是本次清零后的全新重建。全新重建不能读取清零前的 checkpoint；旧目录
只能归档留证，不能删掉后谎称 resume。

在三个仓库分别执行并记录结果：

```bash
git status --short
git rev-parse HEAD
git log -1 --oneline
```

要求三个工作区干净；生产部署版本必须与记录一致。正式回填开始后不得修改或重新发布被冻结的
Plan、Questionnaire、Model。

## 2. 在 Staging 完成两轮演练

生产清理之前，先在 Staging 使用生产同构的 MySQL、Mongo Replica Set、Redis、Outbox 和 Worker：

1. 恢复一份脱敏的生产数据副本，并验证三个备份均可还原。
2. 完整执行本文第 4～8 步，确认医护和配置数据未变化。
3. 使用独立批次 ID 回填 `2025-01-01..2025-01-03`，覆盖四种 journey 和 Plan 场景。
4. 清理或重新恢复 Staging 后，再回填 `2025-01-01..2025-01-31`。
5. 执行 Statistics repair/validate/publish、K6 preflight/smoke 和一次中断后 `--resume`。

Staging 必须证明：

- 每日 checkpoint 只在完整验证后推进；
- Assessment 场景具有唯一 AnswerSheet、Assessment、Outcome、Report；
- Plan 子场景的 `task_open/task_complete` 完整；
- Report 超时、payload conflict 或版本漂移会停止当天；
- 医护、Questionnaire、Model、Norm、Template 和非目标事件不受影响；
- 恢复演练可以从同一时间点的三侧备份回到操作前状态。

两轮均通过后才签署生产 `GO`。Staging 的 batch ID 不得复用正式 batch ID。

## 3. 构建和只读预检

在各自仓库执行：

```bash
# qs-server
go test ./...
make docs-verify perf-verify
git diff --check

# seeddata-runner
go test ./...
go test -race ./...
go build -o tmp/bin/seeddata ./cmd/seeddata
git diff --check

# iam
go test ./...
git diff --check
```

再验证目标数据库 migration 没有 dirty 状态，并已包含当前历史回填结构。qs-server MySQL 至少为
`62`；实际生产执行前仍应与待发布二进制内 embedded migrations 的最新版本一致。

```sql
SELECT version, dirty FROM schema_migrations;
```

MongoDB 同样检查 migration 水位和 Replica Set primary 状态：

```javascript
db.schema_migrations.findOne({})
db.hello()
```

确认 `configs/seeddata.yaml` 的有效配置符合本批次：

```text
dailySimulation.countMin = 40
dailySimulation.countMax = 200
dailySimulation.workers = 5
IAM mock consumer workers = 1
timezone = Asia/Shanghai
```

所有 secret 只能从密钥系统注入，不得写入 YAML、manifest、日志或 shell 脚本。

## 4. 进入生产维护窗口

按以下顺序执行，不得交换：

1. 禁止新的 Testee 注册、Entry、Plan 和 AnswerSheet 流量。
2. 停止普通 seeddata daemon。
3. 暂时保持 Worker 和 Outbox relay 运行，等待 MySQL/Mongo Outbox、队列和消费者 backlog 全部归零。
4. 停止 apiserver、collection-server、Worker、Outbox relay、Plan scheduler、Statistics scheduler。
5. 确认没有 Testee 消费者、定时任务、运维脚本或人工连接继续写库。
6. 记录 Redis/消息队列中相关 key、stream、consumer group 的 pending 状态；旧消息不得在恢复服务后重放。
7. 对 QS MySQL、QS MongoDB、IAM MySQL 做同一维护窗口备份，并抽样恢复。

从第 5 步导出 ID 开始，到三库删除后验全部通过之前，不允许启动任何服务。

准备本地安全目录和连接变量。MySQL 使用权限为 `0600` 的 defaults file；URI、Token 和密码从
密钥系统注入，以下占位符不得原样执行：

```bash
umask 077
mkdir -p tmp/testee-data-reset

export QS_DB_NAME='<qs_database>'
export IAM_DB_NAME='<iam_database>'
export QS_MYSQL_CNF='/secure/qs-mysql.cnf'
export IAM_MYSQL_CNF='/secure/iam-mysql.cnf'
export QS_MONGO_URI='mongodb://<host>/<qs_mongo_database>?replicaSet=<replica_set>'
export QS_MONGO_DB='<qs_mongo_database>'
```

## 5. 采集删除前基线并导出 IAM 边界

记录 QS Testee、Profile 和 AnswerSheet 数量：

```bash
QS_TESTEE_COUNT="$(mysql --defaults-extra-file="$QS_MYSQL_CNF" "$QS_DB_NAME" --batch --skip-column-names --execute='SELECT COUNT(*) FROM testee')"
QS_PROFILE_COUNT="$(mysql --defaults-extra-file="$QS_MYSQL_CNF" "$QS_DB_NAME" --batch --skip-column-names --execute='SELECT COUNT(DISTINCT profile_id) FROM testee WHERE profile_id IS NOT NULL')"
QS_ANSWERSHEET_COUNT="$(mongosh "$QS_MONGO_URI" --quiet --eval='db.answersheets.countDocuments({})')"
```

导出 IAM 待删 Profile ID 和必须保护的医护 User ID：

```bash
mysql --defaults-extra-file="$QS_MYSQL_CNF" "$QS_DB_NAME" --batch --skip-column-names \
  --execute='SELECT DISTINCT profile_id FROM testee WHERE profile_id IS NOT NULL ORDER BY profile_id' \
  > tmp/testee-data-reset/iam-profile-ids.tsv

mysql --defaults-extra-file="$QS_MYSQL_CNF" "$QS_DB_NAME" --batch --skip-column-names \
  --execute='SELECT DISTINCT user_id FROM staff WHERE deleted_at IS NULL ORDER BY user_id' \
  > tmp/testee-data-reset/iam-healthcare-user-ids.tsv

IAM_PROFILE_COUNT="$(wc -l < tmp/testee-data-reset/iam-profile-ids.tsv | tr -d ' ')"
IAM_HEALTHCARE_USER_COUNT="$(wc -l < tmp/testee-data-reset/iam-healthcare-user-ids.tsv | tr -d ' ')"

test "$IAM_PROFILE_COUNT" = "$QS_PROFILE_COUNT"
shasum -a 256 \
  tmp/testee-data-reset/iam-profile-ids.tsv \
  tmp/testee-data-reset/iam-healthcare-user-ids.tsv
```

将以下只读证据另存到工单：

- 各 Testee-bearing 表和 Mongo collection 的行数；
- `staff`、`clinician`、`assessment_entry`、`assessment_plan` 的行数和 ID checksum；
- IAM User/LoginIdentity/Credential/TokenAudit 的行数；
- Mongo Questionnaire/Model/Norm/Template 的行数；
- 两侧共享 Outbox 的 event type 分布；
- 两份导出 TSV 的行数和 SHA-256。

任何基线查询失败、数量无法解释或导出数量不一致，立即 `NO-GO`。

## 6. 按固定顺序删除三库数据

### 6.1 QS MySQL

```bash
mysql --defaults-extra-file="$QS_MYSQL_CNF" \
  --show-warnings \
  --init-command="SET @qs_reset_confirm='DELETE_ALL_TESTEE_DATA', @qs_expected_database='${QS_DB_NAME}', @qs_expected_testee_count=${QS_TESTEE_COUNT}, @qs_expected_profile_count=${QS_PROFILE_COUNT}, @qs_delete_batch_size=1000, @qs_reset_resume=0" \
  "$QS_DB_NAME" \
  < scripts/oneoff/reset_testee_historical_data/01-reset-testee-facts-qs-mysql.sql \
  | tee tmp/testee-data-reset/qs-mysql-result.log
```

### 6.2 QS MongoDB

```bash
QS_RESET_CONFIRM='DELETE_ALL_TESTEE_DATA' \
QS_RESET_EXPECTED_DATABASE="$QS_MONGO_DB" \
QS_RESET_EXPECTED_ANSWERSHEET_COUNT="$QS_ANSWERSHEET_COUNT" \
QS_RESET_BATCH_SIZE=1000 \
QS_RESET_RESUME=0 \
mongosh "$QS_MONGO_URI" --quiet \
  --file scripts/oneoff/reset_testee_historical_data/02-reset-testee-facts-qs-mongo.js \
  | tee tmp/testee-data-reset/qs-mongo-result.log
```

### 6.3 IAM MySQL

先确认数据库策略允许 `LOCAL INFILE`：

```sql
SHOW GLOBAL VARIABLES LIKE 'local_infile';
```

只有结果为 `ON` 才执行：

```bash
mysql --defaults-extra-file="$IAM_MYSQL_CNF" \
  --local-infile=1 \
  --show-warnings \
  --init-command="SET @iam_reset_confirm='DELETE_EXPORTED_TESTEE_PROFILES', @iam_expected_database='${IAM_DB_NAME}', @iam_expected_profile_count=${IAM_PROFILE_COUNT}, @iam_expected_healthcare_user_count=${IAM_HEALTHCARE_USER_COUNT}, @iam_delete_batch_size=1000, @iam_reset_resume=0" \
  "$IAM_DB_NAME" \
  < scripts/oneoff/reset_testee_historical_data/03-reset-testee-profiles-iam-mysql.sql \
  | tee tmp/testee-data-reset/iam-mysql-result.log
```

若生产策略禁止 `LOCAL INFILE`，不得临时放宽；停止并改走经 DBA 审批的临时表导入方案。

### 6.4 删除阶段的中断规则

只有确认故障是纯连接中断、所有服务仍停止、数据库仍对应同一组备份、范围和预期行数未变化，才可
用原命令重跑并修改一个参数：

- QS MySQL：`@qs_reset_resume=1`
- QS MongoDB：`QS_RESET_RESUME=1`
- IAM MySQL：`@iam_reset_resume=1`

若发生 schema 不匹配、医护冲突、未知并发写入、跨存储数量不一致或删除范围疑问，不得 resume。
保持服务停止，从同一维护窗口的三侧备份整体恢复。

## 7. 验证清零结果和保护数据

QS MySQL 的核心 Testee 事实必须全部为零：

```sql
SELECT 'testee', COUNT(*) FROM testee
UNION ALL SELECT 'assessment', COUNT(*) FROM assessment
UNION ALL SELECT 'evaluation_outcome', COUNT(*) FROM evaluation_outcome
UNION ALL SELECT 'assessment_score', COUNT(*) FROM assessment_score
UNION ALL SELECT 'assessment_task', COUNT(*) FROM assessment_task
UNION ALL SELECT 'plan_enrollment', COUNT(*) FROM plan_enrollment
UNION ALL SELECT 'clinician_relation', COUNT(*) FROM clinician_relation
UNION ALL SELECT 'entry_resolve', COUNT(*) FROM assessment_entry_resolve_log
UNION ALL SELECT 'entry_intake', COUNT(*) FROM assessment_entry_intake_log;
```

还必须确认：

- Mongo 删除日志中所有目标 collection 的 `remaining` 为零；
- IAM 导出的 Profile/ProfileLink 均已删除；
- `staff`、`clinician`、`assessment_entry`、`assessment_plan` 的行数和 checksum 与基线一致；
- IAM User/LoginIdentity/Credential/TokenAudit 与基线一致；
- Mongo Questionnaire/Model/Norm/Template 与基线一致；
- 共享 Outbox 中 Questionnaire/AssessmentModel 和未知事件未被删除；
- Redis/消息队列不存在会重建旧 Testee 事实的 pending 消息。

任一保护断言失败时，不得启动服务或开始回填；三侧整体恢复。

## 8. 采集回填基线并开启历史能力

空状态验证通过后、创建第一条新事实前，采集 Statistics 基线：

```bash
export QS_MYSQL_DSN='<inject-from-secret-manager>'
go run ./scripts/oneoff/verify_historical_statistics \
  --mode capture-baseline \
  --org-id <org-id> \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --output /secure/path/hist-20250101-20260727-v1.baseline.json
```

在部署配置覆盖层开启受保护历史能力，不要把 secret 写回仓库。apiserver：

```yaml
historical_seed:
  enabled: true
  pause_plan_scheduler: true
  allowed_org_ids: [<org-id>]
  earliest_date: "2025-01-01"
  latest_date: "2026-07-27"
  timezone: Asia/Shanghai
  freshness: 5m
  secret_env: QS_HISTORICAL_CONTEXT_SECRET
```

collection-server 使用相同 Org、日期范围、时区、freshness 和 secret env。以一次性高强度随机值注入：

```bash
export QS_HISTORICAL_CONTEXT_SECRET='<inject-from-secret-manager>'
```

启动顺序：

1. IAM 及其正常依赖；
2. apiserver、collection-server；
3. Outbox relay；
4. Evaluation/Interpretation Worker；
5. 必要的 Statistics 查询服务。

独立 Plan scheduler、Statistics 定时 scheduler 和普通 seeddata daemon 保持停止。验证有效配置中
`enabled=true`、目标 Org 唯一、日期边界正确，并确认 Worker 能消费到 Report generated。

## 9. 执行 573 天历史回填

### 9.1 锁定源码并构建 seeddata-runner

在 serverA 的 seeddata-runner 仓库执行。正式批次只允许使用已提交、工作区干净且与
`origin/main` 一致的 revision；不要直接运行未记录源码对应的临时二进制：

```bash
cd /path/to/seeddata-runner

test -z "$(git status --porcelain)" || {
  echo "ERROR: seeddata-runner 工作区不干净"
  exit 1
}

git fetch origin
git switch main
git pull --ff-only origin main

seeddata_revision="$(git rev-parse HEAD)"
printf '%s\n' "$seeddata_revision" |
  tee /secure/path/hist-20250101-20260727-v1.seeddata-revision.txt

go version
env \
  -u IAM_USERNAME \
  -u IAM_PASSWORD \
  -u IAM_MOCK_CONSUMER_SHARED_SECRET \
  -u QS_HISTORICAL_CONTEXT_SECRET \
  GOTOOLCHAIN=go1.25.9 \
  go test ./...

mkdir -p tmp/bin
GOTOOLCHAIN=go1.25.9 go build -trimpath \
  -o tmp/bin/seeddata \
  ./cmd/seeddata

test -x tmp/bin/seeddata
sha256sum tmp/bin/seeddata |
  tee /secure/path/hist-20250101-20260727-v1.seeddata-binary.sha256
go version -m tmp/bin/seeddata |
  tee /secure/path/hist-20250101-20260727-v1.seeddata-build.txt
```

`go.mod` 当前要求 Go `1.25.9`。serverA 的基础 Go 可能低于该版本，而普通 `go version` 显示的是
自动选择后的模块工具链，因此这里显式固定 `GOTOOLCHAIN=go1.25.9`。测试进程同时移除运行期
Secret，避免环境变量污染负向配置测试。保存 revision、二进制 SHA-256 和 build metadata，后续
所有 `--resume` 必须继续使用同一 revision 构建的同一二进制。

### 9.2 准备 Secret 和持久化状态目录

使用持久化且受限访问的 state dir：

```bash
umask 077
mkdir -p /secure/path/seeddata-historical-state
test -z "$(find /secure/path/seeddata-historical-state -mindepth 1 -maxdepth 1 -print -quit)"
```

上面的空目录断言只适用于本次清零后的首次正式执行；后续 `--resume` 必须保留并复用其中的原文件。

```bash
export IAM_MOCK_CONSUMER_SHARED_SECRET='<inject-from-secret-manager>'
export QS_HISTORICAL_CONTEXT_SECRET='<same-one-time-secret-as-qs-server>'

# 现有 tenant/org 1 的 qs:admin 账号；用于取得可自动刷新的 QS 管理 API Token。
export IAM_USERNAME='<existing-qs-admin-username>'
read -r -s -p 'IAM_PASSWORD: ' IAM_PASSWORD
echo
export IAM_PASSWORD

test -n "$IAM_USERNAME"
test -n "$IAM_PASSWORD"
test -n "$IAM_MOCK_CONSUMER_SHARED_SECRET"
test -n "$QS_HISTORICAL_CONTEXT_SECRET"
```

`IAM_USERNAME/IAM_PASSWORD` 不是模拟 guardian 的身份，而是 runner 获取和刷新 QS 管理 API
Token 的现有账号。该账号必须属于 tenant/org `1`，处于 active 状态并具有 `qs:admin`；只具有
`qs:evaluation_plan_manager` 不能访问为任意 clinician 创建入口所需的 Org Admin 路径。

不要打印密码或两个 Secret 的明文。若 runner 与 qs-server 不在同一 shell 中运行，应从受限访问
的 Secret 文件或 Secret Manager 重新注入，不能依赖上一次 SSH 会话中的临时 shell 变量。

### 9.3 首次执行

仍在刚才完成构建的 seeddata-runner 仓库中执行：

```bash
tmp/bin/seeddata historical-backfill \
  --config configs/seeddata.yaml \
  --state-dir /secure/path/seeddata-historical-state \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --batch-id hist-20250101-20260727-v1
```

每日监控：

- 当天预期人数、四种 journey 分布和 Plan 60% 确定性选择；
- runner 日 checkpoint、manifest 和本地 stage ledger；
- qs-server `seed_backfill_stage` completed 事实和 attempt 失败原因；
- Mongo/MySQL Outbox backlog、Worker retry、Assessment/Outcome/Report 唯一性；
- MySQL/Mongo 容量、锁等待、复制延迟和错误率；
- 所有业务时间位于对应上海自然日且满足阶段顺序。

任一天存在 timeout、failed、conflict、版本漂移、阶段/资源 ID 缺失或时间异常时，命令必须非零停止，
且不得推进 checkpoint。先修复原因，再用完全相同的二进制、batch、日期范围、配置、冻结版本和
state dir：

```bash
tmp/bin/seeddata historical-backfill \
  --config configs/seeddata.yaml \
  --state-dir /secure/path/seeddata-historical-state \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --batch-id hist-20250101-20260727-v1 \
  --resume
```

不得删除 ledger、修改 manifest、变更 idempotency key 或换 batch ID 绕过 conflict。

## 10. 验证 runner 和服务端事实一致

回填命令成功退出后，在 seeddata-runner 仓库执行：

```bash
tmp/bin/seeddata historical-manifest \
  --state-dir /secure/path/seeddata-historical-state \
  --batch-id hist-20250101-20260727-v1 \
  > /secure/path/hist-20250101-20260727-v1.manifest.json

tmp/bin/seeddata historical-verify \
  --config configs/seeddata.yaml \
  --state-dir /secure/path/seeddata-historical-state \
  --batch-id hist-20250101-20260727-v1 \
  > /secure/path/hist-20250101-20260727-v1.runner-verification.json
```

必须同时满足：

- `HistoricalVerification.Complete=true`；
- `CompletedThrough=2026-07-27`；
- 每个预期场景的本地、manifest、服务端资源 ID 一致；
- `register_only/create_testee/resolve_entry/submit_answer` 停止位置正确；
- Assessment 场景具有 completed 的 AnswerSheet、Assessment created/submitted、Outcome、Report；
- Plan 子场景具有 completed 的 task open/complete；
- 没有 payload/version conflict、未来业务时间或倒序时间。

任何一项不满足都不能进入 Statistics。

## 11. 修复、校验并发布 Statistics

在 qs-server 仓库执行统一编排。Token 从密钥系统注入：

```bash
export QS_STATISTICS_TOKEN='<inject-from-secret-manager>'
go run ./scripts/oneoff/rebuild_statistics \
  --base-url https://<qs-host> \
  --org-ids <org-id> \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --window-days 31 \
  --reason hist-20250101-20260727-v1 \
  --mode historical-backfill \
  --confirm
```

命令应完成：

1. 将 573 天拆成 19 个不超过 31 天的窗口；
2. 每个窗口依次执行 `repair -> validate`；
3. 对 `2026-07-27` 到执行时最新完整上海自然日继续 catch-up；
4. 仅对最新完整上海自然日 publish，并校验最终水位。

随后与删除后的基线逐日对账：

```bash
go run ./scripts/oneoff/verify_historical_statistics \
  --mode verify \
  --org-id <org-id> \
  --batch-id hist-20250101-20260727-v1 \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --baseline /secure/path/hist-20250101-20260727-v1.baseline.json \
  --output /secure/path/hist-20250101-20260727-v1.statistics-verification.json
```

要求无 fact conflict，逐日阶段数与 Statistics 增量一致，publish 水位等于执行时最新完整上海自然日。

## 12. 恢复调度并执行 K6 验收

先恢复 Plan scheduler 和 Statistics scheduler，但普通 seeddata daemon 暂不恢复。确认没有历史日期任务被
误过期或重复生成后，在 qs-server 仓库准备 K6：

```bash
make perf-init
```

检查并补齐本机私有文件：

- `tmp/perf/qs-perf.config.json`：目标 URL、paths、qpsProfiles、token 文件和查询配置；
- `tmp/perf/iam-users.json`：医护/Collection 和 apiserver 的真实验收账号；
- `autoDiscoverSeeddata` 必须为 `true`；不需要手工配置固定 `ASSESSMENT_IDS` 或 `TESTEE_IDS`；
- 若配置模板有更新，先运行 `make perf-sync-profiles`，再人工核对本地覆盖项。

全量清理会删除 K6 Collection 账号原有的 Testee/ProfileLink，但保留其 IAM 登录身份。因而第一次运行前，
需要在本地私有配置中暂时设置 `autoCreateSubmitTestees: true`，允许 K6 账号通过正常 API 为自己创建
Testee。该设置只写入 `tmp/perf/qs-perf.config.json`，不得修改或提交 example 配置。

刷新 Token 并先执行连接/样本初始化：

```bash
make perf-tokens
make perf-preflight
make perf-smoke
```

`perf-preflight` 只验证 Token、模型、问卷、Testee 列表和 Statistics API，不负责发现 Report。
Report 样本在 `perf-smoke` 的 setup 阶段发现；第一次 smoke 可能先创建 K6 Testee 和新的 Assessment，
因此这次只作为 bootstrap，不能作为最终非 degraded 验收。等待本轮 Assessment 全部生成 Report 后，
将 `autoCreateSubmitTestees` 改回 `false`，再次执行：

```bash
make perf-preflight
make perf-smoke
```

通过标准：

- preflight 的 Token 和基础 API 检查全部为 2xx；
- 第二次 smoke 自动发现 K6 账号有权访问的 Testee、Assessment 和 generated Report；
- 第二次 smoke 不再出现 `no-report degraded`；
- smoke 中 AnswerSheet 提交、Assessment/Outcome/Report 和查询路径全部成功；
- 没有历史能力影响普通、未签名 REST/gRPC/K6 请求的证据。

若第二次 smoke 仍找不到 Report，不要人工塞 ID 绕过：检查 K6 Collection 账号的 active ProfileLink、
Testee 归属、上一轮 Assessment/Report 终态和 setup discovery 日志；历史批次的 runner/server/manifest 或
Statistics 不一致时，再回到第 10～11 步处理。

## 13. 关闭一次性历史能力

所有验收通过后：

1. 停止 historical-backfill 相关进程，确认不存在 pending 历史请求。
2. 将 apiserver/collection-server 的 `historical_seed.enabled` 改回 `false`。
3. 将 apiserver 的 `pause_plan_scheduler` 恢复为正常值。
4. 重新部署并验证没有历史头的普通请求行为不变，携带历史头的请求已被拒绝。
5. 从运行环境移除并轮换 `QS_HISTORICAL_CONTEXT_SECRET`。
6. 按正常策略决定是否恢复普通 seeddata daemon。
7. 保存 manifest、stage ledger、stage/attempt、Statistics 基线和验证结果、删除日志、备份及工单至少 90 天。

不要在同一个维护窗口立即删除 qs-server 的历史表、Historical Context 代码或 runner resume 能力。
至少经过 90 天观察期，并确认不再需要回滚/对账后，再按
[HISTORICAL_DATA_REBUILD_AND_RETIREMENT_PLAN.md](HISTORICAL_DATA_REBUILD_AND_RETIREMENT_PLAN.md)
另开生产代码退场变更。退场变更必须独立测试、迁移和发布，不能与数据重建混在一次操作中。

## 14. 统一停止和回退判定

以下任一情况立即停止，不进入下一阶段：

- 数据库名、基线数量、导出 hash、manifest、payload 或版本不一致；
- 医护与待删 Testee Profile 发生引用冲突；
- 删除脚本的保护断言失败或三侧删除结果不一致；
- 必需服务端阶段/资源 ID 缺失，Report 未 generated，Plan task 未 completed；
- 业务时间越界、倒序或落入未来；
- Outbox/Worker backlog 无法排空或出现非幂等重复；
- Statistics repair/validate/publish 或 K6 非 degraded 验收失败。

回退原则：

- 删除阶段发生无法安全 resume 的问题：保持全部服务停止，恢复同一时间点的 QS MySQL、QS MongoDB、IAM MySQL；
- 回填阶段需要撤销正式 batch：先停止 runner、worker、relay 和 scheduler，使用
  `cleanup_perf_testee_data` 的 batch + manifest 模式执行 dry-run，核对 receipt v2 后再 apply；
- 禁止按整个 Org、模糊日期或 `--mongo-only` 回滚历史批次；
- 跨存储回滚未全部完成前保留 `seed_backfill_stage`、attempt 和 rollback operation；
- 任何部分成功状态都不得直接恢复外部写流量。

## 15. 最终签字清单

只有以下各项全部为“是”，本次重建才算完成：

- [ ] Staging 三天、整月、resume、备份恢复演练通过。
- [ ] 三库 revision、部署 digest、migration 水位已冻结并留档。
- [ ] 三侧同时间点备份已完成恢复抽查。
- [ ] QS Testee 事实和 IAM Testee Profile/ProfileLink 已按脚本清零。
- [ ] 医护、配置、迁移、索引和非目标事件与基线一致。
- [ ] 正式批次 573 天全部完成且 runner/server/manifest 一致。
- [ ] Assessment、Outcome、Report 和 Plan 必需阶段唯一且完整。
- [ ] Statistics 19 个历史窗口、catch-up、publish 和逐日对账通过。
- [ ] `make perf-preflight` 与 `make perf-smoke` 使用真实 Report 样本通过。
- [ ] Historical Context 已关闭，密钥已轮换，正常 scheduler 已恢复。
- [ ] 备份、日志、manifest、ledger、attempt 和验收输出已设置至少 90 天保留期。
