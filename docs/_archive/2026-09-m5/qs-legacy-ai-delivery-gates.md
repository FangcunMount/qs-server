# QS 旧 AI 实现门禁历史

> 此处为旧引擎的历史交付记录，不再作为当前验收清单。

## 17. AI 解读：实现与发布门禁

> 状态（2026-09-01）：轻量 v2 已合并并完成受控生产部署，`participant_enabled=false` 继续阻止用户流量。
> 后端提交 `4f9dab9ea594cea192d83a9ef6bdfb12e62f85be` 的
> [CI 33470001144](https://github.com/FangcunMount/qs-server/actions/runs/33470001144) 与
> [Production Deploy 33470547988](https://github.com/FangcunMount/qs-server/actions/runs/33470547988) 成功；
> 治理台空数组兼容修复的 [Production Deploy 33472710822](https://github.com/FangcunMount/qs-operating-system/actions/runs/33472710822) 也成功。
> 生产 v2 Run `635356837083886126` 已收集 35 个 review-ready Candidate 和 35 份 Semantic 证据并进入 `awaiting_review`，当前人工审核为 `0 / 70`。
> 这证明 Slot/Generation/Candidate/Semantic 分离、有界补执行、prepared recovery 和治理台证据链在生产自动收集阶段可工作，但不等于 G1～G5 已通过。
> 历史 16 个 Run、525 次 Generation 的 BSON/输出只读分布已取得；包含 Semantic Execution 的完整 v2 BSON 复测因审计程序上传阶段取消而尚无数据。多集合、增量审核、完整双运行时和审核自动写回 Prompt 不进入近期实现。
> 首个 approved Evidence、Profile、金额成本、SLO/告警、备份恢复/导出验收、客户端和用户流量发布仍未完成。本节不得据此宣称用户生产能力已经开放。

### 17.1 设计与发布门禁总表

| ID | 优先级 | 门禁主题 | 必须持续回答的问题 | 关闭产物 |
| --- | --- | --- | --- | --- |
| AIX-D001 | P0 | CurrentReport 来源与过期语义 | 如何唯一解析当前标准报告；标准报告后来变化时，既有 AI Artifact 显示历史、过期还是重新可触发；无完整 provenance 的 legacy 报告是否准入 | CurrentReport port、source/staleness 状态表、兼容门禁与测试矩阵 |
| AIX-D002 | P0 | 应用命令与查询契约 | 手动触发、重复请求、状态查询、成品查询、人工重试分别是什么用例；`not_ready`、`not_applicable`、`pending`、`failed` 如何稳定表达 | Application use-case contract、错误码和幂等响应矩阵 |
| AIX-D003 | P0 | Participant API 与 owner | participant gRPC 和内部 automation 是否分服务；外部 REST/BFF 由 apiserver 还是 collection-server 暴露；如何表达 capability/隐藏按钮 | proto/OpenAPI 草案、ACL action、owner 和兼容策略 |
| AIX-D004 | P0 | 可执行 PromptPackage | Markdown 如何生成受版本控制的运行资产；如何校验 template ID/version、指纹、Git blob SHA 与 allowlisted placeholders；启动时缺包如何 fail closed | PromptPackage 格式、生成器、embed/load port、发布和回滚流程 |
| AIX-D005 | P0 | Provider route capability | 是否支持严格结构化输出、稳定 InvocationID 幂等重放、按 request ID 查结果、超时/取消、token 上限、确定模型版本和数据保留要求 | Provider port、route capability schema、错误映射和 adapter contract tests |
| AIX-D006 | P0 | 事件与异步结算 | requested、generated、failed、retry 的名称和 payload；手动触发事务与 outbox；Worker ACK/NACK；automation 超时后由谁结算 | `events.yaml` 契约、时序图、事务表和崩溃窗口矩阵 |
| AIX-D007 | P0 | 持久化与数据治理 | 运行与评测集合的 PO、唯一索引、CAS、事务、TTL；规范 Input 如何最小化保存和授权访问；真实输出保留多久；备份和导出如何处理 | Mongo 设计、migration、数据分类与 retention policy |
| AIX-D008 | P0 | 失败、重试与未知结果 | 每个 failure code 是否可重试、最大次数和 backoff；`result_unknown` 如何人工恢复；retry 与 regenerate 的差异；是否允许取消 | Failure/RetryDecision 表、状态机、runbook 和故障注入矩阵 |
| AIX-D009 | P0 | 输出与安全校验实现 | Schema、ref、Profile、safety 四类 validator 的确定性范围；方向性主张怎样与标准事实核对；Provider refusal 和内容安全拒绝如何展示 | validator contracts、版本收据、规则库边界与负例套件 |
| AIX-D010 | P1 | Profile 发布治理 | 初始 Profile 如何 bootstrap；谁可 publish/disable；如何用索引和发布校验消除同级歧义；历史 Profile 如何持续解析 | 发布用例、治理权限、唯一性设计、rollout/rollback 流程 |
| AIX-D011 | P1 | 授权、滥用与成本控制 | participant delegated subject 如何配置；每用户/Assessment/机构限流；并发和预算如何分配；供应商数据处理边界是什么 | AuthZ 矩阵、rate/cost/concurrency policy、供应商合规清单 |
| AIX-D012 | P1 | 可观测性与 SLO | 端到端时延、成功率、排队、校验失败、未知结果和成本的目标；哪些日志字段必须脱敏；告警和治理入口是什么 | 指标/日志/trace 规范、SLO、告警和运维查询设计 |
| AIX-D013 | P1 | Prompt 评测与发布门禁 | 如何区分 Slot、生成执行、Candidate 与裁判执行；怎样有界补样而不隐藏失败或替换低质量结果；Provider/模型/Prompt/Profile/Policy 变化怎样触发全量回归 | EvaluationExecutionPolicy、G1～G5 Gate、执行证据、review 记录和 schema |
| AIX-D014 | P1 | 客户端体验 | 按钮何时展示；pending 如何刷新；失败、重试、过期和标准/AI 内容如何区分；免责声明放在哪里 | Participant 状态/交互矩阵和端到端响应样例 |
| AIX-D015 | P1 | 灰度与生产验收 | feature flag 粒度、试点模型/人群、kill switch、质量抽检、成本观察和回滚阈值是什么 | rollout plan、production acceptance matrix 和 runbook |

### 17.1.1 当前实现进度（不等于门禁关闭）

| ID | 当前进度 | 仍未满足的关闭条件 |
| --- | --- | --- |
| AIX-D001 | Current source 已按 Catalog 精确 `SourceID` 解析；legacy fail closed；历史 Generation 返回 `current/stale/unavailable/unknown` | 真实 legacy/data 分布验证 |
| AIX-D002 | `Capability`、`Request`、`Get` 已实现，先授权、重复语义复用同一 Generation；REST/gRPC 状态已接线。Participant `reason_code` 已收敛为 `standard_report_not_ready`、`feature_disabled`、`source_not_supported`、`profile_unresolved`、`profile_mismatch`、`not_applicable` 机器枚举，并在 BFF 校验其与状态的合法组合。failed Participant Generation 只能由机构管理员提交带 expected attempt、稳定 request ID、成本确认和未知结果风险确认的 governed retry；Prompt 评测只允许在 Provider dispatch 前取消。Participant Generation 不提供用户取消：dispatch 后无法证明外部调用已停止，也不返还日预算 | 真实客户端兼容与端到端验收 |
| AIX-D003 | collection-server 提供 capability/request/get REST；apiserver 分离 participant 与 automation gRPC；delegated subject 和 default-deny ACL 已接线 | 真实身份灰度、前端隐藏/展示与兼容验收 |
| AIX-D004 | 编译期 PromptPackage、固定指纹/blob SHA、规范 Markdown 漂移测试和安全渲染已实现 | 发布/回滚运维流程 |
| AIX-D005 | Provider/Route port、冻结 revision/fingerprint、one-shot executor 和 OpenAI/DeepSeek Structured Output Adapter 已实现；v6/v3 Responses 已发布，v7/v4 strict-tool 回归证据保留；服务端继续执行完整 Schema/领域校验 | DeepSeek 完整业务契约的稳定符合率、供应商幂等/回查/保留证据、生成与裁判故障域独立性 |
| AIX-D006 | requested/generated/failed wire contract、durable Outbox、Worker ACK/NACK、内部 Automation 和终态审计已接线 | 跨进程预发、MQ 重投与故障注入证据 |
| AIX-D007 | 运行时/评测/预算/活跃槽集合已有 PO/Mapper/Repository、CAS、事务提交器与 migration 25～33。Participant Generation 保存规范 Input，Artifact 保存通过校验的 Content，不保存渲染 Prompt 或 Provider 原始正文；Prompt 评测为审计目的保存生成 raw/normalized output，按 365 天保留。生产 v2 已在原 Run 集合中按明确版本保存 Slot、两类 Execution、receipt、raw/normalized output 和 typed failure，旧 v1 记录只读。16 个历史 Run 和 525 次 Generation 的生产只读 BSON/输出分布已取得。系统不引入应用层数据密钥；生命周期、TTL、Artifact 导出和 Replica Set 集成测试已有仓库证据 | 包含 Semantic Execution 的完整 v2 BSON/输出复测、容量并发竞争、lifecycle backfill、备份恢复、真实授权/大数据量导出和生产 TTL 行为 |
| AIX-D008 | Participant requested 重投可恢复过期 prepared lease；默认关闭的 HA scanner 会把精确 Run/lease/phase wake-up 与 Outbox 原子提交，Worker 校验证明后恢复同一 attempt；非幂等 route 的过期 dispatch 落 `provider_result_unknown` 且不重放。failed Run 已实现仅限机构管理员的人工 governed retry，完整授权、每 attempt 日预算与 durable retry event 原子提交，未知结果需显式接受重复调用/计费风险，Worker 校验 exact authorization 后再竞争活跃槽，硬上限 3 attempt；Prompt 评测已实现 start/step checkpoint、过期 lease 人工恢复和 dispatch 前取消，并增加默认关闭的 prepared-only HA scanner。Scheduler Runbook 已明确启停、取证、`result_unknown` 人工处置和告警基线 | 两类 scanner 的目标环境告警落地与真实 Mongo/MQ 故障演练；自动 retry 仍不启用 |
| AIX-D009 | strict JSON、typed schema、ref、Profile、层级/数量、确定性 safety gate 与独立 semantic evaluator adapter 已实现并部署；裁判 Prompt/Schema/Route 均冻结并校验。裁判 Provider、result unknown、missing/size、Schema、Decode、Receipt 和 Decision 失败已拆为 7 个稳定码；生产 v2 Run 已保存 35 份 Semantic 证据，治理台可以读取 raw/normalized/receipt | 生成内容契约低基数子码、单条生产 Recheck、真实裁判方向性/新增事实结果和实际人工复核 |
| AIX-D010 | Profile draft/publish/disable governance service 与内部 REST/OpenAPI 已实现；mutation 要求 `CapabilityOrgAdmin`，actor 从 JWT user ID 推导；draft 重算指纹并保存创建审计；publish 强制绑定 approved PromptEvaluationRun、校验 release identity，并以预查 + published selector-slot partial unique index 拒绝同 specificity 冲突；首个 suite 治理通用 `participant-scale-score-range-default/v1` | bootstrap/rollout/rollback、真实身份联调与真实发布演练 |
| AIX-D011 | 当前 Prompt 评测每机构最多 1 个 collecting Run；v2 按冻结 `EvaluationExecutionPolicy.WorstCaseProviderCalls()` 预留 140 次最坏调用，生产机构 UTC 日预算为 1024。Participant global/user token bucket、机构/用户/Assessment 日预算和活跃槽已实现 | 余额处置、token/金额/历史预算、供应商数据处理清单和真实竞争验收 |
| AIX-D012 | 请求、容量、lease recovery、Provider 调用、token、校验和 Prompt 评测 `stage/code` 低基数指标已实现，Mongo ledger 作为预算事实；v2 管理投影已增加 Slot、Generation/Semantic Execution、Gate 进度和 typed failure | 分层 Dashboard、route revision 对比、金额 cost、SLO、部署告警、脱敏 trace/log、历史/跨机构治理和生产阈值验证 |
| AIX-D013 | 历史 v1 Run `635231960356106798` 证明任一技术失败会冻结全部审核，且 `AttemptRecord` 混合样本、生成和裁判语义。轻量 v2 已部署 Semantic 完整 Outcome、固定 35 Slot、两类有界 Execution、Candidate 不可替换、`result_unknown` 人工确认、v1 只读、Policy 总预算、Outbox/CAS、统一双角色审核、可靠性 Gate、管理 API 和 Profile 仅接受 approved v2 Evidence；生产 Run `635356837083886126` 已完成 35 Candidate/35 Semantic 并进入待审 | 完整 v2 BSON/输出复测、生产 Recheck、70 条实际复核、首份 v2 approved Run 和生产 Profile |

AIX-D010～D015 的生产与运营证据仍保持开放；D001～D009 也只代表代码边界已有首版，不代表真实环境或生产门禁关闭。AIX-D010/D013 的结构落地也不是实际发布证据。
尤其没有 approved PromptEvaluationRun 和已发布 Profile 时，即使配置 Provider 也只能返回 `profile_unresolved`。

### 17.2 生产启用前必须关闭

以下事项决定发布正确性，必须先于功能开关启用：

```text
AIX-D005 真实 Provider/model/schema 与数据保留证据
AIX-D007 生产保留期限、历史 backfill、备份/导出与容量治理
AIX-D008 周期恢复、人工处置与 retry governance
AIX-D010 Profile 发布治理
AIX-D011 成本与滥用控制
AIX-D012 metrics/SLO/告警
AIX-D013 Prompt 评测、semantic evaluator 与人工复核
AIX-D014 客户端状态和免责声明
AIX-D015 灰度、kill switch 与生产验收
```

已经落地的 Schema、单测、ACL、Adapter 和分支 Replica Set 测试只能证明各自层级；它们不能替代真实模型质量、生产数据治理、隐私合规或生产验收。

### 17.3 剩余交付顺序与文档预算

当前仓库的 active Markdown 受 `docs/document-closure.json` 硬上限约束，后续不能机械新增多份 AI 设计文档。
[AI 解读核心设计](./25-核心设计-AI解读.md) 已整合领域知识、Candidate/Execution 目标模型、失败分类、有界补样、发布 Gate、运行治理和未来演进；下一轮按以下顺序实施并同步本台账：

近期只维护[核心设计中的 12 项顶层验收](./25-核心设计-AI解读.md#9-顶层验收项)，具体测试映射放在实现 PR 和测试代码中。本清单只维护门禁是否关闭，不复制第二套验收口径。

1. Semantic Execution 完整证据、轻量 Slot、Policy 总预算、可靠性 Gate、v1 只读/v2 新写、Replica Set 测试和治理最小投影已合并并部署；
2. 历史生产 Run BSON 与 Generation 输出分布已经取得；包含 Semantic Execution 的完整 v2 余量仍待修复审计上传路径后复测；
3. 远端 CI 和维护窗口部署已完成，Participant 继续关闭；
4. 完整生产 v2 Run 已进入 `awaiting_review`；单条生产 Recheck 仍是独立诊断证据缺口；
5. 完成 70 条双角色复核、首个 approved Evidence 和 Profile 发布；
6. 最后完成客户端、灰度、SLO/告警、导出/备份恢复和生产验收。

当内容稳定且单篇文档已经影响阅读时，可以把第 2～6 项拆成独立 canonical 文档，但必须同时完成以下任一动作：

- 合并、替换或归档现有 active 文档，为新文档释放预算；
- 经过仓库文档治理决策后调整 hard ceiling 和 exception，并重新通过 `make docs-check`。

不先创建空占位文档，也不通过把设计移到 archive 或普通临时目录来绕过 active 文档预算。

### 17.4 当前明确冻结的非目标

在 v1 完成并取得真实评测、灰度和生产证据前，不启动：

- 多次测评趋势和跨报告综合；
- 用户画像、原始答案和第三方上下文接入；
- clinician audience；
- typology、behavioral rating、cognitive 等机制；
- Agent、Tool Calling、检索、长期记忆或多轮对话；
- Testee 删除传播、按主体擦除 sink 或删除状态机；
- 自由文本 fallback、自动修复 Prompt 和强制重新生成。

这些能力不是当前设计缺口，而是明确的产品非目标；未来需求出现时应建立新的契约、Profile 和评测范围。

- [查询模型、授权与 Audience 投影](./24-核心设计-查询模型、授权与Audience投影.md)
- [从 Outcome 到 InterpretReport](./30-关键链路-从Outcome到InterpretReport.md)
- [从报告查询到组合状态](./31-关键链路-从报告查询到组合状态.md)
- [ModelCatalog 设计问题与重构清单](../20-model-catalog/90-设计问题与重构清单.md)
- [Evaluation 设计问题与重构清单](../30-evaluation/90-设计问题与重构清单.md)

本文最终固化的治理边界是：

> Interpretation 不需要推倒重写。后续重构应先让每一份报告都建立在正确 Outcome、精确冻结输入和严格授权之上，再让每一次失败可解释、每一份历史成品可自证，最后根据真实容量和产品需求演进查询协议与报告类型。
