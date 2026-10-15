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

这些是未执行的调用示例。默认 inspect、capture 和 verify-target 都只读；只有显式 restore 写四个固定索引和原账本。捕获材料与批准使用绝对路径、0600、单硬链接、当前 UID 或 root、无符号链接。首次 capture 以 O_EXCL 创建、文件及目录 fsync、按原始 SHA 回读，不覆盖旧材料。capture在读取连接输入前与restore使用同一个父目录owner/permission守卫：必须root或当前UID拥有且不可group/world写；因此root不能先捕获到另一个UID拥有的目录后才在恢复时失败。材料原始 SHA 来自外部冻结；材料内部同时绑定首次 capture 的真实规范绝对路径，后续不能将同字节材料复制到别处后作为新恢复根使用。缺少该路径的旧材料明确拒绝。

capture 在 SQL RR 只读事务读取严格单行 clean101，rollback 后再核对。Mongo 账本原 BSON和两集合精确 `$count` 在同一 snapshot session 读取，集合目录及索引在该快照前后独立读取并完整比较。游标须实际 EOF、ID0 和 Close 成功；不存在或失败的集合不能算作零。原 clean39 文档保留 `_id`、字段顺序及 BSON 整数宽度，账本只接受空 options、一个标准 `_id_` 索引及严格单行 typed version/dirty。业务集合保留 UUID、完整 options、全部完整索引及精确计数；材料没有业务正文或全量业务 hash。

MySQL 身份绑定实际 server UUID 和选中数据库；Mongo 稳定身份绑定实际 setName、排序后唯一成员和数据库。会被迁移 provider 合法重建的账本 UUID 单独观测，不拼入稳定集群身份。

## 恢复和启动边界

恢复批准绑定固定 source/resource、工具 source、材料原 SHA、writer-freeze 和责任回执、双库身份、实际捕获窗口、恢复决策、有效期与绝对截止时间。数据和恢复责任人为 `system (10001)`，材料至少保留30天；恢复截止不晚于决策后60分钟。这些是批准条件，不能据此宣称生产恢复预算已通过。

执行控制方负责 API1、Collection2、Worker3 六实例及其他相关写入者持续冻结，在首个新 runtime 启动命令发出前，以 O_EXCL 持久写入批准绑定的 first-runtime marker。工具不生成或删除该 marker。marker 缺失本身不证明停写；计数相同也不证明业务字段没有被更新。首个新启动命令已发出或有新持久事实之后，禁止回放旧材料，只能使用目标 schema 兼容版本向前恢复。

restore 接受实际 provider 产生的 dirty40 或 clean40，缺失索引必须是原 DROP 前缀0、1、2、3、4，SQL始终须 clean101。逆序补回缺失索引，使每个实际成功ACK后仍是更短的缺失前缀；clean40 也接受这种已恢复后缀。每次继续须使用新的独立批准并保持原 first-capture、持续冻结和绝对恢复截止。首次批准在原材料的固定 parent 中，以材料 SHA 命名的 O_EXCL 文件持久冻结完整恢复边界；续跑只允许更换 operation，其他字段必须与首份结构完全相同，包括原 marker、决策、截止、expiry、停写与责任。批准消费文件由同一固定根和 operation 摘要定位，不依赖批准文件名或 JSON 空白，因此复制批准或重编码同一 operation 不能重复使用。边界缺失、损坏、未知、写入或 fsync 未证实时停止，既有边界只读核验并重新确认落盘，不覆盖或自动重建。 同一固定根还保留不可重新初始化的有界追加日志（最多128个operation、64KiB，每条只含operation、批准SHA和材料SHA），与每个claim双向核对。删除任何已用claim、日志缺失/为空/截断、未知claim或首次boundary/journal只保留一半时，同operation和新operation均拒绝；不会根据缺失状态重新初始化。日志追加使用非阻塞排他flock，持锁后再次比对原字节，再单次追加、文件及目录fsync、claim落盘并完整回读闭合后才允许可变阶段。部分写入、并发忙或释放失败保持unknown并保留状态。该机制防止单文件丢失重开授权，不承诺抵御有权恶意重写全部本地状态的root。原索引、业务 UUID/options、精确计数和无关索引均须匹配；非前缀缺失、已存在规格变化、额外索引或其他 head 拒绝自动推进。verify-target 仍只接受 clean40 且四索引全部缺失，不把部分或全部恢复的状态当作B安装成功。

批准再次核读并单次消费后，只按四个固定位置执行原始完整 createIndexes spec，随后完整核对索引、集合、计数、SQL身份和原40账本。全部通过后，在同一个显式 RS 事务内 DELETE 当前唯一账本文档、INSERT 捕获的原39 BSON，最后 COMMIT；不 Drop/Create 集合、不 Force、不构造 clean 文档、不直接清 dirty。成功后新快照须证明原39 BSON逐字相同，账本 UUID 仍是 provider40 已产生的当前 UUID。 每次新的索引写入、账本事务、DELETE、INSERT和最终 COMMIT 前都重新核对原 marker、首次边界和绝对期限；最后 SQL 身份/账本回读完成后，紧邻 COMMIT 发令再次核对，不能沿用该有界 RPC 等待前的检查。

只读请求、连接和选择服务器预算为5秒，总时限默认15分钟、最高20分钟。createIndexes 单独使用总 deadline、恢复绝对 deadline 和批准 expiry 三者最早边界的有限剩余预算，并设置 server maxTimeMS/writeConcern 等待；不被 URI 中的 client/socket 5秒超时隐性截断。DDL或最终提交结果未知时停止，不自动猜测完成或重跑。

应用只调用一次 CommitTransaction，不用 WithTransaction、不重跑整个事务、不由应用重新调用 commit。固定 driver v1.17.6 的 `mongo/session.go` 内 CommitTransaction 使用同一 lsid/txnNumber 的 `RetryOncePerCommand`；这不是新事务。任何最终返回错误仍是 unknown/complete=false。该 SDK 的 AbortTransaction 会吞 wire 错误，因此只记录应用 abort 已调用；未确认的 Mongo 事务释放与 SQL rollback 分开报告，不能以一次外部 ledger40 读取或 SDK nil 冒充服务端事务释放。只有明确 COMMIT ACK 才解除工具的 Mongo 事务 unknown 标记。

回执仅在全部操作、owned SQL Conn/pool（OpenConnections/InUse均0）、Mongo Disconnect、实际游标释放、文件落盘与总 deadline 都通过后 complete。EndSession 的字段仅表示已调用，不声称服务端确认；失败时计数不输出，主错误与释放错误分别保留。stdout 交付失败以非0退出。

## 验证状态

协议 source `ed97ae9a20aa369982c58d0a99cf18385f03fe8d` 的42项单测通过，只证明当时协议；当时 DB 入口尚未实现。只读 capture source `f7a23591e34c862bc745748e1a0315055b8d389a` 完成57项单测；恢复 source `857e9b5ca4f3eb6e09c8474579fb8bfc473486eb` 完成69项单测。Mongo 事务释放语义修正 source `3d0d3f4645740536f83cdafba9f83044567fb437` 的69项单测、race、vet、目标 lint 均通过；不可变私有 `qs-m4-b-object-tool-release-3d0d3f46/receipt.json` SHA为 `17b16c51bab54b2e3eace4c10da2448711c316dbbbe887433fe9fa9774af457f`。这些不是 native、生产权限或数据库操作验收。

native 源 `68c2615970e56533b015e73ae224ca9d6b7f8a0b` 与 CI 接入 `33511c0751e14d08073e44aa1038f5a50384b011` 定义以下必跑矩阵。CI复用已有 authenticated Mongo7.0.31 rs0 和独占 MySQL8.0.36，在 A 进程结束后串行运行 B。该历史26项清单由七个顶层、恢复矩阵六个 child、批准边界五个 child 与续跑八个 child 构成。当前源码新增首次恢复边界八个 child 及最后 SQL 回读 marker 顶层，共九个顶层和28个 child、37个必需事件；全部须实际 RUN/PASS，任一 SKIP/FAIL 或 selector 缺失均失败。此前26项证明不能覆盖新反例：

- 捕获、真实 provider40 完整升级及 clean10140 核对。
- dirty40 的0–4 DROP前缀及 clean40 全部退出后的对象恢复、真实原账本最后事务回放、当前40 UUID保留和 fresh10139 只读核对。
- 同计数的业务值冲突导致真实 unique index 重建失败，保留 dirty40，不能删业务文档解冲突。
- 账本事务写入中断、服务端 COMMIT 已成功但全部响应被丢失，以及同一事务重发摘要。
- first-runtime marker、外冻材料变化、过期批准、实际消费后再次使用、真实 SQL102 未来 head 拒绝。
- dirty40/clean40 各在第1–4个真实 createIndexes ACK后中断，旧批准复用无DDL拒绝，同一原材料和原绝对截止的新批准继续；四索引全恢复但账本尚未回放的中断也须通过。
- 实际ACK1中断后，复制批准路径、同operation重编码、延期、变更决策、更换已经存在的原marker、复制材料、删除或损坏首次边界，全部须具名拒绝且无新增DDL或账本写入。新增删除已消费claim后，同operation与新operation都须在日志闭合检查处拒绝。
- 账本INSERT实际ACK后，以自有SQL表锁阻挡正式最后回读，独立确认该SELECT正在等待时写原marker，再释放锁；必须无COMMIT、真实abort ACK、原40 BSON/UUID保持，公开事务释放仍为unproven。

原生测试独立读取全部合成 BSON、全部业务集合 UUID/options、无关索引及 SQL 账本进行比较；这些正文不进入工具材料。测试代理仅使用 owned loopback，记录成功回包和事务身份摘要，不写生产或更改真实请求。编译或内存 wire helper 测试不代替这些 native 用例。

目前未签生产 capture/持续冻结/权限、索引生产规模恢复时长、业务镜像安装、三服务启动、真实业务、RPO0或生产30/60分钟预算。无需完整业务文档备份的逐对象方案，不能被解释为没有持续冻结和逐对象责任。

历史33511c07的非DB检查实际完成普通69项/race、内存wire helper1项/race、integration编译、tagged vet、tagged lint0及configcontract；私有 `qs-m4-b-object-tool-native-source-33511c07/receipt.json` SHA为 `9a3184f59496454960aea5b97824d40a55c2d6a9f8fa813687a19d138709dba0`。这份检查当时基于main3112，未包含A超时修复，不能转签为后续来源。

首次真实 native 在 clean source `3b5faa8d4df5a4a03104cb3dcb1c22a2af03d32f` 执行：MySQL8.0.36/MongoRS7.0.31，17 RUN、5 PASS、12 FAIL、0 SKIP，exit1。不可变 `qs-m4-b-object-tool-native-v1/receipt.json` SHA为 `28bd0f6ea0e63930a1584982979a181214bad0a41839b07ad91a623e44373491`，原始 JSONL SHA为 `bbc894219f20109ecc6b326c97ab884f2c5ab69bad9ccf7b96c8cf9a7ab07206`。失败包括完整合成BSON比较和索引创建；当时未捕获 typed server code，不能将 generic错误定性为具体服务端原因。旧冲突用例仅证明失败，未证明11000；两项wire故障用例未到达目标事务/commit。两自有容器和网络均已释放，原CID/images/volumes保留；旧运行元数据变化，不签全环境稳定。

修正 source `535849914a3d719a6794889a6435986ecf725dc1` 和 `445815303cb9c659fa9e5d77a9dedb0522722843` 关闭两项已确认的代码问题：URI应用完成后显式清除 ClientOptions.Timeout 为 nil，避免 SetTimeout(0) 仍启用CSOT而与固定 maxTimeMS 冲突；native provider 使用真实当前版本，避免把39误作0准备而重新创建退役命名空间。前者曾有一次namespace常量的编译错误，后者已修正；这些静态因果不冒充首次未捕获的server错误码。source `3447cbded3352301c4378c2c7cd2fd8bedf959da` 增加仅本工具内的nil-default typed observer：CLI不装配，测试仅记录固定操作、数值code、闭集name和OK；BSON差异只记录集合、ID/raw摘要及键名/类型，绝不输出值或原始错误正文。冲突用例现须实际 typed11000，wire用例仍须到达原定故障点。

正常 merge `1356621084e26eaacd82cfd776a583920931a445` 已整合实际main `8f08f28fe395e11c55ac119f0490f539b6b30995` 的A超时修复，A工具文件逐字保持主线。该来源实际77项普通/race、2项内存native helper/race、integration编译、tagged vet、tagged lint0和configcontract通过；私有 `qs-m4-b-object-tool-diagnostic-source-13566210/receipt.json` SHA为 `998d5fb1f21c30884643b731e244d67cd956bd84bc9c04fdd041d0359a8ede97`。该非DB检查不代替后续真实native。

实际修正后 source `8eeaef32cafa9f17dacee7767d41ccd99f90683c` 已完成唯一第二轮 owned native：MySQL8.0.36/MongoRS7.0.31，六个顶层与十一个 child 全部17 RUN/PASS、0 FAIL/SKIP、exit0。不可变 `qs-m4-b-object-tool-native-v2/receipt.json` SHA为 `6faac785f89fa4a8e8b6ec9951e5add00c4489c0fe67642b1f4e37a477832de8`，原始 JSONL SHA为 `ef4b65fc8a92c4a049267676e3c368924093e08829a39a8560a8ecf45c502b60`，runner SHA为 `22ac7909475f4d315b2ae6b498a71093edda213833468664320759caa2e907ee`。这些文件保留实际完整argv、15个工具文件指纹和全事件，不能把未执行的规范化命令登记为通过。

全部 dirty0–4/clean4 恢复均逐字核对原合成业务BSON、UUID/options、无关索引和SQL账本，原39 BSON在显式事务最后回放，provider40账本UUID保留。冲突实际获得 typed11000/DuplicateKey及ok=false；事务写中断实际到达；服务端COMMIT两次成功响应均被丢弃，同一lsid/txnNumber摘要相同，应用仍unknown/不重跑。真实消费后重复使用和其余批准反例均通过。此轮81.608秒仅是小型合成矩阵总耗时，不能作为生产索引恢复或30/60分钟预算。两自有容器和网络精确释放，既有CID/images/volumes保留；动态运行元数据变化，不签全环境稳定。首次失败保持独立来源，当前只有组件证明；最终PR head还需fresh CI，生产与完整业务镜像安装未验收。

原head `da1f162735e7077c5e2cc06b814d48f86b2b4b87` 的审查发现恢复续跑缺口：按正序补回索引后中断会留下非前缀；clean40补回一部分或全部后、账本回放前中断也被旧校验拒绝。先前17项没有覆盖这些恢复中断，不作为该能力证明。source `5588d8ddd` 改为逆序重建并只在restore扩展clean40合法前缀；source `fe45c321f10cdebb8462e05b81f14e895b79523b` 加入八个真实ACK中断child和26项显式CI清单。新普通/race87项、内存helper/race2项、integration编译、tagged vet/lint0及configcontract通过，私有 `qs-m4-b-resume-source-fe45c321/receipt.json` SHA为 `fd215f8c9ec6fe1637887ec7a2f2c6ace13082a7c824302acc4a377ce7d413a6`。该source检查时26项真实native尚未执行；旧17项证明保留原来源，后续native与新headCI/审查分别登记。

修复后 clean checkout `0827df92685971aceb1fc5257f53bff7aa1e5af5`（Go/CI与fe45逐字相同）实际完成26 RUN/PASS、0 FAIL/SKIP、exit0，MySQL8.0.36/MongoRS7.0.31。不可变 `qs-m4-b-object-tool-native-v3/receipt.json` SHA为 `17382b370630cea9b14a1e32bd7515e915e34d3f3132fc737e871fddef3b56ec`，原始JSONL SHA为 `1877b7911e178efc8a92a5c45204a75b264892adbefbe2fa0242606d6fed91fc`，runner SHA为 `98bcbdc0b53920e2800e6eb53949c257bf81af28661bbc7130ce3cdcb8369435`。八个 dirty/clean ACK1–4中断均实际到达，独立核对实际40 BSON/UUID和原spec后缀，旧批准无DDL拒绝，新批准不改变原材料、恢复决策或绝对截止并完成账本最后回放；全部原17场景也再次实际通过。两自有容器和网络释放，原CID/images/volumes保留，动态运行元数据变化如实保留。113.907秒仅是这轮小型矩阵总耗时；生产、规模、30/60分钟和RPO仍未签。新PR head必须fresh CI并结清原P1审查，不能仅凭本地26项转签。

新批准边界 source `a872ac5a96476dcd71d70d8b9ebf759dfd99e6b7` 已完成精确101项普通/race RUN/PASS、0FAIL/SKIP、integration仅编译、tagged vet/lint0；私有 `qs-m4-b-boundary-source-a872/receipt.json` SHA为 `9254d13af5e3a253edc6d5547ba9294976e043a3518c18428ce4c073be719cb1`。独立 `fe03d8cc8` 关闭最后SQL回读的检查过时窗口；source `64f25149e6c0c0734ff7ca95a8d217eff62b2c6d` 加入新的真实反例、修正旧消费文件断言，并使CI从实际完整JSONL输出matched RUN/PASS names、FAIL/SKIP计数和实际编译工具source与checkout核对的紧凑摘要。本轮非DB QA在fe03的待提交fixture上执行，文件逐字等于64f提交：101普通/race通过，integration编译、tagged vet/lint0、actionlint带shellcheck和configcontract通过，receipt `qs-m4-b-boundary-fixture-source-64f25149/receipt.json` SHA为 `066b0659f5c0b6f393f29c8beb2e179d1f6b84876bb530a25d2501373fae71ec`。新增36项真实native、新head CI及两条批准P1审查尚未执行/结清；原26证明保持原来源，生产状态不提升。

CI-only source `727558f29dc0a65cfa6716826bdb4bdf4b13a288` 将既有共享 authenticated Mongo fixture 明确限制为 memory/swap各2GiB、WiredTiger cache0.25GiB，保留所有36个B必需名称及A/metadata路径。actionlint带shellcheck、configcontract通过；Go与a7逐字不变，纯资源准备不重跑普通测试。新本地v5准备同样只增加资源护栏：SQL1GiB/64MiB buffer/tmpfs1GiB、Mongo2GiB/0.25GiB cache/tmpfs1GiB+32MiB；MySQL初始化前真实读取VM内存并要求总量至少7.5GiB、可用至少4GiB，SQLready后再记一次当前安全整数才创建Mongo，两个实际HostConfig预算须回读一致。startup/native任一自有依赖停止或OOM即记录具名安全状态、停止并清理，不滚动重试；实际36native仍未执行，v4是保留的未执行准备包。

精确 clean checkout `fe17e2e8ca2716a1600664ffae3a2aed3d3a2629` 的新v6隔离验证已实际完成九个顶层、27个child，共36 RUN/PASS、0 FAIL/SKIP、exit0，MySQL8.0.36/MongoRS7.0.31。封存race测试二进制SHA为 `27c350e8bc4e6adcec448c5e9941fab31a303620e11dae6fd2ff7dd88816fcaa`，实际编译工具source同fe17，19个包文件前后保持。不可变 `qs-m4-b-object-tool-native-v6/receipt.json` SHA为 `1bf38ab5e32762f753760499ab957c0e7a66d0b7911790eb4531256d9ce83543`，完整JSONL SHA为 `870d1c33a542a04df640f8768e424442ad2c916cdf02faa536f571aad46365bd`，runner SHA为 `5f3cef95fdc2a864afe5437a34f2a6b1d9afd4c6d876caa02b9bda6de89f8340`，完整事件/关键见证交付SHA为 `19312e578573f75c7197665ec42299939b90dc3d1387236bc7f52838ada61d5b`。实际argv与私有路径保留在回执，不将未执行的规范化命令登记为通过。

八个新攻击均先实际createIndexes ACK1后取消，独立核对缺失前缀3和原40 BSON/UUID；复制批准、更换JSON编码或恢复范围、复制材料、删除/损坏边界均在新DDL或账本写入前具名拒绝。最后SQL回读反例实际等待自有metadata锁，在观察到正式SELECT等待后才创建原marker并释放锁，观察到COMMIT请求0、服务端abort ACK，公开Mongo事务释放仍为unproven。原26场景也全部重新通过，包括同绝对预算的八个合法续跑、typed11000冲突，以及服务端两次成功COMMIT响应全部被丢弃、同一事务摘要而应用结果unknown。

本次初始VM total为8,214,851,584字节、available为5,011,025,920字节，SQLready后available为4,494,671,872字节；实际HostConfig核对SQL1GiB与Mongo2GiB的memory/swap、CPU和tmpfs成功。测试进程组已回收且不存在，两自有容器和网络精确释放，旧CID/images/volumes保留，cleanup错误为空；一个旧容器的选定运行元数据变化，因此不签全环境稳定。148.186秒仅是小型合成矩阵总耗时，不能作为生产规模、RPO0或30/60分钟预算证明。

前一次唯一v5实际失败为2 RUN、0 PASS、2 FAIL、0 SKIP：封存测试二进制代替go test后，launcher遗漏包工作目录，原资源相对路径检查在工具连接前失败。v5回执SHA `4a71bebcb435e9957065a2a439ee2958ea2bffe288eb036c324d7d41599336b3` 及JSONL `b1bff80d6267b62e8e8dad7263726d1b2766b1e15ada9d048fa27915a62eabde` 原样保留；资源门槛通过、全部自有物料释放。v6只改启动cwd一行，Go和封存测试二进制未改，先在实际包目录重算282资源/60a024并执行既有不连库协议用例，再独立获准执行。v4仍是未执行准备，不能记为失败或通过。后续文档子提交只以Go/CI逐字等价继承组件证据；新PR head的CI/native和两条批准P1审查仍需独立完成，生产状态不提升。

实际7eb审查后补两项来源独立的保护：`383b45cfc` 将capture父目录守卫前置并与恢复共用；`58f88395eabf4e8f4c331445b435e690f04177aa` 增加固定日志与claim双向闭合。精确58f完成113普通/race RUN/PASS、0FAIL/SKIP、integration仅编译、tagged vet/lint0；不可变 `qs-m4-b-journal-source-58f88395/receipt.json` SHA为 `b6979d06d7605a51d0854e98cb03efcee73eaaaa133a535a4548f8a24664c54b`。其unit实际覆盖单claim删除、首次单边初始化、空/partial/后缀截断日志、非阻塞锁冲突及条数/字节界限。原36项只绑定fe17，不能覆盖这两项新修复。

夹具/CI source `99e1b9a89a6ebff5e296d51883f0d2be6ca083bc` 增加DeletedConsumedClaim真实ACK反例，使必跑总数为37；ACK计数异常时输出已有封闭typed observer字段，并在CI自有Mongo移除前记录固定State/Memory/Swap以及tmpfs安全整数或明确UNAVAILABLE。integration编译、tagged vet/lint0、configcontract和actionlint带shellcheck通过；新37项真实native与新head CI/AI尚未执行。旧7eb CI Mongo范围实际36 RUN、27 PASS、9 FAIL、0 SKIP，首失败为DirtyAfterACK3只观察到两个ACK及generic失败，后续primary不可用；原日志tail120k且truncated=true，尚无退出/OOM/空间证据，保持原因unknown，不由清理共用ctx耗尽推断SQL宕机。两个setup-go下载故障及其独立重试与该Mongo失败分别保留，未借本地36覆盖CI。

精确 clean checkout `872b3d7d4bcb3b7395fae027a62ebcfaefea16b3` 已完成修复后的唯一v7隔离验证：九个顶层、28个child，共37 RUN/PASS、0 FAIL/SKIP、exit0，MySQL8.0.36/MongoRS7.0.31。实际封存race二进制SHA为 `f92c23fb95ce88d5d3909ac0da3ed8110f093f9ef6f1dca7f791565c95a0a5e1`，编译工具source同872b，21个包文件运行前后保持。不可变 `qs-m4-b-object-tool-native-v7/receipt.json` SHA为 `8da9eb9fd8bd031e5b167b80ff2606497486d130cff3dd6639779768f09b0927`，完整JSONL SHA为 `17079e4c81f685cce2d497baf95f839c6a6265100ed11d6646da6fe4bc85f4c0`，runner SHA为 `078b59f33ee637b1e9468f9d742399dfc810fa95a0ae7fa4ef3def83de1b8698`，完整事件/见证交付SHA为 `e385ef13a3e85170da11dd3aeaa83626ce3375b7a61e45b4fa3db51ca15eec00`。实际argv保留在回执，不将示例命令或其他source改签为通过。

新增DeletedConsumedClaim在真实ACK1取消后删除已消费claim，同一operation和沿原绝对预算的新operation均被拒，未发新索引或账本写入、没有重建丢失claim。原八项ACK后批准攻击、八项合法续跑和其余旧矩阵再次实际通过；最后SQL锁等待期间创建原marker，观察到COMMIT请求0和上游abort ACK，公开释放仍为unproven。冲突实际typed11000/ok=false；同一事务的两次上游成功COMMIT响应均被丢弃，应用仍unknown。首次capture parent资格、首次单边状态和日志截断/锁冲突等非DB反例保留精确58f的113普通/race来源，不把它们冒充新数据库场景。

本轮154.374秒只为小型合成矩阵总耗时。真实VM初始total8,214,851,584字节、available5,018,947,584字节，SQLready后available4,539,019,264字节；SQL1GiB/Mongo2GiB实际资源回读通过。进程组已回收且不存在，两自有容器和网络精确释放，旧CID/images/volumes保留、cleanup错误为空；既有NSQ的restart由2740变2742，因此不签全环境运行稳定。生产capture/持续冻结、索引生产规模时长、安装、业务和RPO/30–60分钟仍未签。此后的文档子提交仅以Go/CI逐字等价继承872b组件证据；最终PR head仍须fresh CI和AI审查，旧7eb CI失败及两条审查原件保持。
