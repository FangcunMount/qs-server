# Report 短轮询查询链路

## 1. 解决什么问题

短轮询解决异步报告生成期间客户端如何获取状态的问题。它简单稳定，但如果没有退避和缓存，会形成查询风暴。

## 2. 所在位置

短轮询位于小程序 / 前端和 collection-server report status 接口之间，后端优先读取 report status cache。

## 3. 设计目标

客户端按 `next_poll_after_ms` 查询；服务端优先查 Redis 状态；未完成时避免频繁访问 DB；失败和超时状态明确。

## 4. 整体流程

客户端提交后查询 report status；若 completed 则读取报告；若 processing / queued 则返回状态和下一次查询建议；若 failed 返回失败原因。

## 5. 核心数据结构

report_id、assessment_id、status、message、next_poll_after_ms、report_url 或 report payload、updated_at。

## 6. 正常流程

报告未完成时返回 pending 状态和退避时间；报告完成后返回 completed 并引导读取报告。

## 7. 异常流程

缓存 miss 时受控回源；状态未知时返回 pending 或明确错误；客户端超时按退避继续查询，不能固定高频刷新。

## 8. 幂等 / 降级 / 背压

查询接口幂等；服务端用缓存和 `next_poll_after_ms` 降低频率；客户端需要指数退避或遵守服务端建议。

## 9. 可选方案

无退避短轮询最简单但请求量大；长轮询能减少无效请求；WebSocket 实时性更好但连接管理复杂。

## 10. 当前方案取舍

短轮询作为基础兼容方案保留，必须配合 report status cache 和 `next_poll_after_ms`。

## 11. 观测指标

poll QPS、cache hit rate、pending response count、completed latency、next_poll_after_ms distribution、DB fallback count。

## 12. 代码事实源

- [../../../internal/collection-server/application/reportwait](../../../internal/collection-server/application/reportwait)
- [../../../internal/pkg/reportstatus](../../../internal/pkg/reportstatus)
- [../../04-接口与运维/12-小程序报告等待接入指南.md](../../04-接口与运维/12-小程序报告等待接入指南.md)
