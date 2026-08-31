# audit_ai_explanation_prompt_evaluation_size

AI 解读 Prompt Evaluation Run 的只读容量审计工具。

**不写入任何集合。** 工具读取 `ai_explanation_prompt_evaluations`，输出：

- Run BSON 的 P50、P95、最大值和最大 Run ID；
- generation raw/normalized output 的样本数、缺失数、P50、P95 和最大值；
- semantic raw/normalized output 的同类分布；
- 按 35 个固定 Slot、每个 Slot 最多 2 次 generation 和 2 次 semantic 执行推算的观测 P95/最大载荷。

推算只使用已观测输出大小，不等于发布许可。扫描被截断，或者 generation/semantic 任一输出没有样本时，`v2_observed_output_projection.available=false`，必须 fail closed。

## 用法

```bash
# 先执行有界探针
go run ./scripts/oneoff/audit_ai_explanation_prompt_evaluation_size/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --max-runs 1000

# 发布决策前全量扫描并保存机器可读证据
go run ./scripts/oneoff/audit_ai_explanation_prompt_evaluation_size/ \
  --mongo-uri "$MONGO_URI" --mongo-db qs --max-runs 0 --json \
  > prompt-evaluation-size-audit.json
```

不要把真实 URI、用户名或密码写入命令行历史、报告或仓库；生产环境通过受保护的 `MONGO_URI` 注入。

## 结果解释

- `run_bson` 使用 Mongo 返回的原始 BSON 字节长度，不使用 JSON 大小近似。
- `run_bson_without_stored_outputs` 将 generation/semantic raw 与 normalized 字段移除后重新编码，用于估计非正文开销。
- `missing` 是对应 Execution 没有该证据的数量，不能当作零字节样本。
- `v2_observed_output_projection` 使用当前最小策略的 70 次 generation + 70 次 semantic 上限；它没有伪造未来 Slot 元数据，仍需保留安全余量。

## 验证

```bash
go test ./scripts/oneoff/audit_ai_explanation_prompt_evaluation_size
git diff --check
```
