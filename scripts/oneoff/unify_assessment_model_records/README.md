# 统一 assessment model 记录迁移

该工具仅用于维护窗口。它把 `assessment_models` 的 draft head 与
`published_assessment_models` 的运行时快照合并为同一 collection 内的
`head` / `published_snapshot` 两类文档。

执行顺序：验证备份可恢复后依次运行 `dry-run`、`apply`、`verify`、
`cutover`；新二进制和缓存清理的冒烟验证完成后，再用 cutover 输出的
legacy collection 名称执行 `finalize`。

```bash
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode dry-run
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode apply
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode verify
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode cutover
go run ./scripts/oneoff/unify_assessment_model_records --mongo-uri "$MONGO_URI" --mongo-db qs --mode finalize --legacy-collection assessment_models_legacy_YYYYMMDD_HHMMSS
```

`finalize` 不可逆；只有在运行时、Redis 缓存和四类代表性模型冒烟均通过后执行。

`cutover` 后先部署新二进制，并清除 published-model 的 Redis 对象 key、目录 version token 和 collection-server 热缓存；不要让旧进程在新 schema 上继续服务。工具不连接 Redis，避免在备份/换名阶段引入第二个可变事实源，缓存失效由维护窗口的服务运维步骤执行。
