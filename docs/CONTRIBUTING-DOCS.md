# qs-server 文档写作约定

本文规定现行文档如何进入、如何维护、何时归档。读者入口见 [README](./README.md)。

## 1. 适用范围

- `docs/00-总览` 至 `docs/05-决策记录` 是现行事实与决策层。
- `docs/06-宣讲` 是可重建的派生层。
- `docs/_archive` 是历史快照，不适用现行结构要求，也不参与默认校验。

## 2. 事实优先级

1. 源码与运行时行为：`cmd/`、`internal/`、`pkg/`。
2. 机器契约：`api/`、`configs/`、migration、生成契约和 `Makefile`。
3. `docs/00-总览` 至 `docs/05-决策记录`。
4. `docs/06-宣讲`。
5. `docs/_archive`。

旧文档可以提供问题、术语和历史背景，但任何行为性结论进入现行层前都必须重新核对代码或机器契约。

## 3. 文档职责

| 层 | 只回答什么 |
| --- | --- |
| 总览 | 系统边界、主链路、代码事实入口 |
| 运行时 | 进程职责、装配、调用、安全、生命周期 |
| 业务模块 | 领域模型、领域/应用服务、关键路径、证据 |
| 基础设施 | 系统压力、机制边界、失败语义、验证 |
| 接口与运维 | 可执行的契约核验、接入、部署、操作与排障 runbook；稳定设计事实回链 canonical 文档 |
| 决策记录 | 已确认的取舍，不重复实现手册 |
| 宣讲 | 从现行层派生的外部表达 |

同一事实只在一个 canonical 文档讲透，其它地方摘要并回链。

## 4. 状态标签

- 正文事实状态使用 `已实现`、`待补证据`、`规划改造`、`历史资料`：它说明某一条行为陈述的成熟度。
- 文档收口状态使用 `aligned`、`needs_review`、`drifted`、`planned`、`archive_candidate`：它说明一篇文档是否已经对齐声明的源码基线，以及是否需要复核、修正、建设或归档。
- 实现状态使用 `implemented`、`partial`、`planned`、`not_applicable`：它说明代码能力，不代表生产已启用。
- 证据等级使用 E0-E7，定义见[项目完成画像与验收标准](./00-总览/08-项目完成画像与验收标准.md#4-证据等级)。

三套状态禁止互相替代。例如 `aligned + partial + E2` 表示“文档准确描述了一个仅局部实现、由仓库测试证明的能力”，不能简写成“文档未完成”或“生产已完成”。
规范化元数据集中维护在 [`document-closure.json`](./document-closure.json)，不要求在每篇 active Markdown 中复制 front matter。

未标状态的正文默认声称“当前成立”，因此必须有可回查的代码或契约入口。

### 4.1 优先级命名空间

文档中存在两套用途不同的优先级，禁止混用：

| 命名 | 作用域 | 能否直接判定版本阻断 |
| --- | --- | --- |
| `FINAL-P*` | [当前版本定档验收台账](./00-总览/09-当前版本定档验收台账.md)中的跨模块版本门禁 | 可以；只有状态为“开放”的 `FINAL-P0/FINAL-P1` 才阻断定档 |
| `MC/EV/IR/AR/PL/ST-Rxxx + P*` | 模块发现问题时的固有风险等级与实施顺序 | 不可以；必须同时读取条目的当前状态、适用调用链和全局验收映射 |

模块条目关闭后保留原始 P0/P1，用于说明当初风险，不表示它仍开放。处于“待业务决策”“待容量触发”“兼容观察”或“已接受边界”的模块条目，也不能自动提升为 `FINAL-P1`。需要阻断版本时，必须在总台账创建具名 `FINAL-P*`，写清退出条件、Owner 和证据。

模块活动台账的顶部必须给出当前版本判定，并把较早的实施记录标成日期快照；历史快照不得使用“当前仍有”之类无日期表述覆盖总台账结论。

## 5. 业务模块最低结构

核心模块至少回答：

1. 负责与不负责什么；
2. 聚合、实体、值对象和不变式是什么；
3. 领域服务与 application service 分别落在哪里；
4. 关键路径从入口到持久化/事件如何流转；
5. 如何验证，事实源在哪。

支撑模块可以把上述内容合并在一篇 README，但不能只保留目录或接口清单。

### 5.1 基础设施专题的知识结构

基础设施文档按问题组织，不按 Redis、MQ、GORM 等组件平均分栏。Cache、Event、Concurrency / Resilience 是三条问题主线；
Data Access、Migration & Recovery、Security、Observability、Runtime、Config & Deployment 分别提供事实、恢复、权限、证据、生命周期和交付支撑。

专题可以按篇章拆分，但一组文档整体必须保存以下推理链：

1. 场景、业务损失与常见误判；
2. 统一概念、量纲、identity 和 scope；
3. 安全性、活性、容量不变量；
4. 相邻完成点之间的失败窗口；
5. 候选方案、演化路径、收益与代价；
6. 当前代码、配置、composition root 与机器契约事实；
7. 降级、恢复、终态和人工处置；
8. 测试、指标、故障注入、压测或运行证据；
9. 当前接受的代价和重新决策条件。

禁止只保留以下任一形态：

- 只有 current implementation 的组件/API 清单；
- 一份与所属问题脱节的“核心决策汇总”，其余章节不解释推导；
- 只有优点，没有替代方案、失败语义和接受的代价；
- 把讨论或历史方案直接当成当前事实；
- 把规划能力写成已实现，或用测试通过代替生产容量/故障证据。

跨模块词汇和因果主线只在根级 canonical 文档讲透；专题引用并展开自己的部分。同一机制出现在多个专题时，分别说明其角色并回链最终 owner，不复制多份相互漂移的定义。

文档数量使用“评审目标 + 硬上限”，而不是压缩目标：

- 现行 active Markdown 评审目标为 150，硬上限为 165；
- 单个业务模块评审目标为 18，硬上限为 22；
- 达到评审目标时应检查职责重复、错误分层和可归档内容，不能为了凑数量把独立设计问题重新塞回超长 README；
- 超过评审目标但未超过硬上限时，必须在 `document-closure.json` 的 `budgets.exceptions` 记录 scope、原因、owner 和复核日期；
- 突破硬上限必须先完成独立结构评审并同步修改约定和门禁，不能通过关闭校验绕过。

## 6. 链接与命名

- 现行文档不得链接 `docs/_archive`。
- 指向仓库文件一律使用相对链接，并优先指向稳定目录或契约入口。
- 模块名使用注册表中的 `survey`、`modelcatalog`、`evaluation`、`interpretation`、`actor`、`plan`、`statistics`。
- 产品展示名可以使用 “Assessment Model”，但必须明确其代码模块是 `modelcatalog`。
- 事件名称必须来自 `configs/events.yaml`；信令名称必须来自 `configs/signals.yaml`。

## 7. 何时必须同步文档

以下改动必须在同一批次更新文档：

- 模块注册、组合根、跨模块依赖发生变化；
- REST、gRPC、事件、信令或配置契约发生变化；
- 聚合边界、状态机、事务边界或失败语义发生变化；
- 对外接入步骤、运维操作或验证命令发生变化。

## 8. 归档策略

内容出现以下情况时，移出现行树：

- 只描述历史方案或规划；
- 与 canonical 文档重复；
- 指向已不存在的代码结构；
- 长期无人能为其事实负责。

大型重建优先保存一个完整、带日期的快照，避免把多个历史体系拆成难以检索的碎片。归档只供追溯，删除由单独决策完成。

## 9. 验证

```bash
make docs-check
git diff --check
# 仅修正 docs/ 空白、表格后空行、断裂标记、加粗伪标题（不折行，且指纹不变才写盘）：
python scripts/check_docs_hygiene.py --fix
# 在 --fix 基础上再把正文折行到约 120 列（代码块/表格/HTML 注释不折，指纹不变才写盘）：
python scripts/check_docs_hygiene.py --fix --reflow
```

`docs-check` 先运行门禁自身的正负/mutation 测试，再执行 `docs-hygiene` 与 `docs-facts`。hygiene 覆盖仓库根 README 及代码、配置、脚本旁的现行 Markdown，默认只排除 `docs/_archive`；
其中 `docs/` 额外检查连续空行、标题前空行、`---` 前空行、表格后空行、加粗伪标题，以及正文中换行折断的 `` `* *` `` / `` ` - > ` ``（行内代码中的示例除外）。
`--reflow` 还会把正文段落折到约 120 列，并拼回被错误折断的中文/英文行，但不改信息指纹。facts 验证 active taxonomy、反引号仓库源码路径、目录预算、模块/事件入口、版本/API
数量、scheduler/config 清单、关键状态、Mongo audit 与性能计划。反引号路径若只是历史已删除位置，应改写为历史说明并指向当前防回流测试或 Git 历史，不能继续伪装成现行事实入口。

`docs-facts` 还必须验证 `document-closure.json` 对 164 篇 primary 文档和 27 篇 maintained sidecar 的 exact coverage、source
baseline、状态枚举、七模块七轴签署、十项基础设施七轴签署，以及 REST/gRPC/Event/Signal/Migration 等机器契约 ratchet。164 超过 150 的评审目标但低于 165 硬上限，
例外理由记录在 `budgets.exceptions`，不能据此继续无审查扩张。

台账中的 `checkout_ref: git:HEAD` 是动态工作区引用；历史 CI、部署和生产 SHA 使用独立字段，不得要求它们等于 HEAD。
`verification`、模块与基础设施维度中的可升级证据只能使用严格 `command` 或 `source_selector` 对象；未实际执行的测试不得登记为 passed。
`docs-facts` 能校验命令入口、源码 selector、SHA/date schema 和 source freshness，但不会替操作者重演台账中的历史命令或访问外部证据；命令的实际退出码仍必须由本次执行者/CI 保存并负责。

十项基础设施主题必须各自覆盖 checker 规定的关键 source/config scope，不能用一份通用测试或一个无关目录代替。`passed` 环境测试必须附结构化原始证据引用；没有真实环境执行时保持 `not_run` 并绑定 gap。
生产记录通过 `topics` 声明适用范围，主题可以诚实保留空引用，但 production 维度必须非 ready、unsigned 且有具名 production gap。
只有 topics 相关、未过期、结果 passed、`deployed_sha` 精确等于当前 checkout、`source_baseline_sha` 等于当前机器基线且保存真实 effective-config `sha256`
的记录，才有资格支持 production ready；`unknown_not_recorded` 只允许历史记录。历史基础设施运行证据集中保存在
[`infrastructure-production-evidence.json`](./infrastructure-production-evidence.json)，人工入口见[基础设施生产证据台账](./00-总览/10-基础设施生产证据台账.md)；
历史记录不得自动升级当前 production 签署。

涉及 REST 生成契约时再执行：

```bash
make docs-verify
```
