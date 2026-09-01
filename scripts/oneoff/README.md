# One-off 与运维脚本

本目录只保留仍可用于当前代码和数据契约的工具。脚本进入这里不代表可以直接在生产执行；
任何写操作都必须先 dry-run、备份并核对目标环境。

## 执行原则

1. 默认使用只读或 dry-run 模式；写入必须有明确的 `--apply`、`--confirm` 等开关。
2. 生产执行前确认脚本对应的 schema、模型版本和问卷版本仍与目标数据一致。
3. MySQL 与 MongoDB 共同参与的操作必须成对备份、成对恢复，不能只恢复一侧。
4. 禁止把真实 DSN、Token 或密码写入命令历史、文档或提交；统一使用环境变量。
5. 一次性任务完成后删除专用脚本；需要追溯时从 Git 记录获取，不能直接复制旧脚本到生产。

## 当前保留清单

### 验收与只读审计

| 工具 | 用途 | 写数据 |
| --- | --- | --- |
| `verify_definition_v2_cutover` | ModelCatalog current-only 数据与契约审计 | 否 |
| `smoke_modelcatalog_cutover` | 完成 AnswerSheet → Assessment → Outcome → Report 部署 smoke | 是，创建 smoke 事实 |
| `smoke_modelcatalog_revision_conflict` | 验证 Model/Questionnaire revision conflict 的 REST 409 映射 | 是，恢复测试字段 |
| `audit_norm_usage` | Norm 反向引用、悬空引用和人口学常模审计 | 否 |
| `audit_ai_explanation_prompt_evaluation_size` | AI 解读 Prompt Evaluation Run BSON 与 generation/semantic 输出尺寸分布 | 否 |
| `audit_evaluation_p1_evidence.sql` | Evaluation P1 证据查询 | 否 |
| `audit_evaluation_p2_evidence.sql` | Evaluation P2 证据查询 | 否 |
| `observe_outbox_by_event_type` | Outbox 事件类型观测 | 否 |
| `cleanup_orphaned_assessment_documents` | **blocked / audit-only**：只允许 dry-run 对账缺少 MySQL Assessment 引用的 Mongo AnswerSheet 候选；候选结果不是删除授权 | 否；禁止 `--apply` |

`audit_ai_explanation_prompt_evaluation_size` 本地可使用 `MONGO_URI`；受保护生产环境通过 Database Operations 的 `audit-ai-evaluation-size` operation 传入分离式 Mongo 环境变量，在 `infra-network` 的只读临时容器中执行 `--max-runs=0` 全量扫描。工作流不得打印 URI、用户名或密码。

`cleanup_orphaned_assessment_documents` 当前必须保持 blocked。不得使用 `--apply`、
`--hard-delete` 或 `--skip-backup`；代码中存在这些开关只代表历史实现能力，不代表当前运维授权。
解除 blocked 至少要同时满足：可靠受理状态机已定义可删除终态；完成 MySQL Assessment、Mongo
AnswerSheet、提交幂等/Outbox 以及下游 Outcome/Report 的反向引用审计；形成不可变 ID 清单、
成对备份和恢复演练；由数据责任人与发布责任人独立批准；并以专用负向测试和人工复核证明不会
删除仍待异步创建 Assessment 的合法受理事实。任一条件未满足时只能输出 dry-run 审计结果。

### 可重复的运维修复

| 工具 | 用途 |
| --- | --- |
| `cleanup_perf_testee_data` | 按显式 Testee ID dry-run、备份并且只清理 `origin_type=adhoc` 临时测评数据 |
| `select_seeddata_duplicate_testees` | 只读识别 seeddata 重试产生的重复 Testee，并生成显式 Testee/Profile 清单 |
| `rebuild_statistics` | 通过受保护 Run API 执行 validate、repair 或 publish |
| `repair_stranded_plan_tasks` | 审计、修复、验证及 CAS 回滚历史 stale pending 与 Task due_at |
| `enroll_testees_after_date.py` | 按时间范围补录受试者关系 |
| `govern_interpretation_presentation_profiles` | 以不可变 Artifact dimensions 和受保护 manifest 治理历史 presentation profile |
| `govern_interpretation_template_releases` | 为历史 ReportTemplate release 物化 Builder/schema manifest 并补齐 Enneagram 发布身份 |
| `govern_interpretation_model_template_routes` | 以新不可变 ModelCatalog release 切换显式 TemplateID/TemplateVersion，并固化新旧 DefinitionV2 哈希 |
| `govern_evaluation_outcome_template_routes` | 以可回滚 manifest 显式化历史 EvaluationOutcome 冻结输入中的模板路由 |

具体参数以各命令的 `--help`、相邻 README 和测试为准。

### 仅限全新环境初始化

| 工具 | 限制 |
| --- | --- |
| `seed_brief2` | 内置因子映射只绑定 `gXkk9W@4.0.1` |
| `seed_spm_sensory` | 内置因子映射只绑定 `bJFKi3@4.0.1` |

这两个 seed 只能用于问卷版本与内置映射完全一致的全新环境，不能覆盖现有 Model、
Questionnaire 或 Norm。生产修复必须从当前发布快照导出真实映射，经校验后走正常导入和发布链路。

## 仓库验证

```bash
go test -count=1 ./scripts/oneoff/...
python3 scripts/check_docs_hygiene.py
python3 scripts/check_docs_facts.py
git diff --check
```
