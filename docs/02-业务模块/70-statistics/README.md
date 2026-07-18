# Statistics

Statistics 是读侧投影与查询模块。它从 Actor、Plan、Evaluation、Interpretation 等事实构建统计视图，不拥有这些模块的写侧状态。

## 1. 领域与读模型

`internal/apiserver/domain/statistics` 主要包含：

- 组织概览、访问漏斗、测评服务窗口与趋势；
- 计划任务活动、完成度和趋势；
- 医生、测评入口、内容批量统计；
- 受试者周期统计；
- `AssessmentEpisode`、行为事实、待处理分析事件与 journey mutation。

统计结构是查询语义，不应被误当作其它模块的聚合根。

## 2. 应用服务

`internal/apiserver/application/statistics` 的主要责任：

- `readService` 及拆分 query：聚合读模型并返回 API 视图；
- `assessmentEpisodeProjector`：把行为事件投影到 journey；
- `syncService`：日统计、组织快照、计划统计同步；
- periodic service：受试者周期统计；
- governance/cache：查询缓存、hotset 和运行状态。

## 3. 一致性语义

统计允许可观测的最终一致性。事件投影、扫描补偿、checkpoint 和重建共同保证可恢复性；“可重建”不等于表或投影可以在无依赖审计的情况下删除。

## 4. 权限边界

批量内容统计、医生和入口统计都必须先应用组织/角色/资源访问范围，再查询或聚合。缓存键与热集也必须包含足够的租户和查询维度。

## 5. 证据与验证

- domain：`internal/apiserver/domain/statistics`。
- application：`internal/apiserver/application/statistics`。
- ports/infra：statistics read model、journey repository、MySQL/Mongo 实现。
- 装配：`internal/apiserver/container/modules/statistics`。
- 验证：statistics application/domain/container、投影/同步和访问控制定向测试。

状态：`已实现`（本轮核对到查询、投影、同步与缓存职责；逐指标口径表待补证据）。
