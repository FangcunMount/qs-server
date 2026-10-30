# 热榜按日重建

该工具从完整 UTC+8 业务日的已受理答卷恢复 Redis 热榜。`index-spec`、`inspect`、`receipt` 只读；`prepare-index` 创建 Mongo 索引，`apply` 修改 Redis 投影。两个写模式均要求操作身份、原请求 UUID、预期 fingerprint 和持久审计目录；不发送 MQ、不执行计分或模型调用。

当前 `durable_acceptance.schema_version=2` 从 `rm_tx_execution` 读取冻结计划及精确已提交 witness，验证原始 wire 与答卷元数据。新版路径不读取、不创建 `rm_outbox`。缺少或冲突的凭证使整批失败，不降低统计总数。

版本 1 的历史答卷仍读取原 `rm_outbox`；这是历史专用读依赖。M5 删除该集合之前，必须完成历史责任处置并移除这条读取，不能仅因新版重建通过就直接删集合。

检查工具参数：

```sh
go run ./scripts/oneoff/hotrank_day_rebuild --help
```

连接参数仅从标准输入读取，不放入命令行。先在专用隔离测试环境执行 `inspect`，检查完整日 manifest，再用相同 fingerprint 与原请求身份执行 `apply`。工具在 apply 内先取得重建租约，再捕获新快照并核对 fingerprint；不会直接应用早于租约的 inspection 快照。日快照读取包括软删除答卷，超过 1000 条、索引缺失或为 partial/sparse 时整批拒绝。

不确定 apply 是否返回时，查询相同 request-id 的 `receipt`；不得改用新请求盲目重跑、清除去重键或增量补记。重复 apply 复用原回执，不覆盖修复后新到达的提交。`prepare-index` 和 `apply` 的现库执行需要单独运维授权；本次代码验收不连接或操作现有数据库、Redis。

验证：

```sh
go test ./internal/apiserver/infra/modelcatalog/hotrank ./scripts/oneoff/hotrank_day_rebuild
QS_SERVER_TEST_MONGO_URI='<disposable replica-set URI>' \
  go test -race -tags=integration -run '^TestHotRank(RebuildFromCurrentTransactionFactsWithoutOutbox|RejectsIncompleteCurrentTransactionDay|MixedHistoricalAndCurrentDay)$' \
  -count=1 ./internal/apiserver/infra/modelcatalog/hotrank
```

集成测试使用真实 Mongo 原事务和 SDK witness、自建随机数据库与 Redis 模拟器，验证重复重建、旧集合不存在、缺失及冲突证据、索引/完整日限制和 v1/v2 混合日。CI 显式要求三个测试通过且零跳过。Broker 故障、真实 Redis 与业务重投/重启属于后续 M3-C 验收，不将此测试或已有 infra 结果计作 M3-C 完成。
