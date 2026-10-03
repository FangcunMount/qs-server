# QS-03 单份原答卷测评效果恢复

此命令只修复原标准答卷已持久接单、原 Outbox 已确认，但从未存在 Assessment 的缺口。复用正常 Worker 的原事件解码及 EnsureAssessment 请求映射，经现有 mTLS／生产 ACL、Journey 和 Intake 原事务创建唯一测评及下游意图。**不向原 topic 广播，不重置 Outbox，不调用模型，不修复已接单 Run，也不补发热度或微信。**

## 使用条件

指定恢复主机使用一个固定、持久、绝对路径、权限 0700 的审计目录；目录和记录必须保留。应用前明确限定单个答卷、机构、接单截止水位，并核对正常 Worker 在途、原接单／外部结果，保留操作责任人和原因。只读核查发现缺口不等于已经取得修复许可。禁止另换目录、主机或 request ID 绕过已有预留及未知结果。

宿主显式注入 MONGO_URI、MONGO_DB、MYSQL_DSN；仅 apply 还需 M6_QS03_GRPC_ENDPOINT、M6_QS03_CA_FILE、M6_QS03_CERT_FILE、M6_QS03_KEY_FILE，可用 M6_QS03_SERVER_NAME 指定服务器名称。使用现有 Worker 证书及其最小权限；没有明文或跳过证书验证选项。连接配置／私钥不放命令参数或证据输出。CLI stdout 是标识／摘要／回执 JSON，运行日志写 stderr，不输出答卷答案或原 wire。

## 操作

1. `--mode=inspect --answersheet-id=... --org-id=... --accepted-before=<RFC3339>` 读取原 Mongo 主节点快照及 MySQL。只接受同原事件 ID、机构、冻结准入、问卷／模型版本、填写人、原 payload 摘要、确认事实和预期索引一致的标准已接单答卷。任何已存在测评，包括软删除记录，拒绝修复。
2. `--mode=apply` 使用上述固定参数和 inspect 返回的 `--source-fingerprint`，加 `--audit-dir`、唯一 `--request-id`、`--operator`、`--reason`、`--external-result-reviewed`。在任何效果调用之前独占写入原事件预留及操作意图并 fsync；随后再次读取源，变化则停止。
3. `--mode=reconcile --audit-dir=... --request-id=...` 只读本地原操作，不连数据库或 RPC、不再次执行。缺回执／unknown 都需按原答卷核对 MySQL 权威效果，禁止自动重试。

默认整体期限 30 秒，允许 1 秒至 2 分钟。退出 0 是 inspect 成功，或 RPC 返回已有／新建 Assessment ID，或该操作本地回执为 accepted；**它不是报告完成、生产时限或整个 M6 验收**。退出 1 是输入／源／配置拒绝；退出 2 是预留后源变化、效果结果未知、回执持久化失败或未决对账。apply 回执 `transport_outcome=not_sent`，`effect_outcome=accepted|unknown`，保留原事件及 Assessment ID，`business_completion_proven=false`。

数据库读不是分布式快照；正常消费者在最终核对后到达，仍由原答卷唯一键与 Intake 幂等事务收敛。原回复丢失不能以另建操作重做；明确核对已有效果后人工关闭原疑点。工具不追加隐含审批、后台调度或任意宽限期，截止水位应由当次有界操作方案固定。

## 验证

相关单元／race 验证源变化、组织／冻结输入冲突、缺确认、未决操作、审计先于调用；既有报告重试工具的原审计测试继续覆盖排他预留和日志兼容。`scripts/testing/m6-qs03-original-recovery-proof.sh <外部证据目录>` 只接受本地 Unix Docker 与干净精确提交，创建带唯一 owner 标签的 Mongo 副本集／MySQL／NSQ。真实原答卷事务、Relay 确认、同容器同卷 SIGKILL137 后丢消息，再运行实际命令经生产 mTLS／ACL恢复。随后晚到原 wire 走正常 SDK Subscriber／Worker Dispatch／FIN，检查唯一测评和意图、冻结输入、原 Outbox 不变、软删除拒绝及独占审计。最后核实只清理自有临时容器／卷。该隔离验证不用生产数据库，不调用真实模型／微信，不代验生产负载或持续观察。
