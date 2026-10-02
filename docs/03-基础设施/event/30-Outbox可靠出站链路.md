# Outbox 可靠出站链路

## 1. 当前实现与边界

QS 使用 reliable-messaging SDK 的标准 Outbox 和 Relay。业务服务仍负责在原业务事务中暂存事件、选择业务 Topic、处理消费者幂等和人工治理；SDK 负责标准存储、claim、发布结算和租约恢复。组件不会消除跨数据库或外部副作用的一致性问题。

| Profile | 权威存储 | 业务事件 |
| --- | --- | --- |
| `mongo_domain_events` | Mongo `rm_outbox` | 答卷提交、报告生成／失败／重试授权 |
| `assessment_mysql_events` | MySQL `rm_outbox` | 测评请求／重试授权／结果／失败，以及启用后的任务开放意图 |

配置必须启用已挂载数据库对应的标准 Profile，缺失绑定或禁用标准 Profile 时启动失败。当前源码不提供旧 Outbox 的配置回退；部署回退使用经过核对的旧镜像，并遵守对应数据版本的恢复约束。源码实现不代表生产已更新，生产版本见发布与验收记录。

## 2. 事务提交与投递

```text
原业务事务：业务事实 + SDK Appender.Append
        ├─ rollback：两者一起回滚
        └─ commit：两者同时持久化
                    ↓
            AfterCommit 唤醒 Relay
                    ↓
            SDK 扫描、claim、发布和结算
                    ↓
            消费者按业务事实处理幂等
```

Mongo adapter 通过 `sdkmongo.Bind` 使用原 `mongo.SessionContext`；MySQL adapter 通过 `sdkmysql.BindGORM` 使用原 GORM 事务。没有业务事务时必须拒绝暂存，不另开事务兜底。

`ProfileBinding` 保留 `Stager` 和 `PostCommit` 两个业务端口。`AfterCommit` 没有错误返回，只在成功提交后调用；唤醒丢失仍可通过 SDK 数据库轮询恢复。当前链路不依赖旧 Redis ready-index、ImmediateDispatcher 或 reconciler。

## 3. 失败与恢复

标准未完成状态为 `pending`、`retry_wait`、`publishing` 和 `quarantined`；发送结算成功后为 `published`。SDK Store 使用 claim／租约及结算校验保护并发更新；失败或结果未知按照治理策略安排后续处理，预算耗尽进入持久待处置状态。具体失败分类与退避由 `SDKRetryPolicy` 和业务治理策略决定。

| 失败位置 | 处理边界 |
| --- | --- |
| 业务事务回滚 | 业务事实和事件均不提交 |
| 提交后唤醒丢失 | SDK 数据库轮询发现持久意图 |
| MQ 不可用 | 保留未完成意图，执行有界重试 |
| MQ 接收但发布结算未持久化 | 租约恢复可能再次投递；消费者必须幂等 |
| 自动预算耗尽 | 保留待人工处置事实，按原事件身份授权治理 |
| 不可逆外部调用结果未知 | 保留回执／人工核对，不据此盲目重发外部调用 |

Outbox 提供至少一次投递。`published` 只证明发送结算，不证明消费者业务完成；原消息重投、业务幂等和报告回执仍需分别核验。

## 4. 生命周期和状态

`EventSubsystem` 组装 SDK Profile 与消费者，启动消费者和各 Profile Supervisor；关闭时取消运行任务并等待退出，再执行发布器 drain。状态接口保留业务端口，标准状态快照使用 SDK 形状。消费者独立维护业务消费结果和死信事实。

当前没有 Mongo／MySQL 分布式事务、全局 exactly-once 或统一外部副作用去重账本。业务幂等继续遵循[事件契约与演进](./20-事件契约与演进.md)。

## 5. 代码与验证

- 业务端口：`internal/apiserver/application/eventing`
- 组装：`internal/apiserver/eventing/subsystem`
- SDK 适配与 Supervisor：`internal/apiserver/eventing/standardoutbox`
- Mongo 原事务适配：`internal/apiserver/infra/mongo/standardoutbox`
- MySQL 原事务适配：`internal/apiserver/infra/mysql/standardoutbox`
- SDK 核心：`github.com/FangcunMount/reliable-messaging/{outbox,relay,storage}`

```bash
go test -count=1 ./internal/apiserver/application/eventing \
  ./internal/apiserver/eventing/subsystem \
  ./internal/apiserver/eventing/standardoutbox \
  ./internal/pkg/architecture
```

旧 core／Store／ready-index 及历史混合 Profile 合同从固定旧提交运行，入口为 `scripts/testing/run-retired-outbox-contracts.sh`，清单为 `docs/retired-outbox-contracts.json`。历史合同只能证明固定旧实现的行为，不能替代当前 SDK 的集成验收。删除源码不删除生产历史行、索引、NSQ channel 或回退镜像。
