# qs-server 文档写作约定

本文约定 `docs/` 下的现行文档如何与代码对齐。读者入口见 [README.md](./README.md)。

---

## 适用范围

- 本文适用于 `docs/00-总览` 到 `docs/06-宣讲` 的现行文档。
- `docs/_archive/` 是历史材料层，不适用现行结构要求。
- 归档文档只能作为信息源或迁移参考，不能直接视为当前事实。

---

## 事实来源与优先级

判断文档事实时，优先级如下：

1. 源码与运行时行为：`cmd/`、`internal/`、`pkg/`。
2. 机器可读契约与配置：`api/rest/`、`api/grpc/`、`configs/events.yaml`、`configs/*.yaml`、migration、`Makefile`。
3. `docs/00-05` 现行维护文档。
4. `docs/06-宣讲`。
5. `docs/_archive` 和其它历史材料。

如果 prose 文档与代码或机器契约冲突，以代码和机器契约为准。

---

## 当前命名约定

业务模块文档采用当前业务语言：

```text
survey
assessment-model
evaluation
interpretation-model
actor
plan
statistics
```

代码事实仍保留当前包名：

```text
survey
assessmentmodel
evaluation
report
actor
plan
statistics
```

`scale/personalitymodel` 是 `assessmentmodel` 的兼容注册名或旧能力路径，不再作为独立核心模块维护。

---

## 写作规则

- 先结论，再展开。
- 先写当前事实，再写历史背景或规划。
- 模块入口只做阅读地图和边界，不重复维护深讲细节。
- 长文如果引用旧设计，必须标明 `历史资料`、`待补证据` 或 `规划改造`。
- 一个事实只在一个 canonical 文档讲透，其它文档摘要并回链。

---

## 归档规则

文档满足任一条件时，应归档或删除：

- 指向不存在的代码路径或旧包名，且已有新入口替代。
- 描述的模块边界已被 `registry.go` 或当前代码事实取代。
- 只剩历史设计价值，不应参与现行阅读路径。
- 内容重复且没有独立维护价值。

处理方式：

- 仍有历史参考价值：移动到 `docs/_archive/<date>-<topic>/`。
- 无独立信息价值：删除。
- 归档后必须更新 active docs 中的链接，避免现行文档依赖 archive。

---

## Verify

文档变更后至少执行：

```bash
make docs-hygiene
git diff --check
```

涉及 REST、gRPC、事件或配置契约时，再执行：

```bash
make docs-verify
```
