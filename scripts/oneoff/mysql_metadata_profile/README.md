# MySQL 只读元数据诊断

无 argv 的原权限模式只读诊断当前 session 的 SHOW GRANTS，以及固定 RDS 角色的潜在权限集合，不读取业务记录。独立 readiness 模式只读取明确限定的目录和分页标量分类。新增 inventory 模式只返回固定退役对象的标量计数。三个模式均不激活角色、不替代现有删除门禁，也不证明清理资格。

工作流提供两个独立的只读入口：`metadata-visibility-profile` 使用现有应用账号；`metadata-visibility-admin-profile` 使用 production 环境中的 `MYSQL_METADATA_ADMIN_USERNAME`、`MYSQL_METADATA_ADMIN_PASSWORD`，复用既有 host/port/database，并保持上述生产目标 hash 校验。管理员 Secret 缺失时停止，绝不回落到应用账号。CBPT 清理四个入口固定使用这组管理员身份，但原有全局元数据、依赖、锁、内容与恢复证明门禁不变。新工作流 source SHA 必须重新归档和隔离恢复，原归档及证明保留。 CBPT 手工入口必须提交 `governance_confirm`，在 checkout 和执行前核对实际 GitHub SHA，防止派发期间分支移动导致执行未审查版本。

固定输入：PROFILE_SOURCE_SHA（40 位小写十六进制）、PROFILE_RUN_ID（GitHub run-attempt 数字格式）、PROFILE_EXPECTED_TARGET_HASH（64 位小写十六进制），以及 MYSQL_HOST/PORT/USERNAME/PASSWORD/DATABASE 五项环境变量。无 argv 时执行本页原权限 profile；另行声明的两个固定诊断 argv 为 `--metadata-readiness`、`--retirement-inventory`，均只允许单独使用。没有命令行业务参数；其它 argv 或格式不合法时只输出固定失败。workflow 将预先验证的生产 target hash 作为固定绑定传入；不包含生产连接或凭据。

`metadata-readiness-admin` / `database=mysql` 使用独立 `--metadata-readiness` 模式，只接受固定 `qs` 目标及 clean 100。派发必须在 checkout 前验证完整 `governance_confirm` 等于 GitHub SHA，且 `backup_name` 为空；只使用现有 metadata 管理员 Secret，缺失时停止。SSH 控制器使用显式 `--readiness` 与六成员包，原五成员权限 profile 的格式和行为保持独立。

readiness 当前只接收 `format_version=2`；旧 V1 文件作为历史证据保留，不由当前接收器兼容解析或改写。无 argv 的原五查询权限 profile 及其格式保持独立。V2 成功要求完整读取两个历史 JSON 列、确认一致快照、释放只读事务及 pinned Conn，并关闭 owned DB pool。只输出限定目录的 count/digest、全 schema EVENT 观测、线程/事务/metadata lock 标量、仪表可用性、主键和快照验证布尔、页数、EOF，以及历史列的守恒计数分布；不输出行 ID、上界、JSON 内容、SQL、账号、角色或目录名称。失败不保留 measurements，失败阶段及释放字段保持明确未知/失败。`diagnostic_only=true`、`readiness_only=true`、`freeze_verified=false` 固定不变；一致快照和观测零值不证明停止所有写入、完整备份或 RPO=0，也不授权结构退役。

V2 在同一 pinned Conn 依次执行固定 `SET TRANSACTION ISOLATION LEVEL REPEATABLE READ`、`START TRANSACTION READ ONLY`，再用单条双表 `MAX(id)` 查询建立同一读视图和两个上界。该读取持有两表的 metadata lock 后，固定目录查询确认 `assessment`、`evaluation_outcome` 仍为 InnoDB，且各有 `id BIGINT UNSIGNED NOT NULL` 单列 PRIMARY KEY；结构不符立即失败。每表固定每页1000行，按 PRIMARY KEY 顺序读取 RAM 内的 ID 和五类标量；首段包含 `id=0`，续段采用 `id>last AND id<=upper`，不计算 `last+1`。两个参数均保持 RAM 内经 `ParseUint` 验证的规范十进制字符串，并用固定 `CAST(? AS DECIMAL(20,0))` 作精确边界比较；不能以浮点数或字符串转 UNSIGNED 替代。隔离8.0.36曾实际暴露字符串参数转 UNSIGNED 截断 uint64 最大值、漏末行的问题，必须保留该最大值与 EOF 回归。最多512个非空页，加1个必需的终止空页；空表也须1个空页。完整 EOF、严格递增主键、最后读取键等于固定 MAX 上界和分类守恒均通过才保留测量；不抽样、不重试、不续跑。RR 中的一致读复用首次一致读建立的快照，参见 [MySQL 8.0 一致性非锁定读](https://dev.mysql.com/doc/refman/8.0/en/innodb-consistent-read.html) 和 [SET TRANSACTION](https://dev.mysql.com/doc/refman/8.0/en/set-transaction.html)。

分页结束后，先在总预算内成功 `ROLLBACK`，再于事务外实时重读 clean100 和会话身份；这样版本变化不会被 RR 旧视图遮住。V2 的 `query_count` 仅计只读结果查询（SELECT 与固定 SHOW GRANTS）尝试，成功时精确等于 `19 + assessment_pages + outcome_pages`，范围21～1045；两个页数字段均含终止空页，范围1～513，每表最多512000行，非空页数必须等于总行数除以1000后向上取整。`control_statement_count` 单独计上述两条开始语句和 ROLLBACK 尝试，最大3，成功须为3；`transaction_released` 始终存在，未开始或未核实为 null，成功须为 true。`primary_key_shape_verified`、`consistent_snapshot`、两个 EOF 布尔也须为 true。分页超限为 `pagination_limit_exceeded`，事务释放失败为 `transaction_release_failed`，均清空全部 measurements，不能只保留已完成的一表。

单一 pinned sql.Conn、总 deadline 45 秒、每条读取及事务控制语句5秒、connect 5秒；分页、游标释放和 ROLLBACK 共用原总截止，不因翻页或清理重置。MultiStatements/InterpolateParams=false。配置和 driver 全局 logger 都为 NopLogger。身份 query 读取 session ID/UUID/database/version/CURRENT_ROLE，前后必须完全一致；UUID 仅用于兼容现有 cleanup hashText(host, normalizedPort, database, UUID)，hash 必须匹配 PROFILE_EXPECTED_TARGET_HASH。身份值、角色原文和 SQL 结果不进入输出。

权限与角色诊断字段的含义：

- current_unrestricted_metadata_grants：仅诊断固定、不带 USING 的 SHOW GRANTS FOR CURRENT_USER 返回的账号直接 grant 中四项全局权限及 partial revocations。该输出会列出已授予角色，但不展开角色权限，不能代表全部 active roles 的有效权限；参见 [MySQL 8.0 SHOW GRANTS](https://dev.mysql.com/doc/refman/8.0/en/show-grants.html)。
- rds_role_grants_available：固定 SHOW GRANTS FOR CURRENT_USER USING `rds_superuser_role`@`%` 查询成功为 true；只有 MySQL 数字错误 3530 且 SQLState 为 HY000 才为 false；其它查询错误为 null 并失败。
- rds_role_unrestricted_metadata_grants：上述 USING 查询的账号直接权限加固定角色的潜在集，不能称当前有效集，也不把其它 active roles 并入它。角色未授予或查询失败时为 null。角色输出格式不被识别时为 null、complete=false；不输出角色结论。

- assigned_roles_present：当前规范 SHOW GRANTS 结果是否包含明确已授予角色的行；不输出角色身份，也不证明它们有管理员权限。
- mandatory_roles_present：固定 SELECT @@GLOBAL.mandatory_roles 是否为非空；只返回有无，不输出原始配置。仅 complete=true 时两个有无字段才为布尔，失败时为 null。查询不激活角色。

3530 为 ER_ROLE_NOT_GRANTED，已核对 [Oracle/MySQL 8.0 官方错误参考](https://docs.oracle.com/cd/E17952_01/mysql-errors-8.0-en/mysql-errors-8.0-en.pdf) 的该条目；没有用文字错误匹配推断未授予。

GRANT 解析只认可 bounded 的 privilege/account/scope/role/proxy 语法；静态权限使用固定列表，合法全局动态大写标识符可识别但不贡献 SELECT/SHOW VIEW/TRIGGER/EVENT。全局 ALL PRIVILEGES 展开为这四项后判断；非全局 ALL PRIVILEGES 不贡献全局权限。canonical REVOKE 为已知负结论（该组无论其它行如何都 false），仍继续另一个固定潜在集诊断。未知/畸形输出失败关闭；有 SQL 字面文本、角色或账户的输入不被拼接到固定查询。

RDS 完整 static/dynamic 多行 fixture 依据 [AWS RDS 官方角色模型](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Appendix.MySQL.CommonDBATasks.privilege-model.html) 的 8.0.36+ 权限列表。测试包含 APPLICATION_PASSWORD_ADMIN/ROLE_ADMIN/SET_USER_ID/XA_RECOVER_ADMIN；它们单独出现不能假造四项元数据权限。

原权限模式仅输出受控 receipt 字段：format_version/source_sha/run_id/expected_target_hash/source_target_hash/current_unrestricted_metadata_grants/rds_role_grants_available/rds_role_unrestricted_metadata_grants/assigned_roles_present/mandatory_roles_present/diagnostic_only/complete/error_category。无 raw grant、role/account/host/database/version/UUID/SQL/driver error 或凭据。diagnostic_only 永远为 true；即使两个权限布尔均 true，也不是删表授权或资格。未知错误和身份变化时 current 为 false、四个角色字段为 null，complete=false、exit1；合法 negative（含未授予/partial restriction）为完整诊断，不是权限赋予。

离线 sqlmock 测试覆盖未激活固定角色 potential=true/current=false、current 与固定角色 grant 输入互不混用（不证明真实 active role 权限展开）、3530/HY000 与缺失或其它 state、3523/1045严格区分、RDS完整grant语法、合法REVOKE继续潜在诊断、畸形语法/SQL注入后缀拒绝、未知错误私密、session/role变化丢弃正结论、expected target mismatch不读取权限、driver logger/multiStatement边界。通过 Go race/vet 与 Python 传输测试后，workflow 才会运行诊断。运行结果应独立绑定 source/run/target hash。

四个 MySQL metadata 工作流入口使用用户已配置的 Mac mini 自托管 runner 直接 SSH 目标 `root@47.94.204.124:22`。使用已有 `~/.ssh/known_hosts`，本地 `ssh -G` 必须精确绑定 root、该 IP 和 22 端口，并拒绝 ProxyCommand、ProxyJump 或 HostKeyAlias；逐次要求 `StrictHostKeyChecking=yes`、无密码交互、新连接及关闭 agent/X11 转发。接收器在读取秘密 envelope 前核实 root、Linux 和 `serverA` 主机名；不下载或修改主机密钥、SSH 配置或权限。缺少已登记 host key、所选管理员 Secret 或完整目标绑定时停止，不回落到其他主机、principal 或应用账号。除所选入口外，其余 db-ops job 在这四组 MySQL metadata 输入下均不符合执行条件。

`mysql-metadata-profile-ssh.py` 将唯一固定文件集的 SHA-256 归档及凭据通过 SSH stdin 传入；密码不进入命令行、日志或 Git。远端只在任务拥有的 0700 临时目录解包，使用 Linux 静态 binary，不创建生产 Docker 容器、不拉取镜像。临时 JSON 环境文件为 0600，固定字段经过验证，不经 shell source/eval，并在建立数据库连接前删除该文件；binary 不继承无关凭据或 LD_* 环境。SSH 输出限 256 KiB/90 秒，binary 输出限 64 KiB/50 秒，stderr 只排空不保留，原始异常不输出。只有 task-owned 文件完整释放后才输出安全密封回执，传输、清理或回执错误均失败关闭。封装失败使用四个固定类别：`profile_input_preflight_failed`（操作、输入与绑定）、`profile_package_preflight_failed`（本地包、checksum 与 envelope）、`profile_trusted_ssh_preflight_failed`（本地可信 SSH 预检）、`profile_remote_or_receipt_failed`（实际 SSH、远端执行、回执与释放后输出）。这些类别只描述本次源码的边界，保留 complete=false、权限 false/nullable 字段，不含异常正文、trace、凭据或远端状态猜测；remote 类别不能单独证明是否连接数据库。原来的 `profile_transport_or_receipt_failed` 继续接受为未定位失败。已通过原验证器的具体 SQL 失败回执沿用原类别，不被封装阶段改名。

`complete=true` 还要求 pinned Conn.Close、owned DB.Close 均成功且 pool 的 InUse/OpenConnections 为零。池暂保留最多一个 idle connection，让紧随其后的 DB.Close 能返回底层 driver.Close 错误，避免 MaxIdleConns=0 在 putConn 中忽略该错误。`connection_release_failed` / `database_release_failed` 是封闭失败类别，清空所有权限和角色结论，不输出底层错误。隔离 MySQL8.0.36 required race 门禁验证 schema-only、全局四权限、未激活固定角色、partial revoke、错误目标及会话释放；释放错误使用明确测试 seam，不能冒称真实数据库物理释放失败。

原 `mysql-metadata-profile.py --binary` Docker 路径及其 negative 测试仍保留，但 metadata 工作流通过受限 SSH receiver 调用 `execute_native`。CBPT 删除工具、其他 db-ops 操作和共享传输 helper 保持原样。

readiness 查询失败按固定类别诊断：服务端 MySQL 3024 为 `query_server_timeout`，1317 为 `query_server_interrupted`，1142／1143／1227 为 `query_permission_denied`，Go context deadline／cancel 为 `query_deadline_exceeded`／`query_canceled`；未识别错误保持 `query_failed`。QueryContext、事务控制及读取结果的 Rows.Err 使用相同规则，不输出 Message、SQL、账号或驱动错误正文，不根据耗时推断原因。原五查询 profile 不变；readiness V2 分页仍共享5秒单条／45秒总预算，失败清空部分测量并核验事务和连接释放，不重试或回退认证。本页描述执行协议；隔离 native 的精确证据见下文，生产诊断保持独立验收。

typed MySQL 错误另保留可选 `query_mysql_error_number`，只复制 Number 的0～65535整数，未列入上述类别的号码归 `query_mysql_error`；没有观测到该类型时省略，成功回执禁止带号码。原样保留的网络timeout／其他网络错误、InvalidConn、BadConn及失败Rows.Err中的EOF各有固定类别；正常结果集结束不算失败。driver1.9.3可能先将读取错误统一为InvalidConn，因此 `query_invalid_connection` 不能证明具体网络原因或超时。metadata、事务、Conn 或 DB 释放失败仍优先阻断完成，若此前已有真实MySQL数字码则保留其诊断值；不读取SQLState、Message、网络Op／Addr或原始driver日志。


## 日志回执运输

最终日志只编码 `safe_receipt` 已验证的非秘密字典，内部 Go JSON 与原 `safe_receipt` 输入接口保持不变。共享 `scripts/dbops/receipt-transport.py` 再按显式字段、严格布尔/无符号计数、SHA/hash/run ID 和固定 token 白名单检查；不接受原始日志、GRANT、账号、连接或凭据字段。运输编码用于避免平台掩码损坏安全摘要，不用于传输秘密。

UTF-8 JSON 每个 byte 映射到私用字符 U+E000..U+E0FF，以 U+E100/U+E101 框定；必要时整体改用 +0x200 或 +0x400 的独立字母表。若完整 frame 含选中的 MYSQL_USERNAME 或 MYSQL_PASSWORD 任一非空值，就尝试下一字母表；全部碰撞或编码器不可用时失败关闭，不输出明文 JSON 回退。错误输出也只使用本地固定白名单失败字典，原始异常永不进入编码。

共享 API：`encode_armored_receipt(value, schema=..., secrets=...)` 返回 frame，`decode_armored_receipt(text)` 返回 JSON 文本，错误为固定类别的 `ReceiptTransportError`。schema 是可选字段白名单；字段完整性与绑定仍由原验证器负责。JSON body 上限 64 KiB、日志上限 4 Mi 字符；解码必须恰有一个完整 frame，字母表和 UTF-8 合法、无重复 JSON key。解码后仍须原 source/run/target/exit 验证，不能把可解码等同于清理资格，也不尝试修补旧的被掩码 JSON。共享与 profile 离线测试覆盖短字符掩码、备用字母表、缺失/重复/混合或超限 frame、凭据碰撞、未知 raw 字段及绑定拒绝。

当前权限不足时，诊断进一步输出四项全局 GRANT 是否存在及是否存在部分 REVOKE，均为布尔字段，不含账号或语句。字段只在目标、查询和最终同一会话身份全部验证后发布；失败均为 null。它们解释现有权限结论，不改变清理门禁。schema 级授权不能代表全局可见性，正向 GRANT 与 REVOKE 同时存在也不能代表完整权限。旧摘要仍可解析；新字段若出现必须五项完整且与原 current_unrestricted_metadata_grants 一致。

## V2 分页真实验收

源码 `4160cc63ac881d1addd3f066d6053301e8d5431e` 在自有 MySQL8.0.36 上实际执行 `bash scripts/testing/m4-metadata-readiness-native-proof.sh mysql`：16项 RUN/PASS、0 SKIP/FAIL。包含同一 RR 快照内并发插入／更新／删除、跨页、主键0及uint64最大值、真实有符号主键拒绝，以及原五查询权限与真实错误／释放场景。字符串参数经 `CAST AS UNSIGNED` 的反例保留原失败回执；固定 `DECIMAL(20,0)` 比较后完整边界及 EOF 通过，5秒单条／45秒总预算不变。

原 native JSONL SHA256 `31d513d2f61f1ccf50a9a0b2326d129c9cb3e5779a8bd8593ce3b0d3ef623265`，最终交付摘要 `678e948624238d08fe7384e287e539c956ac15609d92d0d485706c88c84b64a8`；自有容器、网络和连接文件均已释放，旧CID/image保持。既有本机nsqd运行元数据实际变化，未签整个环境稳定或作因果推断。这证明对应组件隔离读取与释放，不能证明当前生产全量空值、持续停写、对象恢复、三服务启动或30/60分钟预算。


## 固定退役盘点

`retirement-inventory-admin` / `database=mysql` 是独立 `--retirement-inventory` 入口。它沿同一已核目标 hash、管理员 Secret 和固定 SSH route；先验证 `governance_confirm` 等于实际 GitHub SHA、`backup_name` 为空，再 checkout、构建和读取 Secret。不试探其它账号，不回落应用身份，不接受任意 SQL、对象名或目标版本。SSH `--inventory` / `--receive-inventory` 的六成员包为原五成员加 `mysql-retirement-inventory.py`；原五查询 profile、readiness V2 和 inventory V1 的 flag、包及回执互不兼容，任何交叉模式或额外参数均拒绝。

固定11条业务读取覆盖九张 InnoDB BASE TABLE：`testee`、`operators`、五张历史维护表，以及 `ai_bridge_requests`、`ai_bridge_request_assessments`。缺对象或非 InnoDB 不变成零值。tags 输出 SQL NULL／JSON null／空数组／非空数组／其它形状的守恒分区；roles 与 effective_roles 仅作 MySQL 规范 JSON 二进制空安全比较。历史状态按固定词汇分组，其它状态只计 `unknown`，不输出原值；保留五表总量和内部孤儿引用计数。AI 同时覆盖所有 payload 的期望 assessment 关系（保留重复条目）和实际关系，分别计非法期望 ID、缺关系、孤儿、零 ID及多余关系。三个 actor 字段要求 JSON STRING，org/testee 是正数且规范 uint64 十进制，subject 是非空、最多128字节且无 NUL 的 opaque 字符串；合法 payload 与已索引 identity 作逐字节比较，不把 subject 当数字。该盘点不验证 stored request_hash、所有运行环境或实际业务授权。

单一 pinned Conn 先核身份和实际 clean100，再执行固定 RR／READ ONLY 事务并读取上述11条语句；首条业务一致读建立 InnoDB 视图，后续计数复用它。成功 rollback 后，在事务外实时重读 clean100 和完整 RAM 会话身份，避免旧快照掩盖迁移变化。成功回执精确 `query_count=15`、`control_statement_count=3`；每条5秒和总45秒共用截止，包含释放，不重试或续跑。每条结果最多1000行，1001为截断哨兵；固定字段、非负 uint64、完整标签集合、各分区及跨表守恒不符便丢弃全部 measurements。完成还要求事务、Conn、DB 与 pool 释放全部为 true。回执版本只允许新的 `format_version=1`；`retirement_inventory_only=true`、`freeze_verified=false` 固定不变，只输出三项受控 MySQL 版本数字、结构/快照布尔及封闭计数，不输出数据库身份、账号、SQL、ID或正文。

非零差异、非法字段、孤儿关系及 `unknown` 状态仍是完整观测中的 finding，不自动得出 eligible 或删除许可。生产盘点须独立绑定 source/run/target 回执；本源码批次的隔离验收不包含生产连接或派发，实际生产观测由另行授权任务登记。盘点不证明持续停写、定向值备份、失败恢复、旧新服务启动或30/60分钟与RPO=0。这个一次性工具随 M7 退出，清理完成后不得扩成永久通用查询入口。

源码 `80f422101ccfd83b4c4eb5bd165272145afe7afd` 在自有 MySQL8.0.36 上实际执行同一 required race proof：三个 top 及子场景共24项 RUN/PASS、0 FAIL/SKIP。新 inventory 的8项包括非零/unknown计数、重复期望关系、非法actor和字节差异、真实列权限1143、缺对象与非InnoDB、dirty/其它head、RR并发actor/AI变化及 rollback 后实时版本、全空对象；原 profile 和 readiness 回归保留。原 JSONL SHA256 `54a68e1c8a491e11d6f4ab7923c9eefa63e3971b2101cd28b9c710dc56b4fb6f`，回执 SHA256 `ff72274b13056b70066880e636fa825b0d504eb50b4a90db5b7e8d3481076f38`。自有容器/网络已释放，旧CID/images/volumes保持，所有组件源码前后相同；该 fixture 是限定 reader 结构/计量证明，不冒称生产迁移初始化或实际生产权限。
