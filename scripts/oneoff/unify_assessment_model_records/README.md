# 统一 assessment model 记录迁移

该工具仅用于维护窗口。它把 `assessment_models` 的 draft head 与
`published_assessment_models` 的运行时快照合并为同一 collection 内的
`head` / `published_snapshot` 两类文档。

迁移目标是生成可被新运行时直接读取的 canonical collection，而不是修复所有
旧历史数据。工具会在报告中统计并跳过以下不兼容记录：缺少 code/version、
payload、payload format、decision kind、DefinitionV2、精确问卷绑定，或仍使用
`personality` 等旧 kind 的 model snapshot；缺少 code/version 的 questionnaire
head/snapshot。无可运行 head 的旧 active、以及可由当前 head 明确判定为旧版的
重复 active，会在临时 collection 中转为 archived。`dry-run` 和 `apply` 都不会
回写原 collection；被跳过的数据在 `finalize` 前仍保留在 legacy collection 中。

执行顺序：验证备份可恢复后依次运行 `dry-run`、`apply`、`verify`、
`cutover`；新二进制和缓存清理的冒烟验证完成后，再用 cutover 输出的
legacy collection 名称执行 `finalize`。

```bash
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode dry-run
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode apply
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode verify
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode cutover
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode finalize \
  --legacy-collection assessment_models_legacy_YYYYMMDD_HHMMSS \
  --legacy-questionnaire-collection questionnaires_legacy_YYYYMMDD_HHMMSS
```

`finalize` 不可逆；只有在运行时、Redis 缓存和四类代表性模型冒烟均通过后执行。
执行 `apply` 前必须人工确认 dry-run 输出中的 `skipped_*`、`archived_*` 和
`normalized_*` 数量符合预期，并且 `issues=0`。

`cutover` 后先部署新二进制，并清除 published-model 的 Redis 对象 key、目录 version token 和 collection-server 热缓存；不要让旧进程在新 schema 上继续服务。工具不连接 Redis，避免在备份/换名阶段引入第二个可变事实源，缓存失效由维护窗口的服务运维步骤执行。
