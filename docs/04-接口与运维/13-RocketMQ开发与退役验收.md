# RocketMQ 开发与退役验收

QS API、Worker 与 IAM 的开发入口使用 RocketMQ。API 使用 `qs.mysql`、`qs.mongo` 两个事务 owner，Worker 只消费；IAM 使用 `iam.mysql`。旧 NSQ 与发布型 Outbox 不再是启动前置。

## 开发前置

1. 准备隔离 MySQL、Mongo replica set、Redis 和带 Proxy 的 RocketMQ。
2. 按当前迁移工具安装 schema，并建立与开发配置一致的数据库绑定。API 和 IAM 的 `migration.enabled` 必须为 false；常驻进程不执行初始化或升级。
3. 将开发 Broker 的 CA、凭据和路由清单放入各仓 `configs/rocketmq/`。文件名见对应 `apiserver.dev.yaml`、`worker.dev.yaml`；这些文件不提交 Git。路由清单使用当前 SDK 格式，逻辑 Topic 与事务 owner 与现行契约一致。
4. 配置中的 `qs-dev-mysql`、`qs-dev-mongo`、`iam-dev-mysql` 和 `dev-1` 仅用于新建隔离环境。已有环境沿用数据库内实际 binding，不通过改名重新初始化。
5. 准备现有 gRPC 证书、IAM 和应用配置，再从仓库根目录启动 API、Worker、IAM。AI 沿用其 `configs/README.md` 的完整配置要求。

开发模板表达真实运行约束，不会自动生成 Broker、账号、证书、路由或数据库绑定。缺少这些前置时不得切回 NSQ 或启用旧 Outbox。

## M1 验收

- `bash scripts/cd/test-verify-observability-nginx.sh`：无 NSQ DNS/路由也可通过；Worker 副本不足仍失败。
- `go test ./internal/pkg/configcontract ./internal/apiserver/process ./internal/worker/process`：使用实际配置解码器验证开发配置进入最终事务／只消费分支。
- IAM：`go test ./internal/apiserver/options`。
- 完整开发启动和正常生产发布分别记录，单元与配置检查不代替运行验收。

应用 Nginx 观测入口只检查 Collection、Worker 和 Prometheus。Broker 协议状态使用 infra 的 `rocketmq-transactions.yml` 和既有 RocketMQ 观测流程，不通过 NSQ `/stats` 代替。

## 本轮交付状态

| 里程碑 | 源码与隔离验收 | 运行／生产验收 |
| --- | --- | --- |
| M1 发布与开发入口 | 移除 NSQ 发布门禁，更新开发配置；检查结果见交付记录 | 尚未执行完整联调和生产发布 |
| M2 指标与后台 | 当前回执统计、后台页面与事务看板已更新；MySQL、UI 与 Prometheus 定向检查通过 | 未部署 |
| M3 恢复与 CI | 未开始 | 未执行 |
| M4 旧代码清理 | 未开始 | 未执行 |
| M5 旧表退役 | 未开始；M4 部署后才进入 | 未执行，无生产 DDL |

旧发布表在 M5 之前保留。状态历史、业务 ACK、会话撤销任务、事务凭证和调度记录继续保留。

## 当前消息指标

- `qs_server_ai_mq_awaiting_command_receipts`：有已提交事务档案、尚未收到 accepted/rejected 的当前命令；held 单独统计。
- `qs_ai_mq_pending_interpretation_receipts`：由 MQ 持有、尚未收到 QS 业务 ACK 的解读状态历史；不代表所有评测事件或 Broker 队列深度。
- `reliable_messaging_transaction_*`：继续负责原事务准备、确认观察缺口与 Checker 状态。
- `*_observation_available=0`：本次读取不可用，不生成零积压结论。

原 staged/due/Outbox held 指标退役。后台“事务消息指标”链接对应 infra 的 `event-transactions` dashboard；使用自定义地址时配置 `REACT_APP_GRAFANA_EVENT_TRANSACTIONS_URL`。新查询没有增加表或修改冻结消息。
