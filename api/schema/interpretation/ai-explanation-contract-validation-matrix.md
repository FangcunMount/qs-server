# AI Explanation 契约验证矩阵

## 结论与状态

这组 v1 契约定义的是“用户手动触发、仅基于本次标准报告、一次性生成跨维度综合洞察和结果相关建议”的能力。标准报告仍是权威结果，AI 输出只是可独立失败、重新生成和审计的补充解读，不参与评分、等级判定或风险重分类。

截至 2026-08-31，Input/Output/Profile v1、Participant 主链和旧版发布评测运行时已经实现；治理与评测可以在生产关闭用户流量的前提下运行，`participant_enabled=false`。本轮新增四份发布治理机器契约及对应领域值对象，但 `PromptEvaluationRun`、Mongo 持久化、应用编排和治理台尚未切换到 `PromptEvaluationEvidence v2`。旧 `AttemptRecord` 继续只读兼容，不能被静默解释成 Candidate/Execution 新证据。本文不构成首个 Profile 已发布或用户能力可用的证明。

规范性文件：

- [AIExplanationInput v1](./ai-explanation-input-v1.schema.json)
- [AIExplanationOutput v1](./ai-explanation-output-v1.schema.json)
- [AIExplanationProfile v1](./ai-explanation-profile-v1.schema.json)
- [AIExplanationSemanticEvaluationOutput v1](./ai-explanation-semantic-evaluation-output-v1.schema.json)（仅用于合成发布评测，不是 participant 输出）
- [AIExplanationEvaluationExecutionPolicy v1](./ai-explanation-evaluation-execution-policy-v1.schema.json)
- [AIExplanationReleaseGatePolicy v1](./ai-explanation-release-gate-policy-v1.schema.json)
- [AIExplanationFailureTaxonomy v1](./ai-explanation-failure-taxonomy-v1.schema.json)
- [PromptEvaluationEvidence v2](./prompt-evaluation-evidence-v2.schema.json)

本文中的 `MUST`、`MUST NOT`、`SHOULD` 是实现约束。Input、Output 和 Profile 三份业务 JSON Schema 是用户解读字段结构的唯一机器事实来源；Semantic Evaluation Output Schema 单独约束内部模型裁判输出；四份治理契约负责冻结评测执行预算、G1～G5 门禁、失败分类和发布证据。本文负责补充跨对象、语义、安全和运行时约束。

## 责任边界

```text
immutable standard report + EvaluationOutcome runtime facts
                         │
                         ▼
          published AIExplanationProfile
                         │
                         ▼
              Input Assembler / Validator
                         │
             ┌───────────┴───────────┐
             │                       │
             ▼                       ▼
 server-side input snapshot     provider payload
 source + profile + context     context + facts only
 + facts                               │
                                       ▼
                              one-shot model call
                                       │
                                       ▼
                              Output JSON Schema
                                       │
                                       ▼
                     reference + profile + safety validation
                                       │
                                       ▼
                          separate immutable artifact envelope
```

Provider payload 只能包含 `context` 和 `facts`。`source`、Profile 标识与指纹、内部对象 ID、鉴权信息、Provider 凭据均不得发送给模型。输入组装器不得查询或拼接历史测评、原始答案、用户身份信息、自由文本画像或本次标准报告以外的业务数据。

## Schema 级验证

| ID | 对象 | 必须成立的规则 | 责任组件 | 失败代码 |
| --- | --- | --- | --- | --- |
| INPUT-001 | Input | `schema_version` 必须为 `ai-explanation-input/v1`，所有对象拒绝未声明字段 | JSON Schema validator | `input_schema_invalid` |
| INPUT-002 | Input | `source.report_type` 必须为 `standard`，且报告来源、模板、构建器、内容版本和生成时间完整 | JSON Schema validator | `input_schema_invalid` |
| INPUT-003 | Input | `profile` 必须固定到明确的 Profile ID、版本和 SHA-256 指纹 | JSON Schema validator | `input_schema_invalid` |
| INPUT-004 | Input | `context.scope` 必须为 `current_assessment_only` | JSON Schema validator | `input_schema_invalid` |
| INPUT-005 | Input | v1 audience 只能是 `participant` | JSON Schema validator | `input_schema_invalid` |
| INPUT-006 | Input | `assessment_result_only` 不接受 focus area；启用 focus area 时至少 1 个、最多 3 个 | JSON Schema validator | `input_schema_invalid` |
| INPUT-007 | Input | 至少提供 2 个、最多提供 50 个维度，维度必须使用 `dimension:*` 引用 | JSON Schema validator | `not_applicable` / `input_schema_invalid` |
| INPUT-008 | Input | v1 模型类别只能是 `scale`，决策类型只能是 `score_range`；未来类型必须发布新契约版本 | JSON Schema validator | `input_schema_invalid` |
| INPUT-009 | Input | 标准建议必须使用 `suggestion:*` 引用；分数、等级、常模上下文均保持结构化 | JSON Schema validator | `input_schema_invalid` |
| INPUT-010 | Input | Input 不存在用户身份、联系方式、原始答案、历史测评或任意扩展字段入口 | JSON Schema validator | `input_schema_invalid` |
| OUTPUT-001 | Output | `schema_version` 必须为 `ai-explanation-output/v1`，根对象和所有子对象拒绝未声明字段 | JSON Schema validator | `output_schema_invalid` |
| OUTPUT-002 | Output | 必须包含摘要、至少 1 条综合洞察、至少 1 条建议和至少 1 条局限说明 | JSON Schema validator | `output_schema_invalid` |
| OUTPUT-003 | Output | 洞察类别只能来自 v1 固定枚举 | JSON Schema validator | `output_schema_invalid` |
| OUTPUT-004 | Output | 每条洞察必须声明 2 到 6 条 evidence reference | JSON Schema validator | `output_schema_invalid` |
| OUTPUT-005 | Output | evidence kind 与 ref 格式必须一致：维度、总体结果、模型结果或标准建议 | JSON Schema validator | `output_schema_invalid` |
| OUTPUT-006 | Output | 建议来源只能是 `standard_derived` 或 `generated_low_risk` | JSON Schema validator | `output_schema_invalid` |
| OUTPUT-007 | Output | `standard_derived` 建议至少引用 1 条标准建议 | JSON Schema validator | `output_schema_invalid` |
| OUTPUT-008 | Output | 输出没有分数、等级、风险等级、诊断、治疗方案等结构化字段 | JSON Schema validator | `output_schema_invalid` |
| PROFILE-001 | Profile | `schema_version` 必须为 `ai-explanation-profile/v1`，所有对象拒绝未声明字段 | JSON Schema validator | `profile_mismatch` |
| PROFILE-002 | Profile | 状态只能是 `draft`、`published` 或 `disabled`，且版本和指纹必须明确 | JSON Schema validator | `profile_mismatch` |
| PROFILE-003 | Profile | v1 selector 必须固定为 `participant + scale + score_range`；可选 model code/version 用于解析精度，model version 非空时 model code 也必须非空 | JSON Schema validator | `profile_mismatch` |
| PROFILE-004 | Profile | 维度超过 Profile 上限时只能拒绝，v1 不允许静默截断 | JSON Schema validator | `not_applicable` |
| PROFILE-005 | Profile | 输入范围只能是 `current_assessment_only` | JSON Schema validator | `profile_mismatch` |
| PROFILE-006 | Profile | 每条综合洞察至少要求 2 个维度证据，且禁止因果断言 | JSON Schema validator | `profile_mismatch` |
| PROFILE-007 | Profile | 洞察/建议条数、建议动作数和允许类别必须有显式上限 | JSON Schema validator | `profile_mismatch` |
| PROFILE-008 | Profile | 七类禁止主张必须全部保留，不能由具体 Profile 放宽 | JSON Schema validator | `profile_mismatch` |
| PROFILE-009 | Profile | Profile 只能持有逻辑 Provider Route，不持有供应商名、模型 ID、Endpoint 或凭据 | Schema + profile publisher | `profile_mismatch` |

## 发布治理契约验证

| ID | 对象 | 必须成立的规则 | 当前代码责任 | 接线状态 |
| --- | --- | --- | --- | --- |
| POLICY-001 | Execution Policy | 固定 7 个 generation case、每 case 5 个 Slot、1 个 preflight；Candidate 只能选择 Slot 中第一个满足结构与内容契约的执行 | `EvaluationExecutionPolicy.Validate` | 领域已实现；Run 未接线 |
| POLICY-002 | Execution Policy | 样本目标与调用预算分离；生成预算至少覆盖 35 个 Candidate，裁判预算至少覆盖 35 份收据，最坏调用数为两类 Run 上限之和 | `RequiredCandidateCount`、`WorstCaseProviderCalls` | 领域已实现；容量账本未接线 |
| POLICY-003 | Execution Policy | `result_unknown` 必须人工确认；质量失败不能替换 Candidate；裁判失败不能触发重新生成 | Schema const + `EvaluationExecutionPolicy.Validate` | 已实现 |
| GATE-001 | Gate Policy | G1 冻结 Suite、Prompt、Profile、Schema、生成/裁判 Route 和 Execution Policy 指纹 | `ReleaseGatePolicy.Validate` | 领域已实现；Run 未接线 |
| GATE-002 | Gate Policy | G2 要求 35 个 accepted Candidate、每个 1 份完整裁判收据、无未处置 `result_unknown`、无预算越界 | `PromptEvaluationEvidenceV2.Validate` | 领域已实现；现行 Run 未接线 |
| GATE-003 | Gate Policy | G3 的三个分母分别固定为 dispatched Provider executions、definite-output generation executions 和 dispatched semantic executions；`result_unknown` 保留在基础设施分母 | `PromptEvaluationEvidenceV2.EvaluateGate` | 领域已实现；Runner 未接线 |
| GATE-004 | Gate Policy | G4 保留当前 4/5、32/35、单份语义最低分与总体均分；低质量 Candidate 不得补样替换 | `PromptEvaluationEvidenceV2.EvaluateGate` | 领域已实现；Runner 未接线 |
| GATE-005 | Gate Policy | G5 要求每个 Candidate 两个角色、两个不同 reviewer、理由必填，共 70 条；任一拒绝导致发布失败 | `PromptEvaluationEvidenceV2.EvaluateGate` | 领域已实现；现行审核服务未接线 |
| FAILURE-001 | Failure Taxonomy | 每条失败同时具有 `stage/kind/code/retryable/result_unknown/disposition/evidence_refs`，且 `retryable` 不直接授权新调用 | `ClassifiedFailure.Validate` | 已实现 |
| FAILURE-002 | Failure Taxonomy | 输出契约失败只能补 generation；semantic execution 失败只能补裁判；quality failure 保留 Candidate；result unknown 进入人工确认 | JSON Schema conditional + `ClassifiedFailure` | 已实现 |
| EVIDENCE-001 | Evidence v2 | 固定 35 个 Slot，Generation Execution、Candidate、Semantic Execution、Review 和 GateResult 分开持久化 | `PromptEvaluationEvidenceV2.Validate` | 领域已实现；Mongo/Application 未接线 |
| EVIDENCE-002 | Evidence v2 | Slot 只能接受第一个 contract-conformant generation execution；Candidate 只能接受第一个完整 semantic execution | `validateCandidateSlots` | 已实现 |
| EVIDENCE-003 | Evidence v2 | 所有执行均受 Policy 上限约束并保留；增量人工审核只能针对已有完整裁判证据的 Candidate | `validateGenerationExecutions`、`validateSemanticExecutions`、`validateCandidateReviews` | 已实现 |
| EVIDENCE-004 | Evidence v2 | `result_unknown` 执行永久保留；Run 必须 blocked，只有显式承认重复调用/计费风险的 resolution 才能清除 unresolved 计数 | `ResultUnknownResolution`、状态迁移历史 | 已实现 |
| EVIDENCE-005 | Evidence v2 | v1 `AttemptRecord` 不自动迁移为 v2；迁移必须通过显式 mapper/版本化持久化和兼容测试 | repository/application migration | 未实现 |

## 跨 Schema 与语义验证

这些规则无法仅靠单份 JSON Schema 完成，必须在调用 Provider 前后由应用层执行。

| ID | 阶段 | 必须成立的规则 | 建议验证方式 | 失败代码 |
| --- | --- | --- | --- | --- |
| SEM-001 | 选择 Profile | 只解析 `published` Profile；`draft` 和 `disabled` 不可执行 | Profile repository query + domain invariant | `profile_unresolved` |
| SEM-002 | 选择 Profile | 优先级固定为精确 model code + version、model code 默认、model kind 默认；同一优先级多条命中必须拒绝 | Profile resolver unit/integration test | `profile_mismatch` |
| SEM-003 | 选择 Profile | `context.audience`、model kind、decision kind、model code/version 必须与已选 Profile selector 一致 | Input assembler comparison | `profile_mismatch` |
| SEM-004 | 构造 Input | `profile.profile_fingerprint` 必须等于已发布 Profile 规范化 JSON 的 SHA-256 | Canonical JSON fingerprint check | `profile_mismatch` |
| SEM-005 | 构造 Input | `decision_kind` 必须来自与报告关联的不可变 EvaluationOutcome 运行描述，不得从报告文案猜测 | Repository provenance test | `input_schema_invalid` |
| SEM-006 | 构造 Input | 维度 code/ref、标准建议 ref 在单次 Input 内必须唯一 | Set-based semantic validator | `input_schema_invalid` |
| SEM-007 | 构造 Input | `parent_ref`、维度中的建议引用以及建议中的维度引用必须能在同一 Input 解析 | Reference graph validator | `input_schema_invalid` |
| SEM-008 | 构造 Input | eligible/excluded dimension code 不得重叠；实际维度数必须同时满足 Profile 的最小值和最大值 | Profile publisher + input validator | `not_applicable` |
| SEM-009 | 构造 Input | Profile 中所有 `min_*` 必须小于等于对应 `max_*` | Profile publisher validation | `profile_mismatch` |
| SEM-010 | 构造 Input | focus area 必须存在于 Profile allowlist；无 allowlist 时只能使用 `assessment_result_only` | Input validator | `input_schema_invalid` |
| SEM-011 | 构造 Input | `include_norm_context=false` 时所有 `norm_context` 必须为 `null`；`include_model_result=false` 时 `model_result` 必须为 `null` | Input validator | `input_schema_invalid` |
| SEM-012 | 校验 Output | 所有 evidence ref 和 `source_suggestion_refs` 必须能在本次 Input 中解析 | Reference graph validator | `output_reference_invalid` |
| SEM-013 | 校验 Output | 每条综合洞察必须引用至少 Profile 要求数量的不同维度，而非重复同一 ref 或用总体结果凑数 | Distinct dimension-ref count | `output_policy_violation` |
| SEM-014 | 校验 Output | 洞察类别、数量和每项维度数必须落在 Profile 限制内 | Profile-aware output validator | `output_policy_violation` |
| SEM-015 | 校验 Output | 建议 origin、category、数量和 actions 数必须落在 Profile 限制内 | Profile-aware output validator | `output_policy_violation` |
| SEM-016 | 校验 Output | `standard_derived` 只能改写/组合已引用的标准建议，不能改变其方向或含义 | Rule-based check + content safety review | `output_policy_violation` |
| SEM-017 | 校验 Output | `model_result` 为 `null` 时输出不得引用 `model_result` | Reference graph validator | `output_reference_invalid` |
| SEM-018 | 校验 Output | 父子维度能否出现在同一洞察中必须服从 `hierarchy_policy` | Dimension graph validator | `output_policy_violation` |

## 运行时、安全与审计验证

| ID | 必须成立的规则 | 验证证据 | 失败代码 |
| --- | --- | --- | --- |
| RUNTIME-001 | 仅允许拥有本次标准报告读取权限的调用者手动触发；不能凭任意 report ID 越权生成 | API authorization integration test | API authorization error |
| RUNTIME-002 | 标准报告生成成功且不可变后才能触发；AI 失败不得修改报告状态或阻断标准解读 | Application state-machine test | `not_applicable` / AI run failure |
| RUNTIME-003 | Input assembler 的仓储调用只读取目标报告及其 Outcome/Profile，不读取答卷、用户档案或历史报告 | Repository spy/contract test | `input_schema_invalid` |
| RUNTIME-004 | 发给 Provider 的对象只能是 `{context, facts}`，不得包含 `source`、`profile`、内部 ID、凭据或调用者身份 | Provider adapter serialization test | `safety_rejected` |
| RUNTIME-005 | Provider 原始输出必须先通过 Output Schema，再通过引用、Profile 和安全校验；任一步失败都不得发布 Artifact | Application integration test | 对应 validation code |
| RUNTIME-006 | 输出不得形成诊断、病因/因果、用药、治疗方案、风险重分类、身份推断或确定性未来预测 | Deterministic safety rules + sampled evaluation | `safety_rejected` |
| RUNTIME-007 | 日志、Trace、Metric 不记录完整 Input/Output、个人数据或 Provider 凭据；审计只保存必要 ID、版本、指纹、状态和错误分类 | Logging/telemetry test and review | `safety_rejected` |
| RUNTIME-008 | Artifact envelope 固定 source report、outcome、profile/prompt/schema/safety 版本、Provider 路由解析结果、生成时间和校验状态；Output 本身不承载这些字段 | Persistence mapper/integration test | AI run failure |
| RUNTIME-009 | 相同请求的并发手动触发必须受幂等键或运行中唯一约束保护；重试创建新 attempt，不覆盖成功 Artifact | Concurrency/integration test | conflict or existing result |
| RUNTIME-010 | Provider 超时、限流、不可用和无效输出均是 AI Explanation 自身失败，可重试但不回滚或降级标准报告 | Failure-path integration test | AI run failure |

## 错误分类

| 错误代码 | 含义 | 是否可重试 |
| --- | --- | --- |
| `input_schema_invalid` | 服务端组装出的输入不满足 v1 结构或事实完整性要求 | 否；修复数据或实现 |
| `profile_unresolved` | 没有可用于该报告和 audience 的已发布 Profile | 否；发布 Profile 后重试 |
| `profile_mismatch` | Profile 本身非法、选择歧义、指纹不符或与 Input 不匹配 | 否；修复 Profile |
| `not_applicable` | 当前报告不满足 v1 跨维度解读资格 | 否；对当前报告是终态 |
| `output_schema_invalid` | Provider 输出不是合法的 `AIExplanationOutput v1` | 是；受重试策略限制 |
| `output_reference_invalid` | 输出引用了本次 Input 中不存在的事实 | 是；受重试策略限制 |
| `output_policy_violation` | 输出超出 Profile 的洞察或建议策略 | 是；受重试策略限制 |
| `safety_rejected` | 输出、Provider Payload 或可观测性数据违反安全边界 | 默认否；人工评估后处理 |

鉴权失败、并发冲突、Provider 超时/限流和持久化失败沿用 API、运行编排和基础设施错误体系，不伪装为契约校验错误。

## Profile 解析与不可变性

Profile selector 的解析优先级必须固定：

1. audience + model kind + decision kind + model code + model version；
2. audience + model kind + decision kind + model code，`model_version=null`；
3. audience + model kind + decision kind，`model_code=null` 且 `model_version=null`。

同一级别只能命中一个已发布 Profile。`published` 版本一旦被 Artifact 引用，除生命周期状态外不可修改；任何策略、Prompt、免责声明或路由变化都必须发布新版本和新指纹。`disabled` 只阻止新生成，不能破坏历史 Artifact 的回放与审计。

Profile 指纹算法固定为：从完整 Profile 中移除顶层 `fingerprint` 和 `status`，使用 RFC 8785 JSON Canonicalization Scheme 得到 UTF-8 字节，对其计算 SHA-256，并编码成 `sha256:<lowercase-hex>`。因此发布后的策略内容不可变，而 `published -> disabled` 的生命周期转换不会改变既有 Artifact 引用的指纹。

Provider Route 是服务端配置中的逻辑键。Profile 发布器必须拒绝看起来像供应商模型名、URL、API Key 或其他秘密的值；实际供应商、模型、超时、重试和凭据由基础设施配置解析并记录在 Artifact envelope 的审计投影中。

## Artifact envelope（不属于 Output v1）

持久化对象至少应记录：

- explanation ID、source report ID、outcome ID；
- Input Schema、Output Schema、Profile、Prompt、安全策略的版本与指纹；
- audience、locale、focus area codes；
- 逻辑 Provider Route 及实际解析版本的非秘密审计标识；
- attempt、状态、生成时间、完成时间、错误分类；
- Input snapshot 的保留期、访问控制策略和完整性摘要；
- 通过全部校验后才写入的 `AIExplanationOutput v1`。

Output 内容不包含上述生命周期字段，避免把模型生成内容与服务端事实混为一体。

## 本地验证命令

Schema 元模式验证：

```bash
python3 - <<'PY'
import json
from pathlib import Path
from jsonschema.validators import Draft202012Validator

for path in sorted(Path("api/schema/interpretation").glob("*.schema.json")):
    schema = json.loads(path.read_text())
    Draft202012Validator.check_schema(schema)
    print(f"ok: {path}")
PY
```

仓库契约和文档门禁：

```bash
go test -count=1 ./internal/pkg/contract
make docs-check
git diff --check
```

## 发布前最低验收

- 保持八份 Schema 的 strict-root、合法/非法 fixture、跨 Schema 引用和 Draft 2020-12 编译门禁；新增版本必须同步契约测试。
- 保持 `SEM-*` 的 Profile resolver、Input assembler、Output validator 与独立 semantic evaluator 单元测试，并用真实模型完成冻结 suite 的 35 次生成和 35 次裁判评测。
- 补齐 `RUNTIME-*` 的真实鉴权、跨进程数据读取边界、失败隔离、并发幂等和 Mongo Replica Set 持久化集成验收；新 Run 必须写 v2，旧 Run 必须保持只读可回放。
- 对每个已发布 Profile 建立固定输入回放集，验证引用完整性、低风险建议和七类禁止主张。
- 生产验收必须另行提供准确部署 SHA、实际 Profile/Prompt/Provider 路由版本、成功率、延迟、拒绝率和抽样安全评估；本文与仓库门禁均不证明生产已发布。
