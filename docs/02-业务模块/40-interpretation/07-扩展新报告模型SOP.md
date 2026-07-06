# 扩展新报告模型 SOP

## 1. 适用场景

新增一种测评报告、一个模型的报告 adapter、或一类新的解释模板。

---

## 2. 步骤

1. 确认上游 `EvaluationResult` 已提供足够结构化结果。
2. 确认 `model-catalog` 能提供模型身份和必要元数据。
3. 定义报告 section 和展示结构。
4. 新增或扩展 ReportBuilder。
5. 新增 score / personality / custom adapter。
6. 持久化 `InterpretReport`。
7. 发布 `report.generated`，并确认统计投影是否需要新增口径。

---

## 3. 验收标准

| 检查项 | 标准 |
| ------ | ---- |
| 边界 | 不改 Survey 提交和 Evaluation 计分逻辑 |
| 输入 | Builder 输入来自结构化结果和模型身份 |
| 输出 | 统一落到 `InterpretReport` |
| 追溯 | 能回到 Evaluation 和模型快照 |
| 文档 | 更新本模块和全链路文档 |

---

## 4. 禁止事项

- 不要在报告 builder 中重新计分。
- 不要把报告模板塞进 EvaluationResult。
- 不要让 Statistics 决定报告内容。
