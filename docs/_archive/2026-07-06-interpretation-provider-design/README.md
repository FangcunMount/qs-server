# Interpretation Provider 旧设计归档

本文档组来自原 `docs/02-业务模块/interpretation-model/` 下的 4 篇 Provider / Context / Registry 设计文档。

归档原因：

- 文档假设存在 `internal/apiserver/application/interpretation` 等代码路径，但当前代码事实并非如此。
- 当前业务模块口径已调整为 `assessment-model / evaluation / interpretation-model(report)`。
- 这些文档仍可作为多模型扩展的历史设计输入，但不能作为当前模块边界或代码事实引用。

现行入口：

- `docs/02-业务模块/assessment-model/README.md`
- `docs/02-业务模块/evaluation/README.md`
- `docs/02-业务模块/interpretation-model/README.md`
- `docs/05-专题分析/01-为什么拆分Survey-InterpretationModel-Evaluation.md`
