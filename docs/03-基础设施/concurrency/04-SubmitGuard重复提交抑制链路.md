# SubmitGuard 重复提交抑制链路

## 1. 解决什么问题

SubmitGuard 解决用户重复点击、客户端超时重试、网络抖动和并发请求导致同一份答卷被多次提交的问题。

## 2. 所在位置

SubmitGuard 位于 collection-server 提交链路中，通常在真正写入 AnswerSheet / Assessment 前检查同一业务动作是否已经提交或正在提交。

## 3. 设计目标

识别同一 testee、questionnaire、assessment task 或 request_id 对应的业务动作；防止重复写入；允许已完成状态复用；失败后可释放或过期。

## 4. 整体流程

请求进入提交链路后构造 guard key；Redis / LockLease 尝试占位；占位成功继续提交；占位失败则返回已提交、正在提交或重复请求结果。

## 5. 核心数据结构

guard key、request_id、user_id、testee_id、assessment_task_id、questionnaire_id、status、ttl、lock token。

## 6. 正常流程

首次提交获取 guard key，写入业务事实并释放或转为完成状态。后续重复请求读取已有状态或被识别为同一业务动作。

## 7. 异常流程

提交失败时 guard key 按失败策略释放或短 TTL 过期；客户端重试如果使用相同 request_id，可复用状态；Redis 不可用时按配置降级，避免生成重复业务事实。

## 8. 幂等 / 降级 / 背压

SubmitGuard 解决业务幂等，不解决总体流量。它需要和 SubmitQueue 配合：队列削峰，guard 防止同一业务动作重复执行。

## 9. 可选方案

只靠限流不能识别同一业务动作；只靠数据库唯一索引能挡重复写入，但可能已经触发重复校验、重复事件或重复计算；只靠前端禁用按钮不可靠。

## 10. 当前方案取舍

采用 Redis guard key + LockLease + 业务状态复用。它在写入前抑制重复动作，并用 TTL 避免锁永久残留。

## 11. 观测指标

guard acquire success、guard conflict、duplicate submit count、status reused count、guard release failed、lock ttl expired、duplicate business key topN。

## 12. 代码事实源

- [../../../internal/collection-server/infra/redisops/submit_guard.go](../../../internal/collection-server/infra/redisops/submit_guard.go)
- [../../../internal/pkg/locklease](../../../internal/pkg/locklease)
- [../../../internal/pkg/locklease/redisadapter](../../../internal/pkg/locklease/redisadapter)
- [../../../internal/collection-server/application/answersheet/submission_service.go](../../../internal/collection-server/application/answersheet/submission_service.go)
