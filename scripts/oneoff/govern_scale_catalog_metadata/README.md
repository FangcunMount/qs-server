# govern_scale_catalog_metadata

以不可变新 release 治理生产环境医学量表目录的 `category/tags`：

1. `audit` 对比 active `published_snapshot` 与唯一 head，生成带内容哈希的只读 manifest；
2. `apply` 在一个 Mongo 事务内归档旧 active release、插入 metadata 修正版 release；
3. `verify` 验证 8 个 canonical 分类、快照内容保护哈希、head 元数据及 MBTI 污染归零；
4. `rollback` 将 active 选择切回旧 release，并以新的 revision 恢复本工具修改过的 head。

工具不会原地修改 published snapshot。除已确认的 `fthN56`（SCL-90）和 `zOO4eG`（QCD）
MBTI 元数据污染外，目标分类与标签全部来自当前 head。DefinitionV2、问卷绑定、算法和历史 release
均保持不变。

生产执行统一使用 `Database Operations` workflow 的 `govern-scale-metadata` 操作。写操作必须提供：

- `governance_manifest`：audit 生成的不可变 manifest；
- `backup_name`：执行 apply/rollback 前完成的 Mongo 全量备份；
- apply 确认词：`activate-scale-catalog-metadata-2026-08-v1`；
- rollback 确认词：`rollback-scale-catalog-metadata-2026-08-v1`。

验收时 `governance_operation=verify` 且 `presentation_require_complete=true`，必须得到：

- `missing_or_invalid_category=0`；
- `forbidden_mbti_tags=0`；
- `active` 与 audit 时的活动医学量表总数一致。
