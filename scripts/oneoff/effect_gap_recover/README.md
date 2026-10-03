# QS-05／06 原业务效果恢复

单事件运维命令，补齐已确认标准意图未到达正常消费者的两种情况：

- `evaluation.retry.requested`：原失败 Run 仍保留同一份到期重试授权，未出现后继接受。
- `evaluation.outcome.committed`：原 Outcome 已提交，报告从未接单；任何已有 Generation、报告（包括软删除）或准入失败证据都拒绝初始恢复。

命令复用正常 Worker 解码／请求映射和现有 Worker mTLS／ACL，调用现役执行入口。它不向 NSQ 广播、不重置 Outbox、不签发新授权、不更换模型／报告模板。评估执行或报告生成仍由正常服务负责；**没有报告不能据此推断模型未调用**。

## 前置核对

固定单个原事件、Assessment、机构及接单截止水位；按正常消费者在途、持久接受事实和外部回执确认可处理，没有未决外部调用。宿主保留一个固定恢复主机上的持久审计目录（绝对路径、0700），不得换主机、目录或请求 ID 绕过已有记录。

源读取使用标准意图的原身份、完整 wire／指纹／确认字段，MySQL 原只读事务快照，以及 Mongo 主节点接受事实。原 Outcome 使用正常不可变事实和报告输入适配器解码，不从当前模型目录补输入。业务 DATETIME(3) 按毫秒存储表示核对，SDK DATETIME(6) 保持原精度；未到期授权不执行。SQL／Mongo 的缺失读取不形成分布式快照，最终竞争仍由正常 Claim 与 Generation 唯一键仲裁。

宿主显式注入 `MYSQL_DSN`、`MONGO_URI`、`MONGO_DB`。apply 还需 `M6_EFFECT_GRPC_ENDPOINT`、`M6_EFFECT_CA_FILE`、`M6_EFFECT_CERT_FILE`、`M6_EFFECT_KEY_FILE`，可用 `M6_EFFECT_SERVER_NAME` 指定服务器名称。没有明文或跳过证书验证选项。连接配置及证书私钥不放命令参数、stdout 或证据文件；工具不自动发现生产环境。

## 操作与回执

1. `--mode=inspect --event-id=... --assessment-id=... --org-id=... --accepted-before=<RFC3339>`。仅输出原标识、源摘要和适用的冻结模板版本，不输出 wire、答案或模型输入。原事件可以是确定性 ID，并非必须 UUID。
2. `--mode=apply` 沿用上述参数和 inspect 的 `--source-fingerprint`，指定 `--audit-dir`、唯一规范 UUID `--request-id`、`--operator`、`--reason`、`--external-result-reviewed`。外部效果前独占持久预留原事件与操作意图、fsync，再读取源；变化则停止。只调用一次正常业务入口。
3. `--mode=reconcile --audit-dir=... --request-id=...` 只读本地原审计，既不连接数据库，也不再次调用业务入口。缺回执／unknown 必须按原 Run／Generation／Outcome 核对权威结果，禁止自动重复。

默认期限30秒，可选1秒至2分钟。退出0为inspect成功或持久回执accepted；退出1为输入、源、配置或预留拒绝；退出2为预留后源改变、结果／回执未知或未决核对。RPC返回原失败Run、blocked、准入拒绝、nil或错误均不能当作新接单；评估要求匹配单步后继Run及attempt，报告要求返回有效Generation／Run身份。

回执保留 `transport_outcome=not_sent`、`effect_outcome=accepted|unknown`、原事件及相关业务身份，`business_completion_proven=false`。accepted可能是新Run已持久分类为失败，**不等于报告已完成、生产时间达标或M6整项通过**。未知回复即使记录了Run ID也不能自动再试。

## 验证范围

单元／race覆盖原授权、冻结关联、未知结果、审计先于调用、回执写入失败、源变化和原操作防绕过；实际无回复MySQL握手验证整体期限。共享审计新增ID字段为omitempty，原报告／答卷工具的旧JSON不变。

`scripts/testing/m6-original-effects-recovery-proof.sh <外部证据目录>` 限定本地Unix Docker和干净提交，创建唯一owner的真实MySQL／Mongo副本集／NSQ。原失败及重试授权、Outcome提交均走现役事务；原Relay确认后同broker同卷SIGKILL丢内存消息，再运行实际命令经生产mTLS／ACL及正常Claim、报告Starter／Executor／提交。迟到原wire走正常SDK订阅／Worker／FIN；核对唯一后继Run、Generation／报告及原published行不变。仅清理自有临时资源。

隔离输入解析失败、种子计算结果和报告内容builder为固定fixture；实际数据库／RPC／事务不替换。该证明不代表外部模型质量、微信、生产P95／300秒或压力容量。缺少环境必须失败，不以SKIP验收。
