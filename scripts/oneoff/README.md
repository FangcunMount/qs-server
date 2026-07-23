# One-off 与运维脚本

本目录只保留仍可用于当前代码和数据契约的工具。脚本进入这里不代表可以直接在生产执行；任何写操作都必须先 dry-run、备份并核对目标环境。

## 执行原则

1. 默认使用只读或 dry-run 模式；写入必须有明确的 `--apply`、`--confirm` 等开关。
2. 生产执行前确认脚本对应的 schema、模型版本和问卷版本仍与目标数据一致。
3. MySQL 与 MongoDB 共同参与的操作必须成对备份、成对恢复，不能只回滚一侧。
4. 禁止把真实 DSN、Token 或密码写入命令历史、文档或提交；统一使用环境变量。
5. 历史内容修订脚本执行完成后应从当前分支删除，需要追溯时从 Git 历史获取。

## 当前保留清单

### 验收与只读审计

| 工具 | 用途 | 写数据 |
| --- | --- | --- |
| `verify_definition_v2_cutover` | ModelCatalog G5/current-only 数据与契约审计 | 否 |
| `smoke_modelcatalog_cutover` | 按 testee 与 model codes 串行完成 AnswerSheet → Assessment → Outcome → Report 部署 smoke | 是，创建 smoke 事实 |
| `smoke_modelcatalog_revision_conflict` | 对专用未发布草稿并发更新，验证 Model/Questionnaire revision conflict 的 REST 409 映射并恢复 basic-info | 是，递增草稿 revision 后恢复字段 |
| `audit_norm_usage` | Norm 反向引用、悬空引用和人口学常模审计 | 否 |
| `audit_evaluation_p1_evidence.sql` | Evaluation P1 证据查询 | 否 |
| `audit_evaluation_p2_evidence.sql` | Evaluation P2 证据查询 | 否 |
| `observe_outbox_by_event_type` | Outbox 事件类型观测 | 否 |

`verify_definition_v2_cutover` 的退出码保持为：`0` 表示通过，`1` 表示证据不可用，`2` 表示发现违规。生产验收必须同时提供 MySQL 与 MongoDB 连接，不能把缺少一侧连接当作通过。

### 可重复的运维修复

| 工具 | 用途 |
| --- | --- |
| `cleanup_perf_testee_data` | 按明确范围清理压测受试者数据 |
| `cleanup_orphaned_assessment_documents` | 对账并清理缺少 MySQL Assessment 的 Mongo 报告/答卷 |
| `rebuild_statistics` | 通过受保护 Run API 校验、修复或重建 Statistics |
| `rewrite_seeddata_assessment_times` | 修正种子测评时间 |
| `enroll_testees_after_date.py` | 按时间范围补录受试者关系 |

这些工具不是“执行一次就永久完成”的迁移，它们保留是因为故障恢复、环境重建或受控数据修复仍可能复用。具体参数以各命令的 `--help`、相邻 README 和测试为准。

### 当前维护窗口专用

| 工具 | 用途 | 生命周期 |
| --- | --- | --- |
| `repair_modelcatalog_cutover` | 在历史事实清零后，以当前 Handler 和 active snapshot 原子规范化线上 Model runtime；不覆盖 draft head | G5 严格关闭并保存证据后删除 |

该工具默认 dry-run，只有所有 MySQL/Mongo 历史清零、Norm、DefinitionV2、精确 Questionnaire 绑定、migration 和索引检查全部通过，才允许显式 `--apply`。它不修 Norm、不猜字段、不兼容旧问卷版本；操作细节见相邻 README。

### 仅限全新环境初始化

| 工具 | 限制 |
| --- | --- |
| `seed_brief2` | 内置因子映射只绑定 `gXkk9W@4.0.1` |
| `seed_spm_sensory` | 内置因子映射只绑定 `bJFKi3@4.0.1` |

这两个 seed 只能用于问卷版本与内置映射完全一致的全新环境。它们**不能**用于修复当前生产中的 `gXkk9W@7.0.1` 或 `bJFKi3@6.0.1`，也不能使用 `--force` 覆盖现有 Model、Questionnaire 或 Norm。生产修复必须从当前发布快照导出真实 Factor/Question 映射，经校验后走正常导入和发布链路。

## 已退役脚本

以下脚本已从当前工作树删除，需要审计时从 Git 历史查看：

- ModelCatalog legacy identity、payload、projection、DefinitionV2 迁移和旧软删除脚本；
- Interpretation lifecycle/report catalog 历史回填脚本；
- retry governance 历史失败运行回填脚本；
- scale category 单次分类回填脚本；
- MBTI OEJTS 单次题干更新脚本；
- 已被 `verify_definition_v2_cutover` 覆盖的旧 identity/Evaluation 审计 SQL；
- 指向旧问卷版本和已删除工具的 BRIEF-2/SPM/SBTI 维护手册。
- Statistics V1 的 Journey/Episode/Footprint 回填、漏斗重建和孤儿清理脚本。

退役脚本不得从 Git 历史直接复制到生产运行。若相同需求再次出现，应先按当前 schema 和业务契约重新实现、测试和评审。

## 历史测评事实清理边界

如果业务确认所有历史答卷、测评、结果和报告都可以删除，可以做一次“测评事实层重置”，但不能只清空 `assessment` 或某一个 Mongo collection。

### 必须保留

- MongoDB：`questionnaires`、`assessment_models`、`assessment_norms`、`interpretation_report_templates`、`schema_migrations`，以及其他字典、配置和迁移集合；
- MySQL：组织、账号、受试者、计划、配置、迁移版本等非测评事实；
- 备份文件、当前二进制版本、发布版本和执行前审计结果。

### 需要作为一个整体核对的测评事实

- MySQL 核心事实：`assessment_score`、`evaluation_outcome`、`runtime_checkpoint`、`assessment`；
- MySQL 派生事实：与 Assessment 关联的统计 Fact、Daily/Snapshot、journey/episode/footprint 和待处理事件；
- MongoDB 答卷事实：`answersheets`、`answersheet_submit_idempotency`；
- MongoDB 解释事实：`report_generations`、`interpretation_runs`、`interpret_report_artifacts`、`report_query_catalog`、`archived_reports`、`interpretation_admission_failures`，以及环境中仍存在的旧报告集合；
- 两侧 Outbox：只清理明确属于 AnswerSheet、Assessment、Evaluation、Interpretation 的事件，不能无条件清空共享 `domain_event_outbox`。

上面的名称是代码中的当前事实入口，不是可直接复制执行的清库 SQL。生产库在执行前仍需用 `SHOW CREATE TABLE`、外键查询、`listCollections` 和事件类型分布生成该环境的精确清单。

### 安全顺序

1. 停止答卷提交、Collection Worker、Evaluation/Interpretation Worker、Outbox Relay 和统计同步，确认没有在途任务。
2. 成对备份 MySQL、MongoDB 和上一版本二进制，并校验备份可恢复。
3. 记录每张表/集合的清理前数量；先清子事实和派生事实，最后清 `assessment`。
4. 不关闭外键检查强行 `TRUNCATE`；优先按依赖顺序执行可审计的 `DELETE`。共享 Outbox 必须按事件类型和业务 ID 删除。
5. 清理后重建 Assessment 派生统计/缓存，重新运行 cutover audit。
6. 恢复服务后只创建新的 smoke 数据，验证 AnswerSheet → Assessment → Outcome → Report 全链路，再开放写流量。

跨 MySQL/MongoDB 的清理没有单个原子事务。如果任一步失败，必须停止服务并使用同一时间点的两侧备份恢复，不能带着“只清了一半”的数据继续运行。

## 仓库验证

```bash
go test -count=1 ./scripts/oneoff/...
python3 scripts/check_docs_hygiene.py
python3 scripts/check_docs_facts.py
git diff --check
```
