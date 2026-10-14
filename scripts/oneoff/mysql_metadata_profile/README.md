# MySQL 只读权限诊断

此工具只读诊断当前 session 的 SHOW GRANTS，以及固定 RDS 角色的潜在权限集合。它不激活角色，不替代现有删除门禁，不证明清理资格，不读取业务记录。

工作流提供两个独立的只读入口：`metadata-visibility-profile` 使用现有应用账号；`metadata-visibility-admin-profile` 使用 production 环境中的 `MYSQL_METADATA_ADMIN_USERNAME`、`MYSQL_METADATA_ADMIN_PASSWORD`，复用既有 host/port/database，并保持上述生产目标 hash 校验。管理员 Secret 缺失时停止，绝不回落到应用账号。CBPT 清理四个入口固定使用这组管理员身份，但原有全局元数据、依赖、锁、内容与恢复证明门禁不变。新工作流 source SHA 必须重新归档和隔离恢复，原归档及证明保留。 CBPT 手工入口必须提交 `governance_confirm`，在 checkout 和执行前核对实际 GitHub SHA，防止派发期间分支移动导致执行未审查版本。

固定输入：PROFILE_SOURCE_SHA（40 位小写十六进制）、PROFILE_RUN_ID（GitHub run-attempt 数字格式）、PROFILE_EXPECTED_TARGET_HASH（64 位小写十六进制），以及 MYSQL_HOST/PORT/USERNAME/PASSWORD/DATABASE 五项环境变量。没有命令行业务参数；额外 argv 或格式不合法时只输出固定失败。workflow 将预先验证的生产 target hash 作为固定绑定传入；不包含生产连接或凭据。

单一 pinned sql.Conn、总 deadline 45 秒、每个 query 5 秒、connect 5 秒，MultiStatements/InterpolateParams=false。配置和 driver 全局 logger 都为 NopLogger。身份 query 读取 session ID/UUID/database/version/CURRENT_ROLE，前后必须完全一致；UUID 仅用于兼容现有 cleanup hashText(host, normalizedPort, database, UUID)，hash 必须匹配 PROFILE_EXPECTED_TARGET_HASH。身份值、角色原文和 SQL 结果不进入输出。

权限与角色诊断字段的含义：

- current_unrestricted_metadata_grants：固定 SHOW GRANTS FOR CURRENT_USER 的当前有效集；绝不称它仅为 direct grants。
- rds_role_grants_available：固定 SHOW GRANTS FOR CURRENT_USER USING `rds_superuser_role`@`%` 查询成功为 true；只有 MySQL 数字错误 3530 且 SQLState 为 HY000 才为 false；其它查询错误为 null 并失败。
- rds_role_unrestricted_metadata_grants：上述 USING 查询的账号直接权限加固定角色的潜在集，不能称当前有效集，也不把其它 active roles 并入它。角色未授予或查询失败时为 null。角色输出格式不被识别时为 null、complete=false；不输出角色结论。

- assigned_roles_present：当前规范 SHOW GRANTS 结果是否包含明确已授予角色的行；不输出角色身份，也不证明它们有管理员权限。
- mandatory_roles_present：固定 SELECT @@GLOBAL.mandatory_roles 是否为非空；只返回有无，不输出原始配置。仅 complete=true 时两个有无字段才为布尔，失败时为 null。查询不激活角色。

3530 为 ER_ROLE_NOT_GRANTED，已核对 [Oracle/MySQL 8.0 官方错误参考](https://docs.oracle.com/cd/E17952_01/mysql-errors-8.0-en/mysql-errors-8.0-en.pdf) 的该条目；没有用文字错误匹配推断未授予。

GRANT 解析只认可 bounded 的 privilege/account/scope/role/proxy 语法；静态权限使用固定列表，合法全局动态大写标识符可识别但不贡献 SELECT/SHOW VIEW/TRIGGER/EVENT。全局 ALL PRIVILEGES 展开为这四项后判断；非全局 ALL PRIVILEGES 不贡献全局权限。canonical REVOKE 为已知负结论（该组无论其它行如何都 false），仍继续另一个固定潜在集诊断。未知/畸形输出失败关闭；有 SQL 字面文本、角色或账户的输入不被拼接到固定查询。

RDS 完整 static/dynamic 多行 fixture 依据 [AWS RDS 官方角色模型](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Appendix.MySQL.CommonDBATasks.privilege-model.html) 的 8.0.36+ 权限列表。测试包含 APPLICATION_PASSWORD_ADMIN/ROLE_ADMIN/SET_USER_ID/XA_RECOVER_ADMIN；它们单独出现不能假造四项元数据权限。

输出仅受控 receipt 字段：format_version/source_sha/run_id/expected_target_hash/source_target_hash/current_unrestricted_metadata_grants/rds_role_grants_available/rds_role_unrestricted_metadata_grants/assigned_roles_present/mandatory_roles_present/diagnostic_only/complete/error_category。无 raw grant、role/account/host/database/version/UUID/SQL/driver error 或凭据。diagnostic_only 永远为 true；即使两个权限布尔均 true，也不是删表授权或资格。未知错误和身份变化时 current 为 false、四个角色字段为 null，complete=false、exit1；合法 negative（含未授予/partial restriction）为完整诊断，不是权限赋予。

离线 sqlmock 测试覆盖未激活固定角色 potential=true/current=false、其它 active role current=true/potential=false、3530/HY000 与缺失或其它 state、3523/1045严格区分、RDS完整grant语法、合法REVOKE继续潜在诊断、畸形语法/SQL注入后缀拒绝、未知错误私密、session/role变化丢弃正结论、expected target mismatch不读取权限、driver logger/multiStatement边界。通过 Go race/vet 与 Python 传输测试后，workflow 才会运行诊断。运行结果应独立绑定 source/run/target hash。

两个 metadata 工作流入口沿用 Mac mini 自托管 runner 的 `root@001-ysj` SSH alias，使用已有 `~/.ssh/known_hosts`，逐次要求 `StrictHostKeyChecking=yes`、无密码交互、新连接及关闭 agent/X11 转发；接收器在读取秘密 envelope 前核实 root、Linux 和 `serverA` 主机名；不下载或修改主机密钥、SSH 配置或权限。缺少可信 alias、已登记 host key、所选管理员 Secret 或完整目标绑定时停止，不回落到应用账号。其余 db-ops job 在两个 metadata 输入下均不符合执行条件。

`mysql-metadata-profile-ssh.py` 将唯一固定文件集的 SHA-256 归档及凭据通过 SSH stdin 传入；密码不进入命令行、日志或 Git。远端只在任务拥有的 0700 临时目录解包，使用 Linux 静态 binary，不创建生产 Docker 容器、不拉取镜像。临时 JSON 环境文件为 0600，固定字段经过验证，不经 shell source/eval，并在建立数据库连接前删除该文件；binary 不继承无关凭据或 LD_* 环境。SSH 输出限 256 KiB/90 秒，binary 输出限 64 KiB/50 秒，stderr 只排空不保留，原始异常不输出。只有 task-owned 文件完整释放后才输出安全密封回执，传输、清理或回执错误均失败关闭。封装失败使用四个固定类别：`profile_input_preflight_failed`（操作、输入与绑定）、`profile_package_preflight_failed`（本地包、checksum 与 envelope）、`profile_trusted_ssh_preflight_failed`（本地可信 SSH 预检）、`profile_remote_or_receipt_failed`（实际 SSH、远端执行、回执与释放后输出）。这些类别只描述本次源码的边界，保留 complete=false、权限 false/nullable 字段，不含异常正文、trace、凭据或远端状态猜测；remote 类别不能单独证明是否连接数据库。原来的 `profile_transport_or_receipt_failed` 继续接受为未定位失败。已通过原验证器的具体 SQL 失败回执沿用原类别，不被封装阶段改名。

`complete=true` 还要求 pinned Conn.Close、owned DB.Close 均成功且 pool 的 InUse/OpenConnections 为零。池暂保留最多一个 idle connection，让紧随其后的 DB.Close 能返回底层 driver.Close 错误，避免 MaxIdleConns=0 在 putConn 中忽略该错误。`connection_release_failed` / `database_release_failed` 是封闭失败类别，清空所有权限和角色结论，不输出底层错误。隔离 MySQL8.0.36 required race 门禁验证 schema-only、全局四权限、未激活固定角色、partial revoke、错误目标及会话释放；释放错误使用明确测试 seam，不能冒称真实数据库物理释放失败。

原 `mysql-metadata-profile.py --binary` Docker 路径及其 negative 测试仍保留，但 metadata 工作流通过受限 SSH receiver 调用 `execute_native`。CBPT 删除工具、其他 db-ops 操作和共享传输 helper 保持原样。


## 日志回执运输

最终日志只编码 `safe_receipt` 已验证的非秘密字典，内部 Go JSON 与原 `safe_receipt` 输入接口保持不变。共享 `scripts/dbops/receipt-transport.py` 再按显式字段、严格布尔/无符号计数、SHA/hash/run ID 和固定 token 白名单检查；不接受原始日志、GRANT、账号、连接或凭据字段。运输编码用于避免平台掩码损坏安全摘要，不用于传输秘密。

UTF-8 JSON 每个 byte 映射到私用字符 U+E000..U+E0FF，以 U+E100/U+E101 框定；必要时整体改用 +0x200 或 +0x400 的独立字母表。若完整 frame 含选中的 MYSQL_USERNAME 或 MYSQL_PASSWORD 任一非空值，就尝试下一字母表；全部碰撞或编码器不可用时失败关闭，不输出明文 JSON 回退。错误输出也只使用本地固定白名单失败字典，原始异常永不进入编码。

共享 API：`encode_armored_receipt(value, schema=..., secrets=...)` 返回 frame，`decode_armored_receipt(text)` 返回 JSON 文本，错误为固定类别的 `ReceiptTransportError`。schema 是可选字段白名单；字段完整性与绑定仍由原验证器负责。JSON body 上限 64 KiB、日志上限 4 Mi 字符；解码必须恰有一个完整 frame，字母表和 UTF-8 合法、无重复 JSON key。解码后仍须原 source/run/target/exit 验证，不能把可解码等同于清理资格，也不尝试修补旧的被掩码 JSON。共享与 profile 离线测试覆盖短字符掩码、备用字母表、缺失/重复/混合或超限 frame、凭据碰撞、未知 raw 字段及绑定拒绝。

当前权限不足时，诊断进一步输出四项全局 GRANT 是否存在及是否存在部分 REVOKE，均为布尔字段，不含账号或语句。字段只在目标、查询和最终同一会话身份全部验证后发布；失败均为 null。它们解释现有权限结论，不改变清理门禁。schema 级授权不能代表全局可见性，正向 GRANT 与 REVOKE 同时存在也不能代表完整权限。旧摘要仍可解析；新字段若出现必须五项完整且与原 current_unrestricted_metadata_grants 一致。
