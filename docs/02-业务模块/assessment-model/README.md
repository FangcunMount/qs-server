# AssessmentModel 模块文档

> 统一测评模型（行为能力 + 人格 typology）后台配置、发布快照与 C 端消费的事实源索引。

| 文档 | 说明 |
|------|------|
| [01-统一测评模型后台配置](./01-统一测评模型后台配置.md) | Draft 生命周期、REST 契约、状态机 |
| [02-人格测评模型定义格式](./02-人格测评模型定义格式.md) | `assessmentmodel.personality.typology.v1` + `RuntimeSpec` |
| [03-发布快照与执行链路](./03-发布快照与执行链路.md) | PublishedModelSnapshot → Evaluation → Report |
| [04-Catalog目录缓存（L1+L2）](../../03-基础设施/redis/10-Catalog目录L1-L2缓存.md) | C 端目录读双层缓存、信令失效、配置与排障 |
