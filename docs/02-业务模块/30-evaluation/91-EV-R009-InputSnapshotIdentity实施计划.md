# EV-R009：可验证 InputSnapshotIdentity 实施计划

> 独立于 EV-R008；本文件是下一编码批次的有界计划。状态：已实施（2026-07-21，见台账 EV-R009「实施结果」）。

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
  norm_table / norm_version / norm_band          (optional)
  subject_digest                                (optional)
  composite_digest                              (hash over canonical field order)
```

对外仍写单一字符串，建议：

```text
isn:v1:<composite_digest>
```

长度控制在 `input_snapshot_ref` VARCHAR(200) 内。

### 2. 写入点

- Claim / 物化成功后：由 [`input_snapshot_ref.go`](../../../internal/apiserver/application/evaluation/execute/input_snapshot_ref.go) 生成 identity，写入 EvaluationRun。
- Outcome commit：复制同一 ref，禁止二次物化后静默改写。
- 重试：重新物化后与原 Run ref 比对；不一致 → validation/terminal（沿用 R004 taxonomy）。

### 3. 稳定序列化

- 白名单语义字段；显式 field order。
- 禁止依赖 JSON map 遍历顺序。
- digest 算法固定（如 SHA-256 truncated hex）。

## 验收

- 任一成分变化 → 不同 ref。
- Run 与 Outcome 同 ref。
- 重试校验失败可观测。
- 旧 `model:` / `answersheet:` ref 读路径兼容，新写入只用 `isn:v1:`。

## 实施顺序

1. Domain/port 定义 `InputSnapshotIdentity` + canonical digest helper + 单测。
2. 替换 `inputSnapshotRefFromResolvedInput`；表征测试保护旧 fixture 读。
3. Engine/Worker 重试校验钩子。
4. 更新台账 EV-R009 → 已实现待验收。

## 依赖

- EV-R008 已收口 CompatibilityResolver（已完成编码）。
- 不依赖 R010/R012 生产证据。
