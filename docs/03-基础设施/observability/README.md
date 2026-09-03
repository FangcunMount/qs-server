# Observability

Observability 分成“瞬时信号”和“可签署证据”两层。日志、指标与 request/event ID 用来定位；probe、runtime snapshot、checkpoint、Outbox 和 dated evidence 用来说明某个时点实际成立了什么。
任何单一平面都不能证明端到端正确。

## 阅读顺序

1. [日志、指标与关联标识](./10-日志、指标与关联标识.md)：日志、Prometheus、关联 ID、label 基数和诊断链。
2. [健康探针、治理与持久证据](./20-健康探针、治理与持久证据.md)：`observability/probes` canonical 文档。
3. [核心设计决策与替代方案](./15-核心设计决策与替代方案.md)：为什么分开 liveness、readiness、deep check 和 durable evidence。

## 当前边界

- apiserver `/readyz` 只检查 Redis runtime snapshot，并在 status service 缺失/报错时回退 Ready；不能证明 MySQL、Mongo、IAM、MQ 或业务主链。
- collection readiness 还包括 resilience control 初次同步；worker readiness 主要表达已配置 Redis family。
- worker 9092 无应用层认证，网络验收见 [Security](../security/20-敏感信息、网络暴露与验收.md)。
- 历史主机数、容器数、扫描数量和某次探针结果不得留在稳定文档，统一进入[基础设施生产证据台账](../../00-总览/10-基础设施生产证据台账.md)。

## 验证入口

```bash
go test -count=1 ./internal/apiserver/transport/rest ./internal/collection-server/transport/rest/handler ./internal/worker/observability ./internal/pkg/redisruntime/observability
```

真实环境还需逐实例探针、Prometheus 抓取、网络正反向、依赖故障注入与 durable state 查询；本地 handler 绿色不能替代这些证据。
