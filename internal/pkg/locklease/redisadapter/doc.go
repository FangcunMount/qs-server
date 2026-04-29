// Package redisadapter 提供 qs-server 基于 Redis 的 locklease adapter。
//
// 本包不是业务幂等框架或调度框架，只负责通过 cacheplane.Handle 构造锁 key、
// 执行 token-based acquire/release，并上报 lock/family 观测结果。
//
// 调用方保留各自的领域语义：
//   - apiserver scheduler 使用 leader lock，竞争失败时跳过本轮任务。
//   - collection-server submit guard 组合 in-flight lock 和 done marker，保护提交幂等。
//   - worker event handler 使用 best-effort duplicate suppression，锁路径降级时继续处理。
//
// 本包不会自动续租。长耗时调用方必须选择能覆盖 critical section 的 TTL，或单独引入续租设计。
// 释放 lease 时必须校验 token；错误 token 不能释放其他 owner 持有的锁。
package redisadapter
