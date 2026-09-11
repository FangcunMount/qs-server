# 受试者门店归属维护

此工具只处理历史受试者的首次门店归属，不修改 IAM 授权，也不替代后续 Scope 数据范围建设。正式执行前暂停受试者归属、医生归属及管理关系写入。数据库连接串通过 `TESTEE_STORE_MYSQL_DSN` 提供，不放在命令行或日志中。

## 只读检查

```sh
testee-store-migrate preflight --max-rows 150000 --output /restricted/preflight-new.json
testee-store-migrate status --migration-id ownership-202609 --max-rows 150000
testee-store-migrate verify --migration-id ownership-202609 --max-rows 150000
```

预演文件包含人员标识，只能保存在受限目录。文件必须不存在，以权限 0600 创建。控制台仅输出摘要。没有有效管理关系的受试者标记为 `deferred`，保持未配置；这不表示拥有全公司访问范围。其他异常仍阻止执行。行数上限同时约束预演与事务锁定，需覆盖历史和有效记录，超限会停止，不截断执行。

`status` 区分 pending、applied_unchanged、applied_drifted、rolled_back、invalid。状态可读取不代表验收通过；`verify` 仅接受当前事实、清单及审计一致的 applied_unchanged。漂移不得通过修改原清单基准消除。

## 执行与补偿

写入使用现有 IAM 服务身份的双向 TLS 凭据。配置以下环境变量，值分别为可信服务地址和证书文件路径：

- `TESTEE_STORE_IAM_ADDRESS`
- `TESTEE_STORE_IAM_CA`
- `TESTEE_STORE_IAM_CERT`
- `TESTEE_STORE_IAM_KEY`

工具实时读取执行人的 IAM 权限，要求无条件 QS 管理能力，并在事务内确认目标公司的有效 Operator。`--actor-id` 是该 Operator 的 IAM UserID。运行工具本身需要受限运维环境及服务凭据；不能将其作为对外匿名接口。

```sh
testee-store-migrate apply --migration-id ownership-202609 --fingerprint ORIGINAL_HASH --org-id COMPANY_ID --actor-id IAM_USER_ID --writes-paused --max-rows 150000 --timeout 5m --output /restricted/apply-new.json
testee-store-migrate rollback --migration-id ownership-202609 --fingerprint ORIGINAL_HASH --org-id COMPANY_ID --actor-id IAM_USER_ID --writes-paused --max-rows 150000 --timeout 5m --output /restricted/rollback-new.json
```

`--writes-paused` 是维护窗口已落实的确认，不会自动关闭线上入口。写入锁定并复核事实，归属、清单与历史在同一事务内提交。已完成且无漂移时 Apply 返回原清单，重复执行不增加历史；回滚仅撤销本次首次归属，保持暂缓和原有归属不变，递增版本并单独记录补偿。已回滚标识不得再次 Apply。出现后续变化则拒绝自动覆盖。

报告写入失败或连接结果不明时，先运行 status 核对数据库结果，不得猜测失败后重复更换迁移标识。回滚保留原迁移历史，不通过删除历史或降低版本恢复状态。

## 隔离演练

真实 MySQL 专项使用 `QS_SERVER_TEST_MYSQL_DSN` 指向隔离 MySQL 8.0，`QS_TESTEE_STORE_REQUIRE_MYSQL=true` 强制缺失配置失败。规模演练额外设置 `QS_TESTEE_STORE_SCALE_TEST=true`，运行 `TestMySQLMigrationProductionScale`；它创建独立临时数据库，使用 55,576 条候选及 28,220 条无关系合成数据，验证完整迁移、重复执行及补偿，不读取生产个人数据。规模演练不能代替生产迁移后的业务访问验收。
