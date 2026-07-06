# Report WebSocket 推送链路

## 1. 解决什么问题

WebSocket / SSE 解决报告状态实时推送问题，理论上可以进一步降低客户端轮询压力。

## 2. 所在位置

WebSocket / SSE 位于客户端和 collection-server report event 接入之间，worker 完成报告后通过状态变更事件或信令触发推送。

## 3. 设计目标

支持客户端订阅 report_id / assessment_id；连接鉴权；断线重连；推送终态；失败后可回退到短轮询。

## 4. 整体流程

客户端建立连接并订阅报告；服务端校验身份和访问范围；报告状态变更后推送消息；客户端收到 completed 后拉取报告。

## 5. 核心数据结构

connection_id、principal、subscription、report_id、assessment_id、last_event_id、heartbeat、push message。

## 6. 正常流程

连接建立后服务端维持订阅关系；报告完成后推送状态；客户端确认或断开连接。

## 7. 异常流程

连接断开后客户端用 report status 查询补偿；推送失败不影响业务事实；服务端重启后订阅丢失，需要客户端重连。

## 8. 幂等 / 降级 / 背压

推送消息可以重复；客户端按 report status 幂等处理；服务端限制连接数、订阅数和发送缓冲；高压下退回短轮询。

## 9. 可选方案

短轮询稳定但请求多；长轮询兼容 HTTP；WebSocket 体验最好但连接治理和运维成本最高。

## 10. 当前方案取舍

WebSocket / SSE 是按配置启用或规划增强的推送能力，不能写成所有环境默认能力。默认兜底仍是 report status + `next_poll_after_ms`。

## 11. 观测指标

active connections、subscriptions、push success / failed、disconnect count、heartbeat timeout、fallback to polling、per-connection buffer usage。

## 12. 代码事实源

- [../../04-接口与运维/12-小程序报告等待接入指南.md](../../04-接口与运维/12-小程序报告等待接入指南.md)
- [../../../configs/signals.yaml](../../../configs/signals.yaml)
- [../../../internal/collection-server/application/reportwait](../../../internal/collection-server/application/reportwait)
