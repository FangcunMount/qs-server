# 离线未来重试计划交接（T25-B 候选）

`qs-retry-handoff` 在已停旧写者、封闭准入的维护窗口内，复用原 MySQL/Mongo 根事务的 `TransferLegacy`。它不创建 Sender/Checker、不连接 MQ、不安装 schema、不生成新消息、不评分、不生成报告，也不启动调度或普通业务 Container。

首次CLI开发档已在隔离GitHub验收原MySQL/Mongo，各30项零失败/跳过；当前新增只读盘点仍待实际依赖验收。完整CI、合并及生产批准尚未完成，不能据本文件执行生产交接。M3进度唯一总表由infra的[M3进度总表](https://github.com/FangcunMount/infra/blob/main/docs/server/rocketmq-m3-execution.md)维护。

## 冻结清单与默认校验

清单由 `retryschedule.ReviewHandoff` 从已经核对的原始计划构造。它固定版本、原 owner/store/epoch、执行工具源码 SHA、具名 actor/reason 和原 `Frozen.Bytes()`；上限 1 MiB、100 个计划，单份清单只属于一个原主库。重复调度身份、错误 owner、非规范编码、重复/未知字段及摘要不符均拒绝。审核方独立提供原文件 SHA256，不能把文件自身声明的摘要当审核。

清单包含首次正文/wire，必须保存在受限目录的 0600 普通文件；工具拒绝最终路径软链接、非普通文件和超限输入。清单不是批准或业务授权；仍须从原记录确认完整责任清单、原授权、首次 wire、身份、attempt 和到期时间。实际旧记录盘点/导出与完整性确认仍属 T25-B/T30，不可手造计划或重建密文来绕过。

从审核过的 clean exact SHA 使用 `make build-maintenance` 构建。工具使用编译时 `pkg/version.GitCommit`，没有源码身份覆盖参数。清单的 SHA 必须匹配该二进制；二进制摘要与保护/恢复材料由最终切换清单记录。

默认只校验文件，不读取数据库环境变量或创建连接：

```sh
/private/task-owned/qs-retry-handoff \
  --review-file=/private/task-owned/review.json \
  --review-sha256=<independently-reviewed-digest> \
  --owner=qs.mysql --store-id=<configured-original-store> \
  --writer-epoch=<configured-original-epoch>
```

标准输出只有固定状态、计划数量和 `cutover_authorized=false`。不输出计划、正文、message ID、数据库地址、具名理由或凭据。默认校验不证明原数据库记录仍可交接，也不证明旧写者已经停止。

## 显式执行与原事务边界

在同一组参数上增加以下选项才进入写执行：

```sh
--apply --database=<explicit-original-database> \
--receipt=/private/task-owned/new-receipt.jsonl \
--all-legacy-writers-stopped-and-admission-closed
```

停写选项是操作员声明，不是自动停止机制或生产批准。MySQL 连接只来自受限环境 `QS_RETRY_HANDOFF_MYSQL_DSN`，所选库必须与 `--database` 一致；Mongo 只来自 `QS_RETRY_HANDOFF_MONGO_URI`，库名由显式参数选择。文件不能选择连接、数据库或活跃 owner。原库 SDK binding、调度 schema、原行、当前业务授权与数据库时钟由既有适配器重新验证；不自动安装或修复。

执行前先独占新建 0600 回执并持久化 `started`。已有路径或软链接拒绝，不覆盖旧回执。回执含源码、审核摘要、绑定、具名责任和停写声明；不包含正文或原 message ID。工具只拥有本次临时连接池/客户端，结束时先关闭，再给出完成输出。

每个计划独立使用原根事务：保存原冻结计划并将原未尝试 Outbox 记录改为精确 `held` 标记。原有实现只允许未到期、未尝试、无 claim/lease/confirmation/manual replay 的源；原 Relay 和 Replay 不能再次取得已交接记录。到期或已有发布责任的记录必须先沿旧链路排空/具名处置，不能被此命令补发。

Mongo 驱动 callback 重入只重复这段确定性 DB 交接；不更换 ID、due、授权或 wire。任一失败停止后续计划；已确认前缀保留，提交回复未知的当前计划记录 `unknown`。不得把整个批次假定为已回滚，也不提供跨库原子提交。原样重复同一审核清单只执行幂等元数据核验/交接，不是业务重试授权。

## 回执与后续判断

回执为 JSON Lines：第一行 `started`，第二行含最终状态与逐计划摘要身份/结果。进程提前退出、连接失败、磁盘写入失败可能只有第一行或不完整末行，此时不能推断交接完成；保留原数据库、清单和私有回执，再按原身份核验。

命令成功也始终 `cutover_authorized=false`。它不证明完整责任已穷尽、旧进程已经退出、NSQ 已排空、生产 DDL 已批准、runtime 已启动或 system 验收完成，不解除原生超限治理、不伪造正向 Broker 审计。启动前恢复/启动后前向修复及最终 30 分钟演练由 T25-C/T29 单独验证。

## 只读原账本盘点

切换主流程先关闭新准入，让旧链路排空。旧自动重试通常仅30/60秒延迟，保护和DDL可能跨过到期点；交接命令不改变原due，也不放宽已到期/已尝试责任的拒绝条件。无法提供完整原计划的责任保持旧链路收敛或阻断切换，不能从当前业务状态生成新事件替代。

显式`--inventory`读取原主库完整状态聚合；可选审核清单只用于核验已经交接的源/目标，不创建计划或改写记录：

```sh
/private/task-owned/qs-retry-handoff --inventory \
  --owner=qs.mysql --store-id=<configured-original-store> \
  --writer-epoch=<configured-original-epoch> \
  --database=<explicit-original-database> \
  --receipt=/private/task-owned/new-inventory.jsonl
```

无清单时所有held都计为未核验责任。已交接记录需要另外提供原`--review-file`与独立`--review-sha256`：逐条核对原消息ID、正文/fingerprint、due、无claim/lease/replay、精确held标记以及目标冻结计划。仅有格式正确的标记不够；原消息或目标缺失、矛盾、未知状态或读取失败均不能报零。published历史行保留，有效确认之外的published也保留为阻断项。

MySQL使用服务器只读Repeatable Read事务；Mongo在新snapshot事务中只读，结束后abort，并在两侧检查原集合UUID。检查前后缺失/UUID不符拒绝；这不替代DDL冻结证明。总预算30秒、选择最多100项、单条正文与计划读取有界，借用的主库和准入容量保持到实际边界结束。该维护预算不改变SDK Checker/worker预算。

盘点需要宿主已显式安装的SDK binding和调度schema；缺失就拒绝，不补建或回填。维护窗口前按旧链路盘点/排空，批准的离线安装之后再以此工具复核。全体旧写者、自主任务、旧消息/业务receipt和未来责任的具名清单仍由T25-C/T30控制；本盘点只描述原rm_outbox账本。

| 退出码 | 只读盘点结果 |
| --- | --- |
| 0 | 该快照中原账本非终态为零、held全部精确配对、published确认无缺口；不授权切换 |
| 2 | 有效快照仍有pending/retry_wait/publishing/quarantined、未核验held或缺确认责任；保持封闭 |
| 1 | 参数、绑定、schema/原集合、读取或清理不可用；不能解释为空 |

盘点同样先持久化私有started，再记录完成时间、计数及核验结果。stdout只有固定状态和总量，不含wire、原ID或连接信息。`--inventory`与`--apply`互斥，不接受停写声明冒充自动停写；所有输出仍为`cutover_authorized=false`。

### 2026-10-31 宿主退役 M4 复核

本批未新增或执行数据库迁移、修复、重投或删除。当前发布构建检查三个实际进程的依赖图，禁止 NSQ、发布型 Relay 及历史测试夹具进入生产；旧 Relay runtime closure 明确在固定历史源码运行。重试交接夹具改用中立 `eventmessage` 编码与 Reference 校验，原身份、wire、计划/receipt 格式、原事务原子性与人工恢复权限保持。历史命令和生产证据保留原提交归属，不由本次源码复核补为当前验收。
