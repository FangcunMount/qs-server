# EV-R009：可验证 InputSnapshotIdentity 实施说明

> 状态：已实施（2026-07-21）。当前系统只接受 `isn:v2`；本文不定义任何旧身份兼容路径。

## 目标

让写入 Run / Outcome 的 `input_snapshot_ref` 能证明「实际执行输入」未在重试间漂移，而不是仅 `model:code@version` 可读标签。

## 非目标

- 不把整份 InputSnapshot JSON 再存一份。
- 不 bump Outcome payload schema 大版本。
- 本批次若 200 字符列不够，另开迁移 PR 扩列；默认先用 compact digest 形式。

## 设计

### 1. 结构化身份

```text
InputSnapshotIdentity
  model_code / model_version / model_digest
  questionnaire_code / questionnaire_version / questionnaire_digest
  answersheet_id / answersheet_digest
  algorithm_family / decision_kind
  definition_v2_digest
  norm_table / norm_version / norm_band          (optional)
  subject_digest                                (显式编码年龄是否存在、年龄值和性别)
  composite_digest                              (hash over canonical field order)
```

对外仍写单一字符串，建议：

```text
isn:v2:<64-hex-composite-digest>
```

长度控制在 `input_snapshot_ref` VARCHAR(200) 内。

### 2. 写入点

- Claim / 物化成功后：由 [`input_snapshot_ref.go`](../../../internal/apiserver/application/evaluation/execute/input_snapshot_ref.go) 生成 identity，写入 EvaluationRun。
- Outcome commit：复制同一 ref，禁止二次物化后静默改写。
- automatic、manual 和 lease recovery：重新物化后必须与上一 attempt 的 v2 ref 完全一致；不一致即 validation/terminal。
- force：只允许 `isn:v2` → `isn:v2` 修订，并记录 previous/current ref、action request ID 和 attempt origin。
- 任意 v1、可读 label、畸形 v2 ref 均拒绝执行；运营侧删除旧 Assessment 后重建。

### 3. 稳定序列化

- 白名单语义字段；显式 field order。
- 禁止依赖 JSON map 遍历顺序。
- digest 算法固定（如 SHA-256 truncated hex）。

## 验收

- 任一成分变化 → 不同 ref。
- Run 与 Outcome 同 ref。
- 重试校验失败可观测。
- 只有严格的 `isn:v2:<64-hex>` 可执行。
- 普通重试发生输入漂移时终态失败。
- force 修订保留完整审计证据；旧 ref 即使 force 也拒绝。

## 实施顺序

1. Domain/port 定义 `InputSnapshotIdentity` + canonical digest helper + 单测。
2. `inputSnapshotRefFromResolvedInput` 只生成 v2，无法生成时 fail closed。
3. Engine/Worker 对普通重试做同一性校验，对 force 做 v2 修订审计。
4. 架构 ratchet 禁止 v1 生成器、识别逻辑和 label fallback 回归。

## 依赖

- EV-R008 已删除 CompatibilityResolver；InputSnapshot 必须携带冻结的 DefinitionV2 与精确运行身份。
- 不依赖 R010/R012 生产证据。
