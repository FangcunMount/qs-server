# Interpretation 历史 Presentation 与 Template 治理台账

> 状态：已完成并签署（2026-08-06）
>
> 生产只读基线：[Database Operations 31075388732](https://github.com/FangcunMount/qs-server/actions/runs/31075388732)
>
> 最终数据核验：[Database Operations 31103985240](https://github.com/FangcunMount/qs-server/actions/runs/31103985240)
>
> 严格运行时代码：`f63e273a`；准入失败持久化修复：`b6407f40`
>
> 生产部署：[Production Deploy 31106231761](https://github.com/FangcunMount/qs-server/actions/runs/31106231761)，attempt 2 完整成功

## 1. 结论

本轮已把 Interpretation 的历史展示与模板路由从“运行时猜测”收敛为“历史事实自描述”：

- 36,860 份历史 Artifact 已由自身不可变 `dimensions` 等价物化 `presentation_profile`，未读取当前 ModelCatalog，也未重算历史报告；
- 5 个 TemplateID 的历史版和当前版共 10 个发布版本已具备精确 Builder、ContentSchema 与 manifest；
- 24 个 active ModelCatalog snapshot 已显式冻结 TemplateID/TemplateVersion；
- 212,586 份历史 Outcome 已冻结精确模板路由，最终 213,150 份 Outcome 全部显式、候选为 0；
- 运行时已删除动态 presentation 解析、默认模板和 `legacy-v1` 猜测，缺失或不一致数据改为 fail-closed；
- `legacy-v1` 仍作为真实历史版本身份保留，不是待删除兼容别名。

## 2. Presentation 治理

### 2.1 基线与执行结果

| 阶段 | 生产证据 | 结果 |
| --- | --- | --- |
| 只读分类 | [31075388732](https://github.com/FangcunMount/qs-server/actions/runs/31075388732) | active 212,457；缺 profile 36,860；全部可由 Artifact 自身 dimensions 等价物化；ambiguous、无 dimensions、非法形态均为 0 |
| 全库备份与验证 | `qs_mongodb_backup_20260806_140515.archive.gz`；[31086706136](https://github.com/FangcunMount/qs-server/actions/runs/31086706136) | 约 253 MiB；归档读取验证成功 |
| 精确回填与核验 | [31089462135](https://github.com/FangcunMount/qs-server/actions/runs/31089462135) | 36,860/36,860 完成；目标缺口、异常与正文变化均为 0 |
| 独立状态复核 | [31100163871](https://github.com/FangcunMount/qs-server/actions/runs/31100163871) | active artifact 213,109；缺 profile/builder/content schema 均为 0；`frozen` 176,249，`legacy_artifact_dimensions/v1` 36,860 |

### 2.2 等价物化规则

```text
presentation_profile.visible_factor_codes
  = dimensions[].factor_code 按原顺序去空、去重

presentation_profile.source
  = legacy_artifact_dimensions/v1
```

该 source 是已物化事实的来源标记，仍属于合法持久化值；已删除的是运行时 `legacy` 动态回退。回填没有修改正文、维度、结论、风险、主体关联、Generation/Run 引用与生成时间。

## 3. Template 与 ModelCatalog 治理

### 3.1 Template release

| 阶段 | 生产证据 | 结果 |
| --- | --- | --- |
| 历史 release apply/verify | [31093798667](https://github.com/FangcunMount/qs-server/actions/runs/31093798667) / [31093890132](https://github.com/FangcunMount/qs-server/actions/runs/31093890132) | `legacy-v1` 发布身份补齐并验证 |
| 当前 release audit/apply/verify | [31095560896](https://github.com/FangcunMount/qs-server/actions/runs/31095560896) / [31096464770](https://github.com/FangcunMount/qs-server/actions/runs/31096464770) / [31096561755](https://github.com/FangcunMount/qs-server/actions/runs/31096561755) | 当前版本发布并验证 |

当前发布矩阵为 5 个 TemplateID × 2 个版本，共 10 条：

| TemplateID | BuilderIdentity | AdapterKey |
| --- | --- | --- |
| `standard` | `factor-scoring` | - |
| `mbti` | `typology` | `personality_type` |
| `sbti` | `typology` | `personality_type` |
| `bigfive` | `typology` | `trait_profile` |
| `enneagram` | `typology` | `personality_type` |

历史 `legacy-v1` 与当前版本都保留精确 manifest，旧版本用于历史重放，新写必须使用 ModelCatalog 冻结的显式版本。

### 3.2 ModelCatalog 冻结路由

| 阶段 | 生产证据 | 结果 |
| --- | --- | --- |
| 只读审计 | [31096657538](https://github.com/FangcunMount/qs-server/actions/runs/31096657538) | 24 个 active snapshot 待冻结路由 |
| Canary apply/verify/rollback | [31097850580](https://github.com/FangcunMount/qs-server/actions/runs/31097850580) / [31097977546](https://github.com/FangcunMount/qs-server/actions/runs/31097977546) / [31098183196](https://github.com/FangcunMount/qs-server/actions/runs/31098183196) | 单对象写入、验证和精确回滚均通过 |
| 全量 apply/verify | [31098350842](https://github.com/FangcunMount/qs-server/actions/runs/31098350842) / [31098459999](https://github.com/FangcunMount/qs-server/actions/runs/31098459999) | 24/24 显式冻结；缺失为 0 |

## 4. Outcome 冻结路由治理

Outcome 必须携带生成时使用的 TemplateID/TemplateVersion，不能在读取时从当前目录推断。

| 阶段 | 生产证据 | 结果 |
| --- | --- | --- |
| Audit manifest | [31100681795](https://github.com/FangcunMount/qs-server/actions/runs/31100681795) | 212,586 个候选；36,860 个 whole-Sections 历史物化；manifest fingerprint `71c8804a474bfd9b3f6d35d707ef94a51e0440f9530879db121f7d9f9d0c3652` |
| Canary apply/verify/rollback | [31101397144](https://github.com/FangcunMount/qs-server/actions/runs/31101397144) / [31101974124](https://github.com/FangcunMount/qs-server/actions/runs/31101974124) / [31102674554](https://github.com/FangcunMount/qs-server/actions/runs/31102674554) | 单对象写入、精确验证和回滚均通过 |
| 全量 apply | [31102892737](https://github.com/FangcunMount/qs-server/actions/runs/31102892737) | selected=212,586，updated=212,586，already=0 |
| 最终 verify | [31103985240](https://github.com/FangcunMount/qs-server/actions/runs/31103985240) | total=213,150，explicit=213,150，candidate=0，materialized_sections=0，verified=212,586，`require_complete=true` |

治理期间新增的 41 份 Outcome 均由当前写链直接写入显式路由，因此不属于 manifest 历史候选，也无需二次回填。

## 5. 运行时兼容退役

`f63e273a` 完成以下行为收敛：

- Outcome 重建只接受冻结资产中的显式且一致 TemplateID/TemplateVersion；
- Registry 不再补默认 report type 或 template version；
- typology template registry 拒绝空 TemplateID；
- 报告读取不再从当前 ModelCatalog 动态解析 presentation；
- factor-score 历史 Artifact 缺持久化 presentation profile 时直接失败；
- `presentation_profile.source=legacy` 不再是合法写入来源；已物化的 `legacy_artifact_dimensions/v1` 继续可读。

首次严格 CI 暴露 AdmissionFailure Mongo upsert 中 `$setOnInsert` 与 `$set/$inc` 字段重叠。`b6407f40` 将 `attempt`、`last_failed_at`、`trace_id` 的 operator 所有权拆开并补充不相交测试；同一提交把运行时闭环 fixture 改为显式 `standard@2026-08-v1`。最新运行时闭环 E2E 已通过。

## 6. 回滚边界

- Presentation 回滚只允许操作治理 manifest 中的 Artifact ID，且仅当已落库 profile 与 manifest 预期完全一致时 `$unset`；
- ModelCatalog 回滚只恢复 manifest 记录的原字段，不修改 Template release；
- Outcome 回滚只允许操作同一 manifest，且先验证当前值仍等于本次写入值；
- Template 回滚只改变后续目录选择，不改写历史 Generation、Artifact 或 Outcome；
- `legacy-v1` release 与 Builder 在历史重放保留期内不得删除。

## 7. 关闭标准与独立风险

`FINAL-P2-001` 已关闭。Production Deploy 31106231761 的 attempt 2 将 `b6407f4033dc312802cfc9718e2c8286820f5520` 实际部署到 apiserver、collection 与 worker：apiserver healthy，collection 2/2 healthy，worker 3/3 治理端点反向验证通过。部署后 31107140357 确认 213,171 个 Outcome 路由全部显式，profile/builder/content schema 缺口为 0。首次 attempt 因 ACR `EOF/TLS handshake timeout` 失败，未被误记为应用失败；重跑失败 job 后闭环。

以下事项不属于本批 presentation/template 治理，必须独立跟踪，不能用本台账的生产写授权处理：

1. 2026-08-06 只读状态复核发现新的 Attention 高风险投影缺口 91 份，涉及 82 个 Testee、9 个重复 Testee 分组；跨库主体与 Assessment 均存在且组织/关系无异常，但修复需新的对象清单、dry-run 和明确授权；
2. qs-operating-system 的两个 DefinitionV2 契约 fixture 仍缺显式 TemplateID/TemplateVersion，导致 qs-server 跨仓 characterization 门禁失败；后端严格路由不应为此前端契约漂移恢复默认值，应在前端仓独立修复。
