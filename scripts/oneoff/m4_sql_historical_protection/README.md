# M4-A 全 NULL 历史列保护工具

本工具只保护 `assessment.historical_lifecycle_evidence`、`evaluation_outcome.historical_committed_evidence` 和原有两个 CHECK。只接受原 clean `100/39` 下两列全为 **SQL NULL** 的情形；任一非 NULL、未知结构、身份变化、保留事实变化、超时或释放未证实都会拒绝完成。它不保存业务字段正文，不提供通用数据库恢复、迁移或维护模式。

工具自身编译 SHA 与 A 执行器分别绑定。A 固定为 `51163edab7038adcdb6b8f1712d6dc434f361dc6`，嵌入迁移摘要 `77e11d801d270ef7b12f86261a05217a95bd8f8049f20d2e5f14ee0c0a20083b`、280 个资源。当前主线的后续迁移不会被自动当作 A 执行器。恢复 ADD 来自不可变原 099 资源的逐字节副本，其 SHA-256 为 `575fa26f588e4fb87ee7bbcdc61fc11eeb8fe8ed0234518209e7e3e13b7b0d7a`。

## 调用与材料

构建时明确注入实际工具来源；不能使用未绑定的默认版本字符串：

```sh
go build -ldflags="-X github.com/FangcunMount/qs-server/pkg/version.GitCommit=$(git rev-parse HEAD)" \
  -o /private/approved-tools/m4-sql-historical-protection ./scripts/oneoff/m4_sql_historical_protection
```

调用者通过 `QS_M4_OBJECT_MYSQL_DSN`、`QS_M4_OBJECT_MONGO_URI` 提供已审定连接，DSN 选中的数据库须与 `--mysql-database` 一致。不要把密码放在命令参数、日志或回执中。实际 MySQL 必须为 8.0.36，Mongo 须为 replica set 且原账本恰有一个严格 typed clean39 文档。账户须能证明所选 SQL scope 的直接 SELECT 可见性；角色或撤权范围不明时拒绝，不切换账户或自动授权。

默认 `--action=inspect` 只读核对身份、版本、会话及退役旧对象缺失。`capture` 与 `verify-target` 也不修改数据库；只有显式 `restore` 可以恢复两个对象及最后的原账本。

```sh
m4-sql-historical-protection --mysql-database=qs --mongo-database=qs
m4-sql-historical-protection --action=capture --mysql-database=qs --mongo-database=qs \
  --capture-file=/private/approved-protection/first-capture.json
m4-sql-historical-protection --action=verify-target --mysql-database=qs --mongo-database=qs \
  --material-file=/private/approved-protection/first-capture.json --material-sha256=<externally-frozen-raw-sha256>
m4-sql-historical-protection --action=restore --mysql-database=qs --mongo-database=qs \
  --material-file=/private/approved-protection/first-capture.json --material-sha256=<externally-frozen-raw-sha256> \
  --approval-file=/private/approved-protection/independent-restore-approval.json --approval-sha256=<independent-approval-raw-sha256>
```

以上是调用示例，未作为生产命令执行。连接、工具 binary/source、A 执行器、输入材料和批准均须由执行控制方独立绑定。材料与批准须为绝对路径的私有 0600 普通文件，单硬链接、无符号链接、属当前 UID 或 root；原始 SHA 固定在独立批准中。首次 capture 用 O_EXCL 写入并 fsync，不覆盖旧材料。批准另以 O_EXCL `.consumed` 文件消费一次，恢复输入不能给自己重新授权。

capture 同一个 RR 只读事务记录完整真实 SQL 账本、两个表 DDL/保留列/类型/主键以及全部主键与保留行 SHA；不保存行正文。三个固定对象必须是 InnoDB BASE TABLE。会话时区 `+00:00`、字符集 utf8mb4 与固定 sql_mode 一致；数据库按类型、NULL 标记、字节长度及 HEX 进行散列，不经过 Go 时间重编码。uint64 主键以 DECIMAL(20,0) 参数比较，包含 id0 和最大值；每表 COUNT 守恒且必须实际读取独立空 EOF 页，不用抽样或以短页代替 EOF。上限每表 500,000 行、材料 128MiB，超限拒绝而不签部分完成。只归一化 AUTO_INCREMENT 动态值与明确目标列/CHECK；其他结构须相同。

## 恢复边界

独立批准绑定真实 writer-freeze 回执、责任回执、材料原始 SHA、双库身份、工具/A 来源、捕获时间、窗口、恢复决策、有效期和截止时间，数据/恢复责任人为 `system (10001)`，保护材料至少保留 30 天。恢复截止不得晚于决策后 60 分钟，批准有效期不得超过签核时后 24 小时；这些上限不代表已经证明生产预算。

执行控制方负责停止入口及全部 **API1、Collection2、Worker3** 六实例和其他写入者、记录在途责任并持续冻结。它必须在发出首个新 runtime 启动命令**之前**，以 O_EXCL 写入批准绑定的 first-runtime marker。工具不删除或修改 marker；marker 缺失本身不能证明六实例已停写。任何首启动已发出或新持久事实出现后，禁止本工具回放旧材料，改走目标 schema 兼容版本向前恢复。

restore 只接受 `101/39` 的具名 pre-start 状态：dirty101 下原 101 的 0、1、2 个 DROP 前缀，或 clean101 且两个对象均已退出。只执行原 099 的固定 ADD 补齐缺失对象，不替换整表、不写历史列值。对象、保留结构、全 NULL、全部保留事实与双库身份核对成功后，才在一个真实 SQL 事务中回放**捕获的完整原账本**，账本为最后一步。中途失败、提交结果未知或连接释放未证实时保留失败，禁止 Force、down、指定版本或直接清 dirty。恢复中出现反向部分形状（assessment 已恢复、outcome 缺失）继续拒绝自动推进，不能猜测先前失败结果。

默认总时限 15 分钟，可显式缩短、最高 20 分钟；单请求/连接 I/O 5 秒，恢复另受批准 deadline/expiry 约束。捕获文件 fsync 后仍检查总 deadline。生产窗口 30 分钟须预留迁移、启动和回读时间；生产规模只读预演不等于 T0 冻结，实际预算仍须独立证明。回执仅在操作成功、SQL connection/pool（含 OpenConnections/InUse 均0）、Mongo 关闭及 deadline 都通过后 complete；事务 rollback 未证实单列，主错误不被覆盖。stdout 回执交付失败以非0退出，不声称成功。

## 已执行的隔离证明与 CI

代码基线 `d64886d52c67e6c87d475e1fb549ee047d944d06` 在 owned MySQL8.0.36/MongoRS7.0.31 上实际执行 `-race`：六个 required top-level 加十个 child，**16 RUN/PASS、0 FAIL/SKIP、exit0**。覆盖原捕获/目标核对、dirty101 的0/1/2 DROP、clean101 pre-start、DDL权限不足、最后账本 DELETE 拒绝、first-runtime marker、保留行变化、新行、非 NULL、id0/uint64最大值/EOF；真实 COMMIT OK 在客户端被丢失后工具仍报告 unknown，不 Force/down/retry。

实际 argv、独立源 hash 与完整结果保存在不可变私有 `qs-m4-a-null-object-native-v6`：harness `bd42eb7bbeeba9ccf7eb52c25087f449c7385f91f10be3b1ea23a9257daec857`，JSONL `48cc28446a4cbbec5b94eb53106c0de2aaef352aa8fe15e3bbc04e6d90d4c3d9`，receipt `aa3c364d8ee727017e48f07a4ad87bb83380f4982551cd42d8cb6db8d7930ae5`。两个自有容器及 bridge 已释放；原容器 IDs/images/volumes 保留，但既有 NSQ 等动态 runtime 元数据发生变化，未声称全部旧服务 runtime 不变。先前夹具失败及真实测试失败均保留，不借用其结果。

CI 复用 `metadata-readiness-mongodb-native` 的 authenticated rs0，与该 job 独占 MySQL8.0.36 服务运行实际 A native，注入实际 checkout SHA，必须六 selector 全部 RUN/PASS 且任何 SKIP/FAIL 均失败。普通测试、编译或 `-run='^$'` 不代替 native。新 merged head 必须独立 CI，以上 component 证明不换签新 head。

该证明只限合成、隔离、小规模对象保护与失败边界；未签生产 capture/权限/冻结/恢复、最终镜像、三服务启动、业务验收、RPO0 或生产30/60分钟预算。
