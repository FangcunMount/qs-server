# 全环境 Testee 历史数据重置

这组脚本用于一次性删除目标环境中的**全部 Testee 及其业务事实**，同时保留医护人员、入口、
计划定义、问卷、模型、常模和报告模板。它只服务于随后执行的历史 seeddata 重建。

> 这不是按 Org 清理工具。只要目标数据库中存在不应删除的 Testee，就不能运行本目录脚本。
> 本方案不适用于按 Org、日期或部分 Testee 清理；这类需求继续使用精确资源清理工具。

## 1. 脚本与边界

| 顺序 | 文件 | 作用 |
| --- | --- | --- |
| 1 | `01-reset-testee-facts-qs-mysql.sql` | 分批删除 QS Testee、业务事实、Statistics 和 Testee 事件控制数据 |
| 2 | `02-reset-testee-facts-qs-mongo.js` | 分批删除 AnswerSheet、Report 及相关 Mongo 生命周期数据 |
| 3 | `03-reset-testee-profiles-iam-mysql.sql` | 仅按 QS 导出的 Profile ID 删除 IAM ProfileLink/Profile |

QS 和 IAM MySQL 脚本都使用数据库级 advisory lock、显式确认值、预期数据库名、预期行数、
固定批次删除和执行后断言。Mongo 脚本保留 collection 和索引，只调用 `deleteMany`。

共享事件表不是无条件清空：只删除当前事件目录中的 AnswerSheet、Evaluation、Interpretation 和
Plan 事件，保留 Questionnaire/AssessmentModel 事件以及未来出现的未知事件类型。

IAM 默认保留：

- `users`
- `auth_login_identities`
- `auth_credentials`
- `auth_token_audit`
- IAM Outbox 和 Session revoke Outbox

如果部署环境仍保留旧 `children`/`guardianships` 桥接表，IAM 脚本会按同一 Profile ID 删除对应
旧行，避免运行时 Profile 已删除但旧历史副本仍保留。

## 2. 强制维护窗口

1. 先禁止新流量，并停止普通 seeddata daemon。
2. 保持 Worker/Outbox relay 运行，直到 MySQL/Mongo Outbox 和消息消费者 backlog 全部归零。
3. 停止 apiserver、collection-server、Worker、Outbox relay、Plan scheduler、Statistics scheduler。
4. 确认没有数据库写连接或消息消费者仍在处理 Testee 事件。
5. 对 QS MySQL、QS MongoDB、IAM MySQL 做同一维护窗口备份，并完成恢复抽查。

从导出 ID 开始到三库后验全部通过之前，**不要启动任何服务**。数据库脚本无法替代对 Redis、
消息队列和运行进程的停机确认；如果 broker 中仍有旧消息，服务恢复后可能重新创建已删除事实。

## 3. 准备连接与只读基线

以下命令都从 qs-server 仓库根目录执行。MySQL 凭据使用权限为 `0600` 的
`--defaults-extra-file`，不要把密码写入命令或仓库。

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

记录三项 QS 基线：

```bash
QS_TESTEE_COUNT="$(mysql --defaults-extra-file="$QS_MYSQL_CNF" "$QS_DB_NAME" --batch --skip-column-names --execute='SELECT COUNT(*) FROM testee')"
QS_PROFILE_COUNT="$(mysql --defaults-extra-file="$QS_MYSQL_CNF" "$QS_DB_NAME" --batch --skip-column-names --execute='SELECT COUNT(DISTINCT profile_id) FROM testee WHERE profile_id IS NOT NULL')"
QS_ANSWERSHEET_COUNT="$(mongosh "$QS_MONGO_URI" --quiet --eval='db.answersheets.countDocuments({})')"
```

导出 IAM 删除边界和医护 User 保护清单：

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
shasum -a 256 tmp/testee-data-reset/iam-profile-ids.tsv tmp/testee-data-reset/iam-healthcare-user-ids.tsv
```

必须把行数、SHA-256、三库备份位置、三个数据库名和三个仓库 HEAD 保存到维护记录中。

## 4. 执行 QS MySQL

```bash
mysql --defaults-extra-file="$QS_MYSQL_CNF" \
  --show-warnings \
  --init-command="SET @qs_reset_confirm='DELETE_ALL_TESTEE_DATA', @qs_expected_database='${QS_DB_NAME}', @qs_expected_testee_count=${QS_TESTEE_COUNT}, @qs_expected_profile_count=${QS_PROFILE_COUNT}, @qs_delete_batch_size=1000, @qs_reset_resume=0" \
  "$QS_DB_NAME" \
  < scripts/oneoff/reset_testee_historical_data/01-reset-testee-facts-qs-mysql.sql \
  | tee tmp/testee-data-reset/qs-mysql-result.log
```

脚本在最后删除 `testee`，并验证 `staff`、`clinician`、`assessment_entry`、`assessment_plan` 的
行数和 ID checksum 与本次连接的执行前基线一致。

## 5. 执行 QS MongoDB

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

脚本不执行 `drop()` 或 `dropDatabase()`，因此 collection validator、索引和 migration 水位保留。

## 6. 执行 IAM MySQL

IAM 脚本从固定的本地文件名读取前面导出的 ID，必须启用 `--local-infile=1`：

```bash
mysql --defaults-extra-file="$IAM_MYSQL_CNF" \
  --local-infile=1 \
  --show-warnings \
  --init-command="SET @iam_reset_confirm='DELETE_EXPORTED_TESTEE_PROFILES', @iam_expected_database='${IAM_DB_NAME}', @iam_expected_profile_count=${IAM_PROFILE_COUNT}, @iam_expected_healthcare_user_count=${IAM_HEALTHCARE_USER_COUNT}, @iam_delete_batch_size=1000, @iam_reset_resume=0" \
  "$IAM_DB_NAME" \
  < scripts/oneoff/reset_testee_historical_data/03-reset-testee-profiles-iam-mysql.sql \
  | tee tmp/testee-data-reset/iam-mysql-result.log
```

DBA 必须先只读确认 `SHOW GLOBAL VARIABLES LIKE 'local_infile'` 为 `ON`；若生产策略禁止
`LOCAL INFILE`，不要临时绕过策略，应改走经审批的临时表导入流程并重新评审脚本。

如果待删 Profile 曾与 `staff.user_id` 清单中的医护 User 建立任何 ProfileLink，脚本会先输出冲突
ID，再以非零错误停止；此时不执行任何 IAM 删除。冲突必须人工判断，不能绕过保护断言。

## 7. 中断恢复

三个脚本都按固定批次提交。连接中断时，已完成批次不会回滚；在确认错误只是连接中断、数据库
仍是同一份备份后的目标且所有服务仍停止后，使用原始预期行数重新运行，并把对应 resume 值改为
`1`：

- QS MySQL：`@qs_reset_resume=1`
- QS MongoDB：`QS_RESET_RESUME=1`
- IAM MySQL：`@iam_reset_resume=1`

如果错误涉及表结构、范围、医护冲突、未知写入或跨存储不一致，不得使用 resume；停止操作并从
同一维护窗口备份恢复三侧。

MySQL client 在存储过程抛错后会停止读取文件，因此失败连接可能暂时留下 `_oneoff_*` helper
procedure。保持服务停止，先保存错误输出；决定恢复备份或 resume 后，成功重跑会自动删除这些
helper。若决定放弃操作，则由 DBA 在核对名称后显式 `DROP PROCEDURE`，不得遗留到维护窗口之外。

## 8. 最终只读验证

QS MySQL 应为零：

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

QS MongoDB 的脚本输出中所有 `remaining` 必须为零。IAM 验证目标 ID 已删除：

```sql
-- 在同一个 mysql client 中先创建/加载临时 ID 表后执行：
SELECT COUNT(*) FROM profiles p JOIN tmp_reset_testee_profile_ids x ON x.profile_id = p.id;
SELECT COUNT(*) FROM profile_links l JOIN tmp_reset_testee_profile_ids x ON x.profile_id = l.profile_id;
```

同时对照执行前记录确认：

- `staff`、`clinician`、`assessment_entry`、`assessment_plan` 未变化；
- IAM User/LoginIdentity/Credential/TokenAudit 未变化；
- Mongo Questionnaire/Model/Norm/Template 未变化；
- 共享 Outbox 中 Questionnaire/AssessmentModel 或未知事件未被删除。

全部通过后才能开始 seeddata 历史回填。在回填、Statistics repair/validate/publish 和 K6 验收完成
前，不能删除 qs-server 或 seeddata-runner 的 Historical Context、stage、attempt 和 resume 能力。
