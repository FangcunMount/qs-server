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

此前 infra 迁移阶段 M4 已移除仅服务旧中间链路的 NSQ runtime、拓扑 HTTP 预检、发布扫描、旧 Stage/ACK rearm/payload reader、旧一体化探针及其孤立合同。新的负向用例只用 SQL 预置历史事实，不执行退役发布器。

本轮宿主工程退役 M4 将共用编码、Reference 与不可变证据校验迁入 `eventmessage`；原 Revision2 wire、指纹和冻结计划不改写。无生产调用的标准 Profile 构造及 Relay supervisor 已从当前源码删除。仍需回归的旧 Stager、发布策略和 NSQ 适配只供 `internal/testutil/legacy*` 历史夹具使用，实际三个进程的依赖图门禁禁止链接这些夹具、NSQ 驱动及发布型 Relay。SDK 的公开兼容能力不删除。

原 `TestCurrentRuntimeClosure` 属于旧 Relay 机制，现固定在 `f77bd0feb3a3559c0a1bfc5493451ae0fba25c5d` 运行并明确标注历史；旧 NSQ 手工证明入口也复用固定历史工作树。当前主机协议验收继续使用 M3-C 原生 RocketMQ 用例，不能把历史证明计为本轮当前运行验收。

QS AI bridge 的当前启动检查不再要求旧 `ai_messaging_outbox`；准入、operations 退役约束、Inbox、失败审计、隔离和技术观察的检查继续保留。旧表的只读审计入口仍按实际存在情况核验，移除启动依赖不等于允许删除历史数据。

只读数据库审计、准入 revision/CAS、离线正式镜像预检、历史 schema/备份、SDK 通用兼容测试和开发依赖保留。旧状态查询仅用于保留资产审计，不能当成新事务队列指标。没有自动删表、卷、业务历史或事务证据。

## 代码与验证

当前原事务与路由实现见 [架构边界](./10-架构与责任边界.md)。真实 Broker 下的 command、event、Payload/mTLS、原根回滚、跨实例和故障验收由 infra 固定源码 SHA 运行；源码 CI、隔离验证、生产部署与 system 验收分别记录。

完整清理进度与原报告见 [M4 总表](https://github.com/FangcunMount/infra/blob/main/docs/server/rocketmq-m4-execution.md)。旧机制的阶段记录可从 Git 历史追溯，不表示当前生产仍运行这些链路。
