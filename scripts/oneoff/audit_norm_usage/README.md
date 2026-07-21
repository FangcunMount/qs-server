# audit_norm_usage

MC-R020 首切片 A：只读 Norm 反向引用审计。

**不写入任何集合。** 引用事实源是 published `AssessmentSnapshot` 的 `definition_v2.calibration.norm_refs`；Norm 目录来自 `assessment_norms`。

## 报告段

| 段 | 含义 |
| --- | --- |
| `usages` | `NormTableVersion` → 引用它的 `code@version` / factor_codes |
| `dangling_refs` | 快照引用了不存在的 Norm 版本（数据完整性问题） |
| `unreferenced_norms` | Norm 存在但无任何 published 引用（仅提示） |
| `multi_version_snapshots` | 同一 snapshot 引用多个不同 Norm 版本 |
| `demographic_norms` | 含年龄或性别 Lookup 的 Norm 版本、相关 factor，以及引用它的已发布模型；用于 MC-R002 历史影响评估 |

## 用法

```bash
# 全量文本报告
go run ./scripts/oneoff/audit_norm_usage/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs_server

# JSON（便于机器消费）
go run ./scripts/oneoff/audit_norm_usage/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs_server --json

# 退役前点查某一常模版本
go run ./scripts/oneoff/audit_norm_usage/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs_server \
  --norm-version 'brief2-parent-2024' --json
```

## Exit code

| Code | 含义 |
| ---- | ---- |
| 0 | 扫描成功且无 dangling |
| 1 | 连接或扫描失败 |
| 2 | 扫描成功但存在 dangling NormRef |

## 非目标

- 不上 Norm draft/publish/archive 状态机
- 不写 Admin usage API / 不反向写入 Norm 聚合
- 不扫 draft working head（仅 published snapshot）
- 不改写或重算既有 Outcome；人口学常模清单仅提供只读影响证据
