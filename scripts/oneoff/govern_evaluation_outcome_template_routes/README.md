# govern_evaluation_outcome_template_routes

将 MySQL `evaluation_outcome.report_input_json` 中依赖运行时默认值的历史报告模板路由显式化。

治理只处理 schema v3 冻结输入：

- `scale` / `behavioral_rating` / `cognitive` 缺失整个 `ReportSpec.Sections` 时，只允许使用同一 `outcome_id` 的唯一 active `legacy-v1` Artifact 中已冻结的 `presentation_profile` 等价物化 `factor_scores` section；
- 非 typology 报告缺失路由时显式写入 `standard@legacy-v1`；
- typology 必须已经冻结一个可识别的 `template_id`，缺失版本显式写入 `legacy-v1`；
- 已显式冻结 `legacy-v1` 或 `2026-08-v1` 的 Outcome 不变；
- Artifact 关联不唯一、模型身份不一致、profile 未冻结、factor code 不属于同一 Outcome 的 `factor_catalog`、冲突或非法 JSON 都会阻断 audit，不做猜测。

流程：

1. `audit` 先只读扫描 `interpret_report_artifacts`，再流式扫描 Outcome，生成包含原始/目标语义哈希、Artifact 指纹与字段级回滚信息的 manifest；
2. `apply` 按 manifest 分批事务、主键和原文 CAS 回填，支持失败后幂等续跑；
3. `verify` 校验 manifest 目标并可要求全表零缺口；
4. `rollback` 按字段级原始状态恢复等价 JSON 语义。

确认词：

```text
materialize-outcome-template-route-legacy-v1
rollback-outcome-template-route-legacy-v1
```

manifest 不保存完整 `report_input_json`，不会复制历史冻结报告内容；生产 manifest 和数据库凭据不得提交。

治理同时需要 MySQL 与 MongoDB 只读凭据。MySQL 是唯一写入目标；MongoDB Artifact 只作为不可变等价来源，apply/verify 会重新核验 Artifact 指纹，rollback 不依赖 MongoDB 当前状态。
