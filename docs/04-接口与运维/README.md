# 接口与运维

本目录只保留可以直接执行的契约核验、服务接入和运行操作。稳定的架构、配置、网络、探针、安全、事件、缓存与并发语义由 [`docs/03-基础设施`](../03-基础设施/README.md) 维护；这里不再复制完整字段表、端口矩阵或某次生产结果。

## 1. 执行入口

| 任务 | Runbook | 完成标准 |
| --- | --- | --- |
| 核验机器契约 | [接口契约总览](./00-接口契约总览.md) | 生成结果无漂移，定向测试通过，命令与 checkout SHA 已记录 |
| 接入 apiserver REST | [apiserver REST](./01-apiserver-REST.md) | OpenAPI、路由、安全正反向请求与调用方适配通过 |
| 接入 collection REST | [collection REST](./02-collection-REST.md) | OpenAPI、路由、提交/查询回退与调用方适配通过 |
| 变更 gRPC | [gRPC 契约](./03-gRPC契约.md) | proto 生成无漂移，server/client/ACL 定向测试通过 |
| 联调 internal gRPC | [internal gRPC](./04-internal-gRPC.md) | mTLS、方法 ACL、委托主体与资源授权正反向验证通过 |
| 检查配置 | [配置与环境变量](./05-配置与环境变量.md) | 静态契约、部署包、脱敏 effective config 与外部文件均已核对 |
| 部署与端口验收 | [部署与端口](./06-部署与端口.md) | exact SHA/digest、listener、正反向网络与逐实例 probe 有证据 |
| 操作 scheduler | [调度任务](./07-调度任务.md) | runner 清单、开关、leader lock、checkpoint 与停止条件已核对 |
| 验收 probe/metrics | [健康检查与观测](./08-健康检查与观测.md) | 逐实例语义、允许访问和禁止访问均已验证 |
| 故障定位 | [常见排障](./09-常见排障.md) | 已沿 request/event/business fact 定位失败层，未执行越权修复 |

## 2. 专项执行指南

- [容量档位与资源配置建议](./10-QPS容量档位与资源配置建议.md)
- [300QPS 混合场景压测 SOP](./11-300QPS混合场景压测SOP.md)
- [小程序报告等待接入](./12-小程序报告等待接入指南.md)
- [小程序接入文档](./15-小程序接入文档.md)
- [测评后台接入文档](./16-测评后台接入文档.md)

接口字段与枚举以 `api/rest/*.yaml`、`api/grpc/proto` 和 `configs/events.yaml` / `configs/signals.yaml` 为准；专项指南与机器契约冲突时，先修调用或更新契约，不能在本文层增加另一份真值。

## 3. 每次执行都要保存的证据

1. `git rev-parse HEAD` 与工作区是否有未提交改动；
2. 实际执行的完整命令、开始/结束时间、退出码和结果摘要；
3. 环境、镜像 digest、脱敏 effective-config hash；
4. 正向成功与反向拒绝两类证据；
5. 限制、owner、失效时间和后续动作。

仓库门禁结果只证明当前 checkout；环境或生产执行结果统一写入[基础设施生产证据台账](../00-总览/10-基础设施生产证据台账.md)，不得把主机数、副本数或一次探针结果回写为本目录的长期事实。
