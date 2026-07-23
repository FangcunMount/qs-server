# IR-R001：Assessment Ownership 授权前置实施计划

> 状态：已实现待验收（2026-07-23）。本地代码与契约测试已完成；预发联调、容量对比和发布观察仍缺。

## 目标

所有 medical、typology、behavior 的 `report-status` / `wait-report` 与 WebSocket 请求，在读取 Redis、订阅 notifier 或进入 DB fallback 前，先证明：

```text
IAM User -> active ProfileLink -> Testee
  -> TesteeID + AssessmentID -> GetMyAssessment -> authorized
```

从而保证 cache hit、miss、unavailable 三条路径具有相同的 Assessment ownership 语义。

## 非目标

- 不把 Redis 改成报告状态事实源。
- 不调整 ProfileLink、Testee 或 Assessment 的数据模型。
- 不处理 Catalog 正文关联、Audience 或模板路由问题。
- 不新增普通状态快照携带的授权结论。

## 实现

1. 共享 `testeeaccess.Authorizer` 统一检查 Testee、IAM Profile 和 active ProfileLink；依赖缺失/关闭返回 `ErrAccessUnavailable`，其他拒绝统一返回 `ErrAccessDenied`。
2. medical、typology、behavior 的报告 REST 中间件和 WebSocket 共用该 Authorizer；AnswerSheet legacy canonical-ID 逻辑不变。
3. `reportwait.Service.GetStatus` 和 `Wait` 在任何状态读取前调用 Assessment ownership 授权：
   - gRPC `NotFound`、`PermissionDenied` 或 nil：统一为 `ErrAssessmentAccess`；
   - 其他查询错误：保持依赖故障语义；
   - 查询服务缺失：fail closed，不读取 cache。
4. 首次授权返回已读取的 Assessment，并只在紧接着的第一次 DB fallback 复用；后续等待循环继续读取最新状态。
5. WebSocket 在升级前要求 JWT UserID，首次订阅和终态 signal 再读取均重新执行完整授权；授权完成前不注册 notifier。
6. 观测：
   - `report_status_testee_access_total{result=allowed|denied|error|misconfigured}`；
   - `report_status_testee_access_duration_seconds`；
   - `report_events_subscribe_denied_total{reason}` 仅使用固定分类。

## 行为边界

- 允许改变：未授权请求即使 Redis 命中，也必须失败。
- 保持不变：已授权请求的状态映射、轮询、signal、DB fallback、TTL、WS 请求与成功帧。
- 安全不变量：授权失败时 cache `Get` 调用次数必须为 0。
- 安全不变量：授权失败时 notifier subscription 必须为 0，WS 错误不得包含底层错误文本。

## 测试

- `testeeaccess.Authorizer` 表驱动覆盖依赖缺失/关闭、Testee 不存在、无 Profile、ProfileLink false/error 与合法关联。
- `GetStatus` / `Wait` 覆盖 foreign Assessment + Redis hit/miss/unavailable；`NotFound` 与 `PermissionDenied` 返回同一 `ErrAssessmentAccess`，且不读取 cache。
- 首次 Redis miss 的 `GetMyAssessment` 调用次数为 1。
- 三种机制的 status/wait handler 覆盖相同 403、404、503 契约，foreign 与 nonexistent 响应体一致。
- 真实 WebSocket 连接覆盖无 JWT、Testee/Assessment 越权、三种合法 kind、依赖故障、拒绝前零订阅和 ProfileLink 中途撤销。

## 剩余验收

- 预发验证合法订阅、越权负例与 HTTP fallback。
- 保存改造前后 observability snapshot，执行 `perf-mixed280-models` 与 `perf-special-report-short-poll`，验证既有阈值及新增授权 p95/p99。
- 小流量发布后至少观察 24 小时授权延迟、拒绝分类、依赖错误、连接池和 5xx。
- 历史日志若未同时记录 JWT User 与订阅 Testee，标记为不可追溯，不据此宣称历史无越权。
