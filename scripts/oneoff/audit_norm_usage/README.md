# audit_norm_usage

MC-R020 首切片 A：只读 Norm 反向引用审计。

同一只读入口的 `--definition-projections` 模式用于 M5 Definition 派生层一致性审计。

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

## Definition 派生层审计

该模式扫描 `assessment_models` 的全部记录，包括 working head、published snapshot、软删除及 archived 保留历史。它使用 Mongo snapshot session，将计数与所有 cursor batch 固定到同一 cluster time，直接调用 canonical `DecisionSpecFrom` / `InterpretationAssetsFrom` 比较存储层。它不调用 stored-assets 优先读取，不经过 PO encoder 重新物化，也不提供写入、CAS、删除或修复选项。

```bash
# MONGO_URI 由私有环境文件提供，避免把密码放入命令行。
go run ./scripts/oneoff/audit_norm_usage/ \
  --definition-projections --mongo-db qs_server \
  --batch-size 100 --max-records 10000 --timeout 5m --json

# 或从只读挂载的私有 JSON 文件读取 mongo_uri / mongo_db。
# 文件必须是普通文件、仅 owner 可读写，最多 16 KiB；不接受符号链接。
go run ./scripts/oneoff/audit_norm_usage/ \
  --definition-projections --private-config /private/connection.json \
  --batch-size 100 --max-records 10000 --timeout 5m --json
```

模式始终输出 JSON，仅包含模型 ID、code、role、revision/release version、原始记录 fingerprint、canonical hash、状态、差异路径和计数。它不会输出报告正文、画像内容、连接 URI 或底层错误内容。每条记录最多展示 32 个差异路径，完整差异计数保留。集合的空值/空数组表达可以等价；数组顺序、重复 code、pointer 存在性、报告路由及资产内容不会被宽松归一化。

`batch-size` 限定 1–1000，`max-records` 限定 1–100000，`timeout` 必须大于零且不超过 5 分钟。禁止与 `--norm-version` 的部分过滤组合。带 credential 的 URI 必须来自环境或私有配置，`--help` 不打印环境中的 URI。审计扫描器只获得 `Find` 和 `CountDocuments` 两项读取能力。

| 输出 | 含义 |
| --- | --- |
| `complete` | cursor 正常结束，且实际扫描数与 snapshot count 相等，没有截断 |
| `zero_mismatch` | 非空、完整扫描，全部记录均匹配，且无缺失、未知 schema、invalid 或 error |
| `compared` | 已完成纯投影比较的记录数；缺失或无法验证的记录不会混入匹配数 |
| `mismatched` | 有实际派生字段差异的记录数；可同时具有 metadata invalid |
| `missing_definition` / `unknown_schema` / `invalid` | 均阻断 zero，不会按不存在问题处理 |
| `record_errors` / `scan_errors` | hash/扫描失败，均阻断 zero |
| `fingerprint` | 整条读取 BSON 的 SHA-256，与 ID/revision 绑定；不是写入凭据 |

| 模式 exit code | 含义 |
| --- | --- |
| 0 | 非空且完整，所有记录及 metadata 均一致 |
| 1 | 配置、连接、扫描或输出失败 |
| 2 | 完整扫描有差异、缺失、未知 schema、invalid/error，或集合为空 |
| 3 | 达到总量上限导致扫描不完整 |

已知 mapper 中 canonical typology profile 可包含 Rarity，而 derived `TypeProfilePO` 未保存该字段。审计保留纯投影的 Rarity，并报告这类存储损失，不通过二次 PO 编码隐藏差异。任何非 zero 结果都需要逐项核查；此工具不会覆盖现有 derived fields、公开契约、冻结报告或历史数据。

## 非目标

- 不上 Norm draft/publish/archive 状态机
- 不写 Admin usage API / 不反向写入 Norm 聚合
- 常模用量模式不扫 draft working head（仅 published snapshot）；派生层模式覆盖所有记录
- 不改写或重算既有 Outcome；人口学常模清单仅提供只读影响证据
