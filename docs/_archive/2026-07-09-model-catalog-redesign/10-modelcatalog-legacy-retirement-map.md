# ModelCatalog Legacy Retirement Map

本文档锁定 `internal/apiserver/domain/modelcatalog` 旧模型迁移和删除边界。目标不是立即删除所有旧类型，而是先把旧职责退到可识别的兼容层，后续按引用清零逐步删除。

## 状态标记

- `migrate`: 生产职责必须迁入 `AssessmentModel + DefinitionV2`。
- `adapter-only`: 只允许作为旧 payload、旧 collection、旧 published row 的 ACL/投影器存在。
- `keep-runtime-dto`: runtime/published payload shape，外部契约未退休前不能删除。
- `delete-after-rg-zero`: 生产引用清零后删除，测试引用随后随迁移用例一起清理。

## Inventory

| 旧模型/代码 | 当前职责 | 目标归属 | 状态 | 删除条件 |
| --- | --- | --- | --- | --- |
| `domain/modelcatalog/scoring/definition.MedicalScale` | 医学量表 authoring aggregate、factor/question/interpret rule 编辑、发布前快照来源 | `assessmentmodel.AssessmentModel` 承载基本信息/生命周期，`definition.MeasureSpec` 承载测量层，`conclusion.RiskConclusion` 承载解释规则 | `migrate` | application scoring 写路径切到 `ModelRepository + DefinitionV2`，旧 scale collection 只读/回填路径完成，生产 import 清零 |
| `domain/modelcatalog/scoring/definition.Factor` / `FactorSnapshot` | 旧医学量表内部因子和值对象快照 | `definition.MeasureSpec + factor.FactorGraph + factor.Scoring`；快照只在旧 scale adapter 内保留 | `adapter-only` | `MedicalScale` aggregate 删除后随包删除 |
| `domain/modelcatalog/scoring/snapshot` | scale published/runtime payload DTO | published payload 边界；由 `DefinitionV2` 投影生成 | `keep-runtime-dto` | published payload format 退休或完成版本化替代 |
| `domain/modelcatalog/factor.LegacyFactor` | shared flat factor payload materialization | `definition.MeasureSpec + definition.Calibration` 的 legacy assembler/projector | `adapter-only` | norming/taskperformance/scoring snapshot decode 不再需要 flat adapter，且生产 token 清零 |
| `domain/modelcatalog/factor.FactorSnapshot` | behavioral/cognitive shared published DTO | runtime/published compatibility DTO | `keep-runtime-dto` | 旧 published payload bytes 不再需要兼容 |
| `domain/modelcatalog/factor.DefinitionBody` | shared JSON body adapter | `definition.ParseMeasureSpecFromDefinitionBody` 的输入 ACL | `adapter-only` | draft/published JSON 均可直接解析成 `DefinitionV2` |
| `domain/modelcatalog/norming/snapshot` | behavioral rating published/runtime payload DTO | published payload 边界；领域语义迁入 `DefinitionV2` | `keep-runtime-dto` | behavioral payload format 退休或完成版本化替代 |
| `domain/modelcatalog/taskperformance/snapshot` | cognitive published/runtime payload DTO | published payload 边界；领域语义迁入 `DefinitionV2` | `keep-runtime-dto` | cognitive payload format 退休或完成版本化替代 |
| `domain/modelcatalog/typology` legacy MBTI/SBTI payload and graph helpers | typology legacy graph、classification/outcome compatibility | `definition.MeasureSpec` 承载测量/计分，`conclusion.TypeConclusion` 承载分类/结果 | `adapter-only` | ruleset/evaluation fallback 不再依赖 legacy graph payload |
| `domain/modelcatalog/legacy/*` | 根门面 alias facade | 无目标领域职责 | `delete-after-rg-zero` | 生产 import 清零后删除 alias 包和 root re-export |
| `infra/evaluationinput` legacy scale resolver | 从旧 scale collection 和旧 published payload 构造执行输入 | DefinitionV2-first reader，legacy fallback 只处理旧 row | `adapter-only` | 所有 published rows 均有 DefinitionV2 且 fallback 监控清零 |
| `infra/ruleset` legacy scale publisher/backfill | 旧 ruleset payload 发布/回填 | DefinitionV2-first ruleset publisher，legacy codec 只处理旧缓存 | `adapter-only` | 缓存/旧 row fallback 退休后删除 |

## Guard Rules

- 新生产代码不得新增 `domain/modelcatalog/legacy/*` import；alias facade 只允许包自身和测试使用。
- `domain/modelcatalog/scoring/definition` 只允许出现在当前迁移白名单：scale authoring、旧 scale repository/cache、ruleset fallback、evaluationinput 兼容 adapter、composition root。
- `factor.LegacyFactor`、`factor.FactorSnapshot`、`factor.DefinitionBody` 只能出现在 factor/definition assembler、snapshot payload、norming/taskperformance 兼容解析、calculation bridge 等 adapter 边界。
- `domain/modelcatalog/export.go` 的旧 re-export 是 transitional surface，后续按 root facade 清理批次移除。

## Deletion Order

1. 删除 `domain/modelcatalog/legacy/*` alias facade 和 root legacy re-export。
2. 将 scale create/update/factor/lifecycle 写路径迁到 `AssessmentModel.DefinitionV2`。
3. 将 evaluationinput/ruleset/published catalog read path 改为 DefinitionV2-first，legacy payload fallback-only。
4. 将 interpret/classification/norm conclusion 语义迁入 `conclusion`。
5. 删除 `factor.LegacyFactor` 非 adapter 使用、legacy flat wrapper、误导性 root facade。
6. 在旧 scale collection 写入停止、backfill 完成、fallback 监控清零后删除 `scoring/definition` authoring aggregate。
