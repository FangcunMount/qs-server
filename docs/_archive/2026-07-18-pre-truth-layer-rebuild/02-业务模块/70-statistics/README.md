# Statistics

**结论**：Statistics 是机构隔离的查询与可重建投影模块。查询主线是 Overview、Clinician、Entry、Periodic、Typed Content；投影主线是行为同步、补偿扫描、pending 重建与 Overview 缓存治理。

## 模块职责

- 查询机构总览、从业者、入口和受试者周期统计。
- 按 `(content_type, content_code)` 批量查询内容形成量、完成量和完成率。
- 将入口、接入、答卷、测评、报告和计划事实投影为 journey/content/plan daily 与 organization snapshot。
- 维护扫描 watermark、pending reconcile、定时重建和 Overview 缓存。

Statistics 不维护问卷、答卷、Assessment、报告或计划的主事实，也不反向驱动这些业务状态。

## 现役模型

| 模型族 | 说明 |
| ------ | ---- |
| Behavior footprint | 幂等行为事实与补偿入口 |
| Journey daily | 机构、从业者、入口旅程聚合 |
| Content daily | questionnaire/scale typed content 聚合 |
| Plan daily | activity 与 fulfillment 聚合 |
| Organization snapshot | 机构累计快照 |

## 文档导航

| 主题 | 文档 |
| ---- | ---- |
| 模块边界 | [01-模块定位与边界.md](./01-模块定位与边界.md) |
| 真实模型 | [02-领域模型.md](./02-领域模型.md) |
| 指标口径 | [03-统计指标模型.md](./03-统计指标模型.md) |
| 投影与扫描 | [04-事件投影链路.md](./04-事件投影链路.md) |
| 查询与缓存 | [05-查询视图与读模型.md](./05-查询视图与读模型.md) |
| 一致性 | [06-一致性与延迟容忍.md](./06-一致性与延迟容忍.md) |
| 接口与存储 | [07-接口事件与存储.md](./07-接口事件与存储.md) |

代码事实入口：

- [`internal/apiserver/application/statistics`](../../../internal/apiserver/application/statistics/)
- [`internal/apiserver/domain/statistics`](../../../internal/apiserver/domain/statistics/)
- [`internal/apiserver/infra/mysql/statistics`](../../../internal/apiserver/infra/mysql/statistics/)
- [`internal/apiserver/container/modules/statistics`](../../../internal/apiserver/container/modules/statistics/)
