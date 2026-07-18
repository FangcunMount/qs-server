# docs archive（历史快照）

本目录保存已经退出现行阅读路径的旧文档，只用于历史检索和重建取证。它不是现行文档层，也不能作为当前实现依据。

归档规则：

- 归档内容只作为历史信息源或迁移参考。
- 归档内容不属于 `docs/00-05` active truth layer。
- 归档内容默认不参与 `make docs-hygiene` 和 `make docs-facts`。
- 如果要把归档内容重新写入现行文档，必须重新核对源码、机器契约和配置。
- 现行文档不得依赖本目录；完成迁移的信息应进入 canonical 文档。
- 归档删除必须作为单独、可审查的决定执行，不在内容重建时零散删除。

当前归档批次：

- `2026-07-18-pre-truth-layer-rebuild/`：本轮退出 truth layer 的集中快照；包含被替换的旧总览、运行时、业务支撑模块、基础设施浅模板、接口说明、专题分析、宣讲和系统设计稿。继续留在现行层的核心模块、cache/event 与执行指南不在此重复保存。

- `2026-07-06-business-module-redesign/`：`docs/02-业务模块` 旧未编号模块目录迁移归档。
- `2026-07-06-interpretation-provider-design/`：旧 Interpretation Provider 设计文档归档。
- `2026-07-06-infra-component-plane/`：旧基础设施组件平面文档归档。
