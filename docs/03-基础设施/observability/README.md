# Observability

## 1. 结论

日志、指标、状态页和审计围绕业务链路与控制面组织，不按库函数罗列。

## 2. 最低观测维度

- 请求：request ID、进程、路由、租户、结果与耗时；
- 异步：event ID/type、topic、attempt、Ack/Nack、最终结果；
- 业务：AnswerSheet、Evaluation、Outcome、Report 的关联标识与阶段；
- 保护：限流、队列深度、防重命中、背压、租约丢失；
- 治理：操作人/请求、目标、幂等键、执行结果、恢复状态；
- 投影：checkpoint、pending、lag、reconcile 结果。

敏感身份、token、答案和报告正文不得直接写入日志。

## 3. 证据

metrics/logging 配置、health/ready/governance handlers、application instrumentation 和操作审计存储。
