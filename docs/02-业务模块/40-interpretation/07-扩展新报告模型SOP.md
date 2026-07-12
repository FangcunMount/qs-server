# 扩展新报告模型 SOP

## 1. 适用场景

新增一种测评报告、一个模型的报告 adapter、或一类新的解释模板。

---

## 2. 步骤

1. 确认持久化 `EvaluationOutcome` 已冻结足够的结构化事实和模板输入。
2. 在 Model Catalog 发布模型身份、报告配置和不可变 `TemplateVersion`。
3. 在 Interpretation rendering 中新增或扩展 Builder，并声明完整机制 Key。
4. 为新 Key、MultiKey、重复注册和未知 TemplateVersion 增加 Registry 测试。
5. 复用 `InterpretationCommitter` 持久化三对象终态与 terminal event。
6. 确认 `report_query_catalog` 事务投影和各 Audience 查询视图无需新增契约。

---

## 3. 验收标准

| 检查项 | 标准 |
| ------ | ---- |
| 边界 | 不改 Survey 提交和 Evaluation 计分逻辑 |
| 输入 | Builder 输入只来自持久化 Outcome 与冻结模板配置 |
| 输出 | 统一落到 `InterpretReport` |
| 追溯 | 能回到 Evaluation 和模型快照 |
| 文档 | 更新本模块和全链路文档 |

---

## 4. 禁止事项

- 不要在报告 builder 中重新计分。
- 不要把报告模板塞进 EvaluationResult。
- 不要让 Statistics 决定报告内容。
- 不要注册运行时 AI、数据库或动态建议策略；建议只能来自冻结输入。
- 不要从 artifact repository 按 Assessment 选择最新报告；当前来源由 catalog 决定。
