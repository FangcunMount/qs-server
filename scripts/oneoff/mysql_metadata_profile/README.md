# MySQL 只读权限诊断

此工具只读诊断当前 session 的 SHOW GRANTS，以及固定 RDS 角色的潜在权限集合。它不激活角色，不替代现有删除门禁，不证明清理资格，不读取业务记录。

固定输入：PROFILE_SOURCE_SHA（40 位小写十六进制）、PROFILE_RUN_ID（GitHub run-attempt 数字格式）、PROFILE_EXPECTED_TARGET_HASH（64 位小写十六进制），以及 MYSQL_HOST/PORT/USERNAME/PASSWORD/DATABASE 五项环境变量。没有命令行业务参数；额外 argv 或格式不合法时只输出固定失败。workflow 将预先验证的生产 target hash 作为固定绑定传入；不包含生产连接或凭据。

单一 pinned sql.Conn、总 deadline 2 分钟、每个 query 15 秒、connect 5 秒，MultiStatements/InterpolateParams=false。配置和 driver 全局 logger 都为 NopLogger。身份 query 读取 session ID/UUID/database/version/CURRENT_ROLE，前后必须完全一致；UUID 仅用于兼容现有 cleanup hashText(host, normalizedPort, database, UUID)，hash 必须匹配 PROFILE_EXPECTED_TARGET_HASH。身份值、角色原文和 SQL 结果不进入输出。

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

Python 传输使用 0700 临时目录中的 0600 环境文件向只读短时容器传入凭据，文件随退出删除；容器仅挂载诊断 binary，不挂载归档或数据库数据卷。当前 cbpt 删除工具与门禁保持原样。
