# 文档重建状态

本页记录 2026-07-18 文档体系重建的证据范围。它是迁移清单，不是产品路线图。

## 1. 已完成

| 范围 | 状态 | 证据 |
| --- | --- | --- |
| 根入口、事实优先级、写作规则 | 已实现 | `README.md`、`docs/README.md`、`docs/CONTRIBUTING-DOCS.md` |
| 现行树与历史树隔离 | 已实现 | `_archive/2026-07-18-pre-truth-layer-rebuild/` |
| 业务模块命名 | 已实现 | `internal/apiserver/container/modules/registry.go` |
| 事件与信令命名 | 已实现 | `configs/events.yaml`、`configs/signals.yaml` |
| Survey / ModelCatalog / Evaluation / Interpretation 深度文档 | 已实现 | 保留已核对的 canonical 模块文档，并重新接入新入口 |
| Cache / Event 深度文档 | 已实现 | 保留问题导向的 canonical 机制文档 |
| 链接与边界自动检查 | 已实现 | `make docs-hygiene`、`make docs-facts` |

## 2. 本轮收缩

- 重建前现行树：181 篇 Markdown，约 47,283 行。
- 超长专题分析和宣讲材料：整体移入重建前快照，不再承担事实真值。
- Actor、Plan、Statistics：由多篇浅模板收敛为各自一篇可维护入口。
- Runtime、Data Access、Security、Observability：从组件清单收敛为职责与证据索引。
- 具体前端接入、报告等待和压测 SOP：因仍具执行价值，保留在接口与运维层。

## 3. 待补证据

| 优先级 | 范围 | 完成标准 |
| --- | --- | --- |
| P1 | Actor / Plan / Statistics 深度设计 | 各模块补齐独立领域模型、服务与关键路径文档，并通过定向测试 |
| P1 | 接口接入文档逐端点复核 | 与当前 OpenAPI、前端调用代码和鉴权要求逐项对照 |
| P2 | Concurrency / Resilience 深度专题 | 以当前 `component-base` 能力、collection 保护链和治理操作为证据重写 |
| P2 | 数据访问与迁移图 | 映射当前 MySQL/Mongo repository、UoW、Outbox 和迁移 |
| P3 | 宣讲材料 | 仅从复核后的 00-05 层重新生成，不从旧宣讲快照复制 |

## 4. 不应回迁的内容

- 把规划目标写成已实现事实的系统设计稿；
- 已被当前事件名替代的旧链路；
- 只罗列组件、接口或目录，没有责任边界和验证方法的模板页；
- 与机器契约重复维护的大段端点清单。
