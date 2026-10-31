# 宿主工程退役：只读责任核对

该工具仅核对固定 `iam.mysql` / `qs.mysql` 原发布表的存在性、结构哈希、实际 schema head 和原状态字节分组。版本头缺失、多行或 dirty 都会使核对失败；记录实际 head 不表示该版本已获退役批准。使用目标进程已有的明确数据库与 VERIFY_IDENTITY TLS 绑定，在有界、REPEATABLE READ / READ ONLY 事务内读取；不安装 schema、不启动 Relay、不回放或修改业务状态。

`publication_closed_rows` 只表示状态恰为 `published` / `confirmed`，不是业务效果完成、ACK、历史引用关闭或删表许可。NULL、大小写不同、尾随空格和未知状态都属于未关闭。缺表、缺状态字段、超时或身份不可读会失败，行数保持 null，不能据此推断零积压。结构哈希也不等于已验证历史正文及全部业务约束。

MySQL 伴随工具只有确认事务回滚和自有连接池关闭后才可报告完整；释放未知即失败。运输入口还要求进程成功退出，不能保留失败进程返回的“完整”声明。AI 审计也先关闭连接池，再输出最终结果。

`release-snapshot.yml` 的 `messaging_retirement_inventory=true` 入口要求完整 `reviewed_source` 等于工作流 SHA。该模式排除默认主机快照，不执行其 root Python 命令；仅复用现有 SSH/Docker 运输，在应用 UID 下运行临时只读伴随工具。AI 分支固定应用 UID `10001:10001` 和镜像启动解释器 `/app/.venv/bin/python`，通过已安装应用的 `Settings()` 读取原数据库及 TLS 绑定，复用 `messaging_audit.inventory`，保存原文哈希与元数据，完整性/截断结果原样保留。失败回执只记录阶段、异常类型和整数驱动码，不记录异常正文、连接串或凭据；无有效回执时，读写效果保持未知，不能推断空表。它不是原正文导出、备份或恢复证明。Mongo 历史读取仍为明确源码依赖；新增集合核对见下文，不能由 SQL 结果替代。

M4 实际部署和回读完成前不执行现网核对。即使报告完整且所有发布状态关闭，`drop_authorized`、`business_responsibility_closed`、`original_body_export_verified` 和 `writers_excluded` 始终为 false。实际删除仍需逐对象的引用与业务责任证明、导出/恢复验证、全新库和升级库测试、回退下限，以及用户审查具体执行方案。

普通单元测试覆盖缺表、结构/字段读取失败和超时。既有 MySQL Native Metadata and Readiness CI 另在独立拥有的本地 MySQL 8.0.36 namespace 验证原状态字节、NULL、未知值与只读事务拒绝写入；不借用生产绑定。

## Mongo 固定集合核对

同一运输支持 `qs.mongo`，固定读取应用显式绑定的 `qs.rm_outbox`，以已校验的独立伴随二进制调用现有元数据工具的 `--messaging-retirement-inventory` 入口。旧元数据入口及其固定历史 head 不变。

计数只读取状态字段，采用单次 `readConcern=snapshot` 聚合与二进制字符串比较；只有精确字符串 `published` 归为发布状态关闭，NULL、缺字段、数组、大小写及尾随空格均保留为其他状态。查询前后复核 schema head/dirty 与集合 UUID/目录哈希；缺集合或身份变化不是零行。该过程不声明多次读取处于同一事务、不证明停写或业务责任关闭，不导出消息正文。

复用既有十条命令预算、响应上限及游标释放校验，另限总读取 25 秒、聚合 2 秒；结束时关闭自有连接池，释放不确定即失败。原生 CI 必须在独占、认证 Mongo 7.0.31 副本集执行旧元数据验收及新增只读账户/原状态类型计数验收，两者均 RUN/PASS、零跳过。源码兼容与原生测试通过不等于已读取生产集合。

本工具用于退役前的原对象核对。SQL111 候选退出旧发布表后，对该旧对象的读取仍保留缺席/null 数量和不完整语义，不将缺表伪装为零行；删后的正常运行验收读取实际 schema、当前启动/指标与业务恢复，不要求重新创建已退役发布表。

只对同一只读快照中已确认零行的 QS `ai_messaging_outbox`，探针另读取原 DDL 并实际读取 body/wire 流到 EOF。完整成功退出及事务/池释放后，运输才以0600创建原 DDL和零字节 JSONL材料；保留原结构与空流哈希。非空或 EOF/释放未知不生成空材料。该结果仍不证明停写、实际恢复或删表许可；必须将真实原结构与独立恢复结果按哈希对应后另行审查。

本批窄范围退出还要求完整元数据可见性，并在执行前及目标锁内复核 schema 例程/触发器/事件及指向目标的外键依赖均无未审查对象、外键检查开启。只读离线检查与空流材料生成也执行该依赖检查；失败保留未知，不删除附属对象或按表空推断无依赖。

同一已审核 SHA 的 `release-snapshot.yml` 另固定构建原离线执行器，并在 `qs.mysql` 分支以应用 UID 执行 `--application-env` 默认只读预览；独立保存源码、二进制摘要、实际 110/44、候选 111/44、嵌入资源及所选双库身份。只接收执行器真实的 `read_only: true` / `snapshot` 输出；进程失败、非只读或身份/版本不符均为不可用。它不生成批准、停写、修改版本或执行 DDL。

空表保护材料还包含同一只读快照中的完整原 `schema_migrations` 账本：验证恰好 version/dirty 两列的实际类型、单行 clean 原值，并保存原 DDL/哈希。未知列、dirty/多行或释放失败使整体材料不完整；不是仅按预期目标数字重建版本。具体恢复只可在独立审定、原对象复原和保留事实核对后事务回放完整原账本，不提供通用 Force/down 或生产修复授权。

## 停机后的维护传输

默认运输仍在原应用容器中执行。全部 QS 进程停止后，需要独立的维护容器，不能把停机前快照冒充同窗口保护材料。

[维护传输入口](../../cd/m5_maintenance_container.py)固定从原 `qs-apiserver` 的本地不可变 image ID、数据库环境和只读借用挂载/网络派生配置。默认仅 inspect 及 `sudo -l` 核实必要 Docker 命令权限，不创建容器或连接数据库。显式 `--stage` 才创建并启动 `qs-m5-maintenance-<run>-<attempt>`，PID1只运行 sleep，固定UID2000，不运行应用或数据库客户端。原镜像ID来自运行快照；不接收服务tag或另一宿主名称。凭据只经过0700临时目录中的0600 env文件，不进入argv、输出或制品；临时文件在创建后移除。原API已经停止也可派生运输，原挂载只读，独立/tmp用于固定维护程序和私有批准。原API必须符合既有Compose的 `infra-network` 与 `qs-network` 双网络合同；维护入口仅连接固定数据库 `infra-network`，不复制业务网络或API别名，缺失/额外网络仍拒绝。这里的原镜像ID严格指同主机Docker快照的容器 `.Image`，不能用OCI config digest代替。

`release-snapshot.yml` 的 `maintenance_transport=preflight|stage` 与 `maintenance_image_id` 提供单独的显式入口，要求 reviewed_source 等于实际工作流SHA；不能与 messaging inventory 或已有维护容器参数混用。该入口使用普通SSH用户的Python和原有sudo Docker权限，不申请通用root Python权限。已有 `SVRA_SUDO_PASSWORD` 通过0700/0600本地临时引导文件及SSH私密stdin传递，仅固定Docker命令使用sudo；不在参数、输出或制品中传密码，也不以root执行Python。错误只输出固定分类，不回显Docker原始stderr。独立维护预检不触发三主机旧快照步骤。preflight只验证权限；stage保持新建的空闲容器及其操作标签，不签发停写或数据库批准。

后续只读盘点单独设置 `messaging_retirement_inventory=true`、`maintenance_container` 和相同 `maintenance_image_id`。仅QS MySQL/Mongo可切换到该容器；运输在执行前复核所有权标签、image ID、UID、只读挂载及固定空闲命令。默认只读离线检查同样支持该运输，原110/44与目标111/44校验不变；IAM/AI继续使用各自原应用。写执行只可使用原canonical执行器的私有批准入口；这里没有自动apply、恢复或生成批准的功能。

镜像原健康命令也显式禁用；容器形状复核要求只有固定空闲命令和 NONE 健康配置。已有名称或不确定的中间状态必须保留并拒绝复用；不自动删除容器、旧批准或消费标记。维护结束后，具名责任人先核验并归档原保护材料、批准原文/摘要、消费标记及实际结果，按批准保留至少30天，才可明确清理这一次操作拥有的维护容器。首次新runtime之后禁止回放旧原账本。生产停写、owner确认观察排空、实际恢复和删除仍须独立审查。

既有 Deployment Contracts CI 另在独占Docker资源中验证真正停止的源容器仍可建立维护传输，UID/秘密筛选/挂载只读/原应用入口不运行/旧操作拒复用和自有资源释放。该测试不启动数据库、不证明生产sudo权限或停写，不计入原生业务14项。
