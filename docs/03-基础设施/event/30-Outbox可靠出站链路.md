# 事务消息与业务效果恢复

当前可靠发布使用 RocketMQ 事务消息。本文保留原路径供链接使用；新业务事实提交不依赖扫描发布型 Outbox。

## 原事务与两个 Profile

先预查幂等、冻结首次消息 ID/时间/wire/路由，准备全部半消息，再由宿主开启原根事务。执行 guard、业务、业务证据、全部 witness 共同提交。MySQL 借用原 SQL/GORM 根事务，Mongo 借用活跃原 session；callback 重入只重复确定性的数据库操作。

半消息失败/未知不启动业务。`LocalCommitted + FinalUnknown` 保留成功的本地事实，由精确 Checker 恢复 Broker 确认；不能重执行业务、重新签名加密或调用模型。取消、超时和 lease 到期本身不证明回滚。没有业务根事务时必须拒绝发送；当前源码不提供旧 Outbox 的配置回退。

## 业务效果与未来调度

消费者必须幂等，继续承担业务状态裁决。Run/Outcome、Generation/Report、Task 收件账本、Inbox、receipt、首次业务 ACK 与不可变正文档案保持各自职责，不因旧命名或表中有 outbox 就删除。

未来 retry decision 保留原授权、due、attempt 和冻结事件，由 `retrydispatch` 到期准备半消息并在原事务 CAS 提交 dispatch 及 witness。协议维护不授予业务重试许可，不提前到期，不增加模型调用。

## AI command、receipt 与 ACK

`MessagingCommandStore` 只进入当前 `TransactionCommands`；缺适配在存储/Seal 前拒绝。接收 AI 事件只进入 `TransactionEvents`，Inbox、投影、首次 ACK 档案和 witness 同根提交。缺原 command Committed 档案的 receipt 回滚并隔离，不确认旧投递。当前 Operation/Payload 不读取旧 Outbox fallback。

重复当前请求返回原计划或结果，保留首次 wire。历史 command ID 仍须参与负向冲突检查；它不能授权新执行，也不能冒充 witness。旧 Inbox 不能被新 ACK 事务领养。

## 保留资产与退出范围

M4 已移除仅服务旧中间链路的 NSQ runtime、拓扑 HTTP 预检、发布扫描、旧 Stage/ACK rearm/payload reader、旧一体化探针及其孤立合同。新的负向用例只用 SQL 预置历史事实，不执行退役发布器。

只读数据库审计、准入 revision/CAS、离线正式镜像预检、历史 schema/备份、SDK 通用兼容测试和开发依赖保留。旧状态查询仅用于保留资产审计，不能当成新事务队列指标。没有自动删表、卷、业务历史或事务证据。

## 代码与验证

当前原事务与路由实现见 [架构边界](./10-架构与责任边界.md)。真实 Broker 下的 command、event、Payload/mTLS、原根回滚、跨实例和故障验收由 infra 固定源码 SHA 运行；源码 CI、隔离验证、生产部署与 system 验收分别记录。

完整清理进度与原报告见 [M4 总表](https://github.com/FangcunMount/infra/blob/main/docs/server/rocketmq-m4-execution.md)。旧机制的阶段记录可从 Git 历史追溯，不表示当前生产仍运行这些链路。
