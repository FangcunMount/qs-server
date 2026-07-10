# ModelCatalog

ModelCatalog 是测评模型资产目录：它管理可编辑模型定义、发布不可变模型快照，并向运行时提供已发布的 `DefinitionV2`。

```text
AssessmentModel + DefinitionV2
  -> publish
Published AssessmentSnapshot + DefinitionV2
  -> resolver
evaluation / collection / survey / notification
```

当前事实：`DefinitionV2` 是配置和运行语义事实；`port/modelcatalog/payload/*` 下的 JSON 是 published wire/runtime DTO，不参与领域判断。运行时只读取 published model，不读取 draft、旧 `scales` collection，也不从 payload 回退推导语义。

| 文档 | 内容 |
| --- | --- |
| [01-模块设计](./01-模块设计.md) | 职责、应用服务和边界 |
| [02-领域模型设计](./02-领域模型设计.md) | `AssessmentModel` 与四层 `DefinitionV2` |
| [03-关键链路分析](./03-关键链路分析.md) | 编辑、发布、运行和 collection BFF 链路 |
| [04-存储与契约](./04-存储与契约.md) | Mongo、REST、gRPC、payload 契约 |
| [05-目标设计草案](./05-目标设计草案.md) | 已落地的终局不变量 |
| [06-重构计划](./06-重构计划.md) | 数据迁移、发布和故障处理运行手册 |

历史设计与已完成的迁移记录放在 `docs/_archive/2026-07-09-model-catalog-redesign/`，不再代表现行实现。
