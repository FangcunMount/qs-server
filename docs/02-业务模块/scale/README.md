# Scale 深讲目录

**本文回答**：Scale 深讲目录说明量表规则为什么独立成界，以及 `MedicalScale / Factor / InterpretationRule / ScoringService` 如何为 Evaluation 提供稳定的规则权威源。

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 模块定位 | `scale` 只维护量表规则、因子、计分策略和解读规则，不保存答卷事实，也不推进测评状态 |
| 核心聚合 | `MedicalScale` 是聚合根，`Factor` 是量表内因子实体，`InterpretationRule` 是分数区间到结论/建议/风险的值对象 |
| 主链关系 | Survey 产生答卷事实，Scale 提供规则，Evaluation 在 pipeline 内组合两者得出测评结果 |
| 当前缓存 | `ScaleListCache` 是 static-list rebuilder，用于全局量表列表读优化，不是 object repository decorator |
| 维护原则 | 新增规则能力必须先确认它属于 Scale 规则域，而不是 Survey 展示结构或 Evaluation 产出状态 |

## 阅读顺序

```mermaid
flowchart LR
    model["00 整体模型"] --> scoring["01 规则与因子计分"]
    scoring --> interpretation["02 解读规则与风险文案"]
    interpretation --> evaluation["03 与 Evaluation 衔接"]
    evaluation --> sop["04 新增量表能力 SOP"]
```

| 顺序 | 文档 | 先回答什么 |
| ---- | ---- | ---------- |
| 1 | [00-整体模型.md](./00-整体模型.md) | Scale 边界、聚合、服务和缓存如何分工 |
| 2 | [01-规则与因子计分.md](./01-规则与因子计分.md) | 因子和计分策略如何表达规则 |
| 3 | [02-解读规则与风险文案.md](./02-解读规则与风险文案.md) | 分数区间如何映射到风险、结论和建议 |
| 4 | [03-与Evaluation衔接.md](./03-与Evaluation衔接.md) | Evaluation 如何消费 Scale，而不是反向污染规则域 |
| 5 | [04-新增量表能力SOP.md](./04-新增量表能力SOP.md) | 新增量表字段、因子策略、解读规则时需要补什么 |

## 代码锚点与测试锚点

| 能力 | 锚点 |
| ---- | ---- |
| Scale 聚合与生命周期 | [internal/apiserver/domain/scale/medical_scale.go](../../../internal/apiserver/domain/scale/medical_scale.go)、[internal/apiserver/domain/scale/lifecycle.go](../../../internal/apiserver/domain/scale/lifecycle.go) |
| 因子与计分 | [internal/apiserver/domain/scale/factor.go](../../../internal/apiserver/domain/scale/factor.go)、[internal/apiserver/domain/scale/scoring_service.go](../../../internal/apiserver/domain/scale/scoring_service.go) |
| 解读规则 | [internal/apiserver/domain/scale/interpretation_rule.go](../../../internal/apiserver/domain/scale/interpretation_rule.go) |
| 应用服务 | [internal/apiserver/application/scale/](../../../internal/apiserver/application/scale/) |
| 列表缓存 | [internal/apiserver/application/scale/global_list_cache.go](../../../internal/apiserver/application/scale/global_list_cache.go)、[internal/apiserver/application/scale/global_list_cache_test.go](../../../internal/apiserver/application/scale/global_list_cache_test.go) |

## Verify

```bash
go test ./internal/apiserver/domain/scale ./internal/apiserver/application/scale
python scripts/check_docs_hygiene.py
```

