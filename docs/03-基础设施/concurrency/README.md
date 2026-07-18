# Concurrency / Resilience

本模块把突发流量、重复提交、下游过载、长耗时租约和治理恢复视为不同问题。

## 1. 保护链

| 机制 | 解决问题 | 典型失败语义 |
| --- | --- | --- |
| RateLimit | 单位时间入口过载 | 快速拒绝，客户端稍后重试 |
| SubmitQueue | 短时突发削峰 | 排队超时或队列已满 |
| SubmitGuard | 同一业务请求并发重复 | 合并、抑制或返回既有请求状态 |
| Backpressure | 下游资源饱和 | 有界拒绝，不继续放大压力 |
| LockLease | 长任务互斥与续租 | 失租后停止持有者动作，避免双写 |
| Resilience control | 运行时治理与恢复 | 操作审计、幂等重试、备用记录回填 |

## 2. 当前事实源

- collection 保护链：`internal/collection-server` 与 collection 配置。
- resilience capability：`internal/pkg/resilienceplane` 及 component-base 集成。
- 治理用例：`internal/apiserver/application/systemgovernance`。
- 组合与恢复 runner：`internal/apiserver/container/modules/platform`。
- 架构护栏：`internal/pkg/architecture/resilience_ownership_test.go` 等。

## 3. 报告查询治理

普通短轮询、服务端等待和 WebSocket 是不同交互策略。无论使用哪一种，最终都必须回查报告状态事实；Redis signal 只负责唤醒。

## 4. 待补证据

状态：`待补证据`。旧细分页已归档；下一轮应以当前 component-base 版本、操作审计恢复实现、具体配置和压测结果重写容量与控制面深度文档。
