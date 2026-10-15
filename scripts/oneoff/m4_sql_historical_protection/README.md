# M4-A 全 NULL 历史列保护工具

本工具只保护 `assessment.historical_lifecycle_evidence`、`evaluation_outcome.historical_committed_evidence` 和原有两个 CHECK。只接受原 clean `100/39` 下两列全为 **SQL NULL** 的情形；任一非 NULL、未知结构、身份变化、保留事实变化、超时或释放未证实都会拒绝完成。它不保存业务字段正文，不提供通用数据库恢复、迁移或维护模式。

工具自身编译 SHA 与 A 执行器分别绑定。A 固定为 `51163edab7038adcdb6b8f1712d6dc434f361dc6`，嵌入迁移摘要 `77e11d801d270ef7b12f86261a05217a95bd8f8049f20d2e5f14ee0c0a20083b`、280 个资源。当前主线的后续迁移不会被自动当作 A 执行器。恢复 ADD 的固定来源仍是不可变原 099 资源的逐字节副本，其 SHA-256 为 `575fa26f588e4fb87ee7bbcdc61fc11eeb8fe8ed0234518209e7e3e13b7b0d7a`。

## 调用与材料

构建时明确注入实际工具来源；不能使用未绑定的默认版本字符串：

```sh
go build -ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$(git rev-parse HEAD)" \
  -o /private/approved-tools/m4-sql-historical-protection ./scripts/oneoff/m4_sql_historical_protection
```

调用者通过 `QS_M4_OBJECT_MYSQL_DSN`、`QS_M4_OBJECT_MONGO_URI` 提供已审定连接，DSN 选中的数据库须与 `--mysql-database` 一致。不要把密码放在命令参数、日志或回执中。实际 MySQL 必须为 8.0.36，Mongo 须为 replica set 且原账本恰有一个严格 typed clean39 文档。Mongo client 显式使用 Primary 覆盖 URI 的 secondary/nearest 偏好；每次 hello 必须证明 actual typed isWritablePrimary=true，mongos 或未知角色拒绝，原 cluster identity 摘要字节不变。账户须能证明所选 SQL scope 的直接 SELECT 可见性；角色或撤权范围不明时拒绝，不切换账户或自动授权。

默认 `--action=inspect` 只读核对身份、版本、会话及退役旧对象缺失。`capture`、`verify-target` 与 `verify-restored` 也不修改数据库；只有显式 `restore` 可以恢复两个对象及最后的原账本。

```sh
m4-sql-historical-protection --mysql-database=qs --mongo-database=qs
m4-sql-historical-protection --action=capture --mysql-database=qs --mongo-database=qs \
  --capture-file=/private/approved-protection/first-capture.json
m4-sql-historical-protection --action=verify-target --mysql-database=qs --mongo-database=qs \
  --material-file=/private/approved-protection/first-capture.json --material-sha256=<externally-frozen-raw-sha256>
m4-sql-historical-protection --action=restore --mysql-database=qs --mongo-database=qs \
  --material-file=/private/approved-protection/first-capture.json --material-sha256=<externally-frozen-raw-sha256> \
  --approval-file=/private/approved-protection/independent-restore-approval.json --approval-sha256=<independent-approval-raw-sha256>
m4-sql-historical-protection --action=verify-restored --mysql-database=qs --mongo-database=qs \
  --material-file=/private/approved-protection/first-capture.json --material-sha256=<externally-frozen-raw-sha256> \
  --approval-file=/private/approved-protection/independent-restore-approval.json --approval-sha256=<already-consumed-independent-approval-raw-sha256>
```

以上是调用示例，未作为生产命令执行。连接、工具 binary/source、A 执行器、输入材料和批准均须由执行控制方独立绑定。材料与批准须为绝对路径的私有 0600 普通文件，单硬链接、无符号链接、属当前 UID 或 root；原始 SHA 固定在独立批准中。首次 capture 用 O_EXCL 写入并 fsync，不覆盖旧材料。材料必须包含实际首次 capture 的 canonical 绝对路径；复制到另一文件名的材料及旧版缺失路径字段的材料均拒绝，不建立在线兼容层。首次恢复在该材料的固定私有父目录，以 O_EXCL/fsync 持久保存 `.m4-a-<materialSHA>.boundary.json` 和第一份完整批准。以后批准除 Operation 外的所有字段必须逐项相同；marker、恢复决定、deadline、expiry、冻结及责任不能重置。操作消费固定为同目录下 `.m4-a-<materialSHA>.operation-<operationSHA>.consumed`，内容另绑定批准原始 SHA 与材料 SHA，复制批准文件名或重编码不能重新使用同一操作。缺损 boundary 或已有 claim 后丢失 boundary 都拒绝重建，所有持久恢复状态须与原材料一并保留。外部材料和批准仍须独立原始 SHA；工具仅对自有固定状态使用相同私有文件检查读取。

capture 在读取连接环境变量前，使用与恢复相同的 canonical 私有父目录规则：父目录须属当前 UID 或 root 且不可 group/world 写入；root 不接受另一非 root UID 的目录，不能先生成恢复入口本身无法使用的材料。操作消费还写入同目录 `.m4-a-<materialSHA>.operation-journal.jsonl`：最多128个唯一操作、总64KiB，每次追加和独立 claim 均须 fsync/目录同步并相互完整，非阻塞文件锁冲突即拒绝。单个 claim 丢失、journal 丢失/截断/少记录、多余 claim 或首 boundary 丢失均保留原状并拒绝重建，不能靠删除消费标记重用原操作。

capture 同一个 RR 只读事务记录完整真实 SQL 账本、两个表 DDL/保留列/类型/主键以及全部主键与保留行 SHA；不保存行正文。三个固定对象必须是 InnoDB BASE TABLE；目标列必须为原099的最后一列且 json DEFAULT NULL，CHECK仅接受不可变099在实际MySQL8.0.36上的两个完整表达式：原 JSON schema ASCII literal 的 introducer 为 `_utf8mb3` 或 `_utf8mb4`（OCTET_LENGTH按服务器编码为length），其余字节、CAST及谓词必须完全相同。材料中的完整原DDL绑定每个对象的原编码，不增加通用DDL归一化或任意SQL输入。捕获、材料验证及恢复snapshot均在任何DDL前拒绝位置/default/谓词/schema漂移。会话时区 `+00:00`、结果字符集 utf8mb4 与固定 sql_mode 一致；数据库按类型、NULL 标记、字节长度及 HEX 进行散列，不经过 Go 时间重编码。uint64 主键以 DECIMAL(20,0) 参数比较，包含 id0 和最大值；每表 COUNT 守恒且必须实际读取独立空 EOF 页，不用抽样或以短页代替 EOF。上限每表 500,000 行、材料 128MiB，超限拒绝而不签部分完成。只归一化 AUTO_INCREMENT 动态值与明确目标列/CHECK；其他结构须相同。

## 恢复边界

独立批准绑定真实 writer-freeze 回执、责任回执、材料原始 SHA、双库身份、工具/A 来源、捕获时间、窗口、恢复决策、有效期和截止时间，数据/恢复责任人为 `system (10001)`，保护材料至少保留 30 天。恢复截止不得晚于决策后 60 分钟，批准有效期不得超过签核时后 24 小时；这些上限不代表已经证明生产预算。

执行控制方负责停止入口及全部 **API1、Collection2、Worker3** 六实例和其他写入者、记录在途责任并持续冻结。它必须在发出首个新 runtime 启动命令**之前**，以 O_EXCL 写入批准绑定的 first-runtime marker。工具不删除或修改 marker；marker 缺失本身不能证明六实例已停写。任何首启动已发出或新持久事实出现后，禁止本工具回放旧材料，改走目标 schema 兼容版本向前恢复。

restore 只接受 `101/39` 的具名 pre-start 状态：dirty101 或 clean101 下原 101 的 0、1、2 个 DROP 前缀，即两个原对象均存在、仅 outcome 原对象存在、两个对象均缺失。只按 outcome→assessment 的逆序执行原 099 固定 ADD 补齐缺失对象，使每个成功 ADD 后仍是合法前缀；不替换整表、不写历史列值。显式恢复中断后，必须继续绑定同一首次材料及 first-runtime marker，并提供新的独立、未消费批准；旧批准仍拒绝，不自动续跑或重试。verify-target 仍只接受 clean101 且两个对象均缺失，不借用恢复的部分形状许可。ADD 对唯一 schema literal 显式使用材料绑定的原 `_utf8mb3` 或 `_utf8mb4`，不依赖恢复连接字符集，也不执行材料中的DDL；已存在对象若是另一合法编码，在消费批准和执行DDL之前拒绝。恢复后两个完整原 SHOW CREATE 字符串须逐字相同。对象、保留结构、全 NULL、全部保留事实与双库身份核对成功后，才在一个真实 SQL 事务中回放**捕获的完整原账本**，账本为最后一步。每个 ADD、账本 BEGIN、DELETE、每条 INSERT 和 COMMIT 前紧邻复核原始持久 boundary、原 marker 和原绝对期限；不在检查后插入其他外部 RPC 再发出 COMMIT。中途失败、提交结果未知或连接释放未证实时保留失败，禁止 Force、down、指定版本或直接清 dirty。恢复中出现反向部分形状（assessment 已恢复、outcome 缺失）继续拒绝自动推进，不能猜测先前失败结果。

COMMIT 回应丢失时，原恢复回执继续保持 `ledger_commit_outcome_unknown`。若服务器实际已恢复为 clean100/39，可另行执行只读 `verify-restored`：必须提供原 canonical 材料及独立原始 SHA、已实际消费的原恢复批准（Operation 和批准原始 SHA 均在完整 journal 中），沿首次 boundary 的 marker、decision、deadline 和 expiry，不创建 boundary/journal/claim，也不消费新批准。它在同一只读 RR 全量核对原账本 DDL/全部行、两列及 CHECK 的完整原 DDL/编码、全 NULL、保留行散列和独立 EOF、双库身份及实时 clean100/39；连接全部释放后再次复核原输入、boundary 和期限才 complete。`verify-target` 仍严格要求 clean101/39、两对象均缺失。验证结果只说明当前原状态，不给旧 unknown 回执补 ACK，不证明停写或业务验收；控制器必须另获具名启动旧实例批准后才能使用该验证证据，不自动启动服务。

默认总时限 15 分钟，可显式缩短、最高 20 分钟。连接（含握手）、固定会话初始化、metadata、MAX(id)、分页读取及 socket 写入保持 5 秒；分页同时保持 MAX_EXECUTION_TIME(5000)。终端独立完整 COUNT 继续使用原 sqlReader、同一个只读 RR 快照及原 action 的绝对截止时间，server MAX_EXECUTION_TIME 取实时剩余的正整数毫秒；没有有限期限或剩余不足 1ms 时拒绝，不以0回退到无限 server 预算。COUNT 查询/Scan 失败、deadline、server3024 和成功读取后的计数不等分别报告，不删除 COUNT、EOF 或全部行散列。所有 action 的 socket 读取使用当前 action 剩余有限时长，普通短读取仍受各自5秒 child context 限制；恢复 DDL 的 context 继续受原有总期限、父 context、批准 deadline 和 expiry 的最早值约束，不提供无期限等待或重试。RR 事务的生命周期保留总 context，不能因给 BEGIN 绑定 5 秒子 context 而在完整分页结束前自动回滚；事务内两条 ledger SELECT 另有显式 5 秒读取 context。客户端超时或连接关闭不证明服务端 COUNT/DDL 已结束或 DDL 已回滚，失败后须独立观察实际结构和语句终态。捕获文件 fsync 后仍检查总 deadline。生产窗口 30 分钟须预留迁移、启动和回读时间；生产规模只读预演不等于 T0 冻结，实际预算仍须独立证明。回执仅在操作成功、SQL connection/pool（含 OpenConnections/InUse 均0）、Mongo 关闭及 deadline 都通过后 complete；事务 rollback 未证实单列，主错误不被覆盖。stdout 回执交付失败以非0退出，不声称成功。

## 已执行的隔离证明与 CI

代码基线 `d64886d52c67e6c87d475e1fb549ee047d944d06` 在 owned MySQL8.0.36/MongoRS7.0.31 上实际执行 `-race`：六个 required top-level 加十个 child，**16 RUN/PASS、0 FAIL/SKIP、exit0**。覆盖原捕获/目标核对、dirty101 的0/1/2 DROP、clean101 pre-start、DDL权限不足、最后账本 DELETE 拒绝、first-runtime marker、保留行变化、新行、非 NULL、id0/uint64最大值/EOF；真实 COMMIT OK 在客户端被丢失后工具仍报告 unknown，不 Force/down/retry。

实际 argv、独立源 hash 与完整结果保存在不可变私有 `qs-m4-a-null-object-native-v6`：harness `bd42eb7bbeeba9ccf7eb52c25087f449c7385f91f10be3b1ea23a9257daec857`，JSONL `48cc28446a4cbbec5b94eb53106c0de2aaef352aa8fe15e3bbc04e6d90d4c3d9`，receipt `aa3c364d8ee727017e48f07a4ad87bb83380f4982551cd42d8cb6db8d7930ae5`。两个自有容器及 bridge 已释放；原容器 IDs/images/volumes 保留，但既有 NSQ 等动态 runtime 元数据发生变化，未声称全部旧服务 runtime 不变。先前夹具失败及真实测试失败均保留，不借用其结果。

CI 复用 `metadata-readiness-mongodb-native` 的 authenticated rs0，与该 job 独占 MySQL8.0.36 服务运行实际 A native，注入实际 checkout SHA，必须十三个顶层及三十四个 child（含八个真实首 ADD ACK 后的批准边界攻击），明确列出全部47个 selector 并要求全部 RUN/PASS 且任何 SKIP/FAIL 均失败。新增 deadline 场景由独立 performance_schema 探针确认 ALTER 实际处于 MDL PENDING 后才持锁六秒并放行；另分别验证更早批准 deadline/expiry，失败时释放自有锁、有限等待实际语句结束并核对原 ledger 未提前回放，不根据超时推断对象回滚。原29项场景保留。新增 COUNT 场景在生产配置函数生成的有限 action socket 上，以同 pinned connection 的只读 RR 事务执行固定完整 COUNT，独立探针证明实际 MDL PENDING，覆盖六秒放锁后成功、早截止失败并独立观察语句结束、实际2行与预期3行不等。新增恢复场景分别在 dirty101/clean101 的第1/2个真实 ADD 成功 ACK 后取消；测试 seam 仅接收固定对象 index，CLI 硬传 nil。它们核实101完整账本未提前变化、旧批准拒绝、新独立批准显式续跑、原DDL/全部保留事实/账本最后恢复及fresh10039。八个新增攻击分别覆盖复制批准路径、同操作重编码、延长deadline、变更恢复决定、首启动后换marker、复制材料路径、claim后丢boundary及损坏boundary；要求所有保留行散列、真实101账本、原Mongo BSON和已恢复对象不变。以上新增场景尚须本 head 的真实 native 证明，编译不作为通过。普通测试、编译或 `-run='^$'` 不代替 native。新 merged head 必须独立 CI，以上 component 证明不换签新 head。

该证明只限合成、隔离、小规模对象保护与失败边界；未签生产 capture/权限/冻结/恢复、最终镜像、三服务启动、业务验收、RPO0 或生产30/60分钟预算。

后续独立 source `b7ae414a64597bfaf9f35031ec975cfa59f541ec` 增加原结构早拒绝与两个真实漂移用例；普通 race、integration 编译、tagged vet、目标 lint已通过。上述16 native仍只属于d648，不换签新增源码。

整合实际 main `c335b2a1a34dd69830261e2d8e11b16da3f8befb` 后，工具 checkout `56c598337578537aa75319f7612dd9690948ef3c` 在 owned MySQL8.0.36/MongoRS7.0.31 上独立完成七个 required top-level 加十二个 child，**19 RUN/PASS、0 FAIL/SKIP、exit0**。原 CHECK 变更和历史列位置变更均在 capture 写材料及 restore 消费批准/执行 DDL 前被实际拒绝。六个 Go/SQL 文件保持与 b7 source 相同，A511/77e/280 绑定不变；当前主线103/40不被当作 A 目标。

实际 argv 与源文件摘要保存在新不可变私有 `qs-m4-a-null-object-native-v8`：harness `6851c25107a19ed7b55235e598e715b62016572ebf5c75a7589c6b6f251c4d39`，JSONL `e50d9506bc04729c39d91ad82891e0eac5dafed4a8576babeb9198fd97a29a7e`，receipt `6ba23dff77f0e56cb829e7dfc0bad97fc7a49b1c477ba463d47e82446f299d9a`。79.090秒仅是本次小规模隔离夹具总耗时。两个自有容器及 bridge 已释放、cleanup errors为空，原 IDs/images/volumes 保留；既有 runtime 动态元数据仍发生变化，未签全部旧服务稳定。旧成功及失败材料均保留。

独立 Linux/amd64 正式 tool 以实际 `56c598337578537aa75319f7612dd9690948ef3c` 编译，六个源文件与上述 native 前后摘要相同；binary SHA-256 为 `07a964a6b47ac24b76cd5abb3e2af38e0229ae974ea5d897d37c097030cb0afa`。私有 `qs-m4-a-null-object-tool-amd64-56c598` 构建 receipt 为 `28a46e6d6c8e687a95688cdb473542dccd73a70defbde71e14d6549dd60fd1a9`，保存实际 argv 和 `go version -m`。这只证明该 binary 的构建，未证明其 amd64 实际运行或生产调用；工具编译 SHA 不替代 A 执行器511。后续文档 head仍须独立CI，不换签56c的真实native。


## 原 CHECK 编码修复的独立证明

先前工具只接受 `_utf8mb4` schema literal，不能捕获经原 `charset=utf8` 连接执行099后保存的 `_utf8mb3` 表达式。修复只枚举上述两种完整原表达式：历史099资源及其摘要不改，CAST仍为utf8mb4；注释、谓词、范围、schema或其他 introducer 变化继续拒绝。原材料版本及字段不变，原编码由已经严格校验的 `OriginalDDL` 绑定。

实际 source `94ab3a7cf090839da5283c7f09e9b7653f92b0e0` 在 owned MySQL8.0.36/MongoRS7.0.31 上完成 `-race` native：九个顶层、十六个 child，**25 RUN/PASS、0 FAIL/SKIP、exit0**。新增四项分别以原 utf8（mb3）或 utf8mb4 连接创建完整历史schema，再用相反字符集连接捕获和恢复；覆盖真正迁移101在首个DROP后dirty和两个DROP后clean的状态。两表完整原SHOW CREATE、全部保留事实和原账本最后回放均通过；另一合法但不匹配的现存编码在批准消费前被实际拒绝。

不可变私有 `qs-m4-a-null-object-native-charset-v2` 保存实际 argv、完整JSONL和源摘要：harness `bc67a8831f9133526fcda9a51bae6ca9c5d6558c6a35759a20e38fb3a3af0514`，JSONL `5ba064d92b73301f08efaae70276b51ca4cbaadca77d3e88d0c8d489c7e3b541`，receipt `13a12fe449f25a9a2bf1725b8462ff9067d63f032acf6f6ae60d44bda2a17167`。100.021秒是本次合成隔离夹具总耗时。owned双库容器及bridge均精确释放、原IDs/images/volumes保留；既有动态runtime元数据变化，仍不签全部服务稳定。首轮新增测试helper的nil map失败和原完整证据保留在 `qs-m4-a-null-object-native-charset-v1`，未作为通过证明。

独立Linux/amd64新tool实际从上述clean source94ab编译；六个Go/SQL文件逐字匹配该25项native来源。binary SHA-256为 `7df1c9d6da4b6cbe1425c1867c7d5916562818d7dc9881266b17fc542d9ae1f2`（9,728,184字节），私有 `qs-m4-a-null-object-tool-amd64-94ab3a/receipt.json` SHA为 `9e87ed4ee54ac3f4447cf1d0390d2725cc647d38657a28c699be1ecc98e52fe9`，保留实际编译参数、Go构建元数据和源文件摘要。旧56c/07a工具及保管材料保持原样；新binary尚未证明amd64实际运行或生产调用。后续文档head通过逐字源闭包继承component证明，最终PR head仍须fresh CI，不能更换native或compiled ToolSource。


## 当前有界 COUNT、显式续跑与持久批准边界的准备状态

当前修复保留完整 COUNT、独立空 EOF 和全部保留行散列；终端 COUNT 使用原 RR/action 剩余期限，分页等普通读取仍为5秒。旧 source530 的容量演练在 verify-target 的完整 COUNT 通用错误处失败，未证明失败表、接收计数、底层错误或恢复成功；原失败材料不改写，也不将其归因为已采证超时。

source8517 的一次本机 native38 未完成：owned Mongo 实读 Exit137/OOMKilled=true，后续连接失败；部分27 RUN/12 PASS/13 FAIL/0 SKIP，新增 COUNT/续跑 top 尚未 RUN。自有双库、网络及数据卷已释放，原 IDs/images/volumes 保留，动态旧 runtime 元数据变化另列。该失败不作为当前源码、47项矩阵或生产资格证明。Primary、持久 boundary 与八项攻击的源码/编译/普通检查也不替代最终同源真实 native、容量恢复与生产窗口验收；当前没有新正式 tool binary 的资格签核。

源码 `94e1b7d1856de57665c96bbe807c13bcc725e8ab` 的独立 owned MySQL8.0.36/MongoRS7.0.31 演练已完成全部 **47 RUN/PASS、0 FAIL/SKIP**（13个 top-level、34个 child），native292.549秒、含精确清理313.652秒；私有 `qs-m4-a-boundary-native47-v2` receipt SHA `a255facd4e257009b3b27b458799f04f49170cb46851ddabb7b366249e85063b`，原JSONL SHA `8acb8e2f5dd1be4b64f3e218e8cfd84c6bb493680129576f7f54812b07caf18e`。COUNT3个 child、DDL deadline3个 child、同首次绝对预算的4个合法续跑及8个攻击全部实际通过。自有双库容器、network、volume及测试进程全部释放，旧CID/image/volume保留；既有 nsqd 自主 restart2710→2715，因此不声称全部旧 runtime 状态不变。该结果仅属于94源码及小夹具，不覆盖后续parent/journal/verify-restored变更或容量/生产预算。

后续源码分别独立提交：`bd468671858b2a9e314ada4c241f3d819f084c83` 同父目录规则、`910ea6265ad7900b3dd45602a5eaa5095502b5d9` 有界 journal、`a01d30ede0ee85e5e25b6300ace90a0b1be3690c` 只读 verify-restored。exact a01 本包 ordinary `-race -count=1 -json` 实际 **125 RUN/PASS、0 FAIL/SKIP**；tagged race编译（未运行测试）、tagged vet和本次增量lint均通过，私有 QA receipt SHA `581749ae0e4e22d65d5632dab141e8596e5762480859a32eea9f2f75e1456001`。原 mandatory COMMIT-response-loss top 已扩展新鲜连接 verify-restored 成功、真实保留事实漂移拒绝及原文件/unknown回执不变的断言，required名单仍47；新源码的完整 native47及容量均待执行，不借94的47 PASS换签。控制器具名 start-old 批准和生产执行资格仍独立，默认权限模式与A511历史迁移字节不变。
