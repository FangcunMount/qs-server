# M4-B 四个历史索引保护工具

本工具仅保护 `answersheets`、`report_generations` 上四个具名历史索引，以及恢复时最后回放的原 `schema_migrations` 集合文档、options 和索引。工具不导出业务文档正文、不提供任意 DDL 或通用数据库恢复；完成后按 M7 退出源码树。新工具 source 与 B 执行器来源独立绑定。

B 固定为 `c6ad16329863a4aed7b8003d0c351db280be22a0`，282 个 up/down 资源摘要 `60a0242e235bf696cb1e10d3c7e994f52d3ab81c17df2bc5eab195f60c90c95b`。当前主线的后续 SQL102/103 不被当作 B 目标，原迁移字节不改。工具要求 MySQL8.0.36 clean101 和 Mongo7.0.31 replica set；原状态为 `101/39`，B 目标为 `101/40`。

| 集合 | 索引 | 原属性 |
| --- | --- | --- |
| answersheets | uk_answersheet_legacy_submission_event_id | unique；仅 string event_id 的 partial filter |
| answersheets | idx_answersheet_legacy_submission_audit | version、deleted_at、domain_id 有序复合键 |
| report_generations | uk_generation_historical_generated_event_id | unique；仅 string event_id 的 partial filter |
| report_generations | idx_generation_historical_generated_audit | version、deleted_at、status、domain_id 有序复合键 |

四个索引均无 TTL。原完整 ListIndexes BSON 与固定原038规格逐字段、逐字节核对，包括 key 顺序及整数编码；额外 TTL、hidden、sparse、其他 collation 或部分过滤条件均不接受。Mongo7.0.31 对 simple collation 的实际返回可省略 collation 字段，因此同时拒绝非 simple 的集合默认 collation，防止重建继承另一种语义。相关行为见 [MongoDB7.0 createIndexes 官方文档](https://www.mongodb.com/docs/v7.0/reference/command/createindexes/)。

## 调用与材料

构建时注入实际工具 source，不能使用未绑定的默认版本字符串：

```sh
go build -ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$(git rev-parse HEAD)" \
  -o /private/approved-tools/m4-mongo-historical-protection ./scripts/oneoff/m4_mongo_historical_protection
```

连接只从 `QS_M4_OBJECT_MYSQL_DSN`、`QS_M4_OBJECT_MONGO_URI` 读取，选中的数据库必须显式与 CLI 名称一致。不要把密码放入 argv 或日志。工具不猜其他账户或自动授权；权限不足、未知目标或身份变化均停止。

```sh
m4-mongo-historical-protection --mysql-database=qs --mongo-database=qs
m4-mongo-historical-protection --action=capture --mysql-database=qs --mongo-database=qs \
  --capture-file=/private/approved-protection/b-first-capture.json
m4-mongo-historical-protection --action=verify-target --mysql-database=qs --mongo-database=qs \
  --material-file=/private/approved-protection/b-first-capture.json --material-sha256=<externally-frozen-raw-sha256>
m4-mongo-historical-protection --action=restore --mysql-database=qs --mongo-database=qs \
  --material-file=/private/approved-protection/b-first-capture.json --material-sha256=<externally-frozen-raw-sha256> \
  --approval-file=/private/approved-protection/b-independent-approval.json --approval-sha256=<independent-approval-raw-sha256>
```

这些是未执行的调用示例。默认 inspect、capture 和 verify-target 都只读；只有显式 restore 写四个固定索引和原账本。捕获材料与批准使用绝对路径、0600、单硬链接、当前 UID 或 root、无符号链接。首次 capture 以 O_EXCL 创建、文件及目录 fsync、按原始 SHA 回读，不覆盖旧材料。材料原始 SHA 来自外部冻结；批准以另一个 O_EXCL `.consumed` 文件消费一次。

capture 在 SQL RR 只读事务读取严格单行 clean101，rollback 后再核对。Mongo 账本原 BSON和两集合精确 `$count` 在同一 snapshot session 读取，集合目录及索引在该快照前后独立读取并完整比较。游标须实际 EOF、ID0 和 Close 成功；不存在或失败的集合不能算作零。原 clean39 文档保留 `_id`、字段顺序及 BSON 整数宽度，账本只接受空 options、一个标准 `_id_` 索引及严格单行 typed version/dirty。业务集合保留 UUID、完整 options、全部完整索引及精确计数；材料没有业务正文或全量业务 hash。

MySQL 身份绑定实际 server UUID 和选中数据库；Mongo 稳定身份绑定实际 setName、排序后唯一成员和数据库。会被迁移 provider 合法重建的账本 UUID 单独观测，不拼入稳定集群身份。

## 恢复和启动边界

恢复批准绑定固定 source/resource、工具 source、材料原 SHA、writer-freeze 和责任回执、双库身份、实际捕获窗口、恢复决策、有效期与绝对截止时间。数据和恢复责任人为 `system (10001)`，材料至少保留30天；恢复截止不晚于决策后60分钟。这些是批准条件，不能据此宣称生产恢复预算已通过。

执行控制方负责 API1、Collection2、Worker3 六实例及其他相关写入者持续冻结，在首个新 runtime 启动命令发出前，以 O_EXCL 持久写入批准绑定的 first-runtime marker。工具不生成或删除该 marker。marker 缺失本身不证明停写；计数相同也不证明业务字段没有被更新。首个新启动命令已发出或有新持久事实之后，禁止回放旧材料，只能使用目标 schema 兼容版本向前恢复。

restore 接受实际 provider 产生的 dirty40 原 DROP 前缀0、1、2、3、4，或 clean40 且四个目标均缺失，SQL始终须 clean101。原索引、业务 UUID/options、精确计数和无关索引均须匹配；反向部分重建形状、已存在规格变化、额外索引或其他 head 拒绝自动推进。

批准再次核读并单次消费后，只按四个固定位置执行原始完整 createIndexes spec，随后完整核对索引、集合、计数、SQL身份和原40账本。全部通过后，在同一个显式 RS 事务内 DELETE 当前唯一账本文档、INSERT 捕获的原39 BSON，最后 COMMIT；不 Drop/Create 集合、不 Force、不构造 clean 文档、不直接清 dirty。成功后新快照须证明原39 BSON逐字相同，账本 UUID 仍是 provider40 已产生的当前 UUID。

只读请求、连接和选择服务器预算为5秒，总时限默认15分钟、最高20分钟。createIndexes 单独使用总 deadline、恢复绝对 deadline 和批准 expiry 三者最早边界的有限剩余预算，并设置 server maxTimeMS/writeConcern 等待；不被 URI 中的 client/socket 5秒超时隐性截断。DDL或最终提交结果未知时停止，不自动猜测完成或重跑。

应用只调用一次 CommitTransaction，不用 WithTransaction、不重跑整个事务、不由应用重新调用 commit。固定 driver v1.17.6 的 `mongo/session.go` 内 CommitTransaction 使用同一 lsid/txnNumber 的 `RetryOncePerCommand`；这不是新事务。任何最终返回错误仍是 unknown/complete=false。该 SDK 的 AbortTransaction 会吞 wire 错误，因此只记录应用 abort 已调用；未确认的 Mongo 事务释放与 SQL rollback 分开报告，不能以一次外部 ledger40 读取或 SDK nil 冒充服务端事务释放。只有明确 COMMIT ACK 才解除工具的 Mongo 事务 unknown 标记。

回执仅在全部操作、owned SQL Conn/pool（OpenConnections/InUse均0）、Mongo Disconnect、实际游标释放、文件落盘与总 deadline 都通过后 complete。EndSession 的字段仅表示已调用，不声称服务端确认；失败时计数不输出，主错误与释放错误分别保留。stdout 交付失败以非0退出。

## 验证状态

协议 source `ed97ae9a20aa369982c58d0a99cf18385f03fe8d` 的42项单测通过，只证明当时协议；当时 DB 入口尚未实现。只读 capture source `f7a23591e34c862bc745748e1a0315055b8d389a` 完成57项单测；恢复 source `857e9b5ca4f3eb6e09c8474579fb8bfc473486eb` 完成69项单测。Mongo 事务释放语义修正 source `3d0d3f4645740536f83cdafba9f83044567fb437` 的69项单测、race、vet、目标 lint 均通过；不可变私有 `qs-m4-b-object-tool-release-3d0d3f46/receipt.json` SHA为 `17b16c51bab54b2e3eace4c10da2448711c316dbbbe887433fe9fa9774af457f`。这些不是 native、生产权限或数据库操作验收。

新增 native 源 `68c2615970e56533b015e73ae224ca9d6b7f8a0b` 与 CI 接入 `33511c0751e14d08073e44aa1038f5a50384b011` 仍待真实依赖执行。CI复用已有 authenticated Mongo7.0.31 rs0 和独占 MySQL8.0.36，在 A 进程结束后串行运行 B。六个必跑顶层、恢复矩阵六个 child 与批准边界五个 child 必须全部实际 RUN/PASS，任一 SKIP/FAIL 或 selector 缺失均失败：

- 捕获、真实 provider40 完整升级及 clean10140 核对。
- dirty40 的0–4 DROP前缀及 clean40 全部退出后的对象恢复、真实原账本最后事务回放、当前40 UUID保留和 fresh10139 只读核对。
- 同计数的业务值冲突导致真实 unique index 重建失败，保留 dirty40，不能删业务文档解冲突。
- 账本事务写入中断、服务端 COMMIT 已成功但全部响应被丢失，以及同一事务重发摘要。
- first-runtime marker、外冻材料变化、过期批准、实际消费后再次使用、真实 SQL102 未来 head 拒绝。

原生测试独立读取全部合成 BSON、全部业务集合 UUID/options、无关索引及 SQL 账本进行比较；这些正文不进入工具材料。测试代理仅使用 owned loopback，记录成功回包和事务身份摘要，不写生产或更改真实请求。编译或内存 wire helper 测试不代替这些 native 用例。

目前未签生产 capture/持续冻结/权限、索引生产规模恢复时长、业务镜像安装、三服务启动、真实业务、RPO0或生产30/60分钟预算。无需完整业务文档备份的逐对象方案，不能被解释为没有持续冻结和逐对象责任。

当前33511c07的非DB检查实际完成普通69项/race、内存wire helper1项/race、integration编译、tagged vet、tagged lint0及configcontract；私有 `qs-m4-b-object-tool-native-source-33511c07/receipt.json` SHA为 `9a3184f59496454960aea5b97824d40a55c2d6a9f8fa813687a19d138709dba0`，真实native仍未执行。分支当前基于main3112，尚未包含独立的A超时修复PR372；正式发布PR之前必须整合其实际合并main、重新闭包和验收新head。
