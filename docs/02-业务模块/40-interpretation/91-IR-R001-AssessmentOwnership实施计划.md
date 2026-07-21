# IR-R001：Assessment Ownership 授权前置实施计划

> 状态：已实现待验收（2026-07-21）。编码与单测矩阵已完成；生产审计与压测证据仍缺。

## 目标

所有 `report-status` / `wait-report` 请求在读取 Redis 或进入 DB fallback 前，先证明：

```text
TesteeID + AssessmentID -> GetMyAssessment -> authorized
```

从而保证 cache hit、miss、unavailable 三条路径具有相同的 Assessment ownership 语义。

## 非目标

- 不把 Redis 改成报告状态事实源。
- 不调整 ProfileLink 的 User → Testee 授权。
- 不处理 Catalog 正文关联、Audience 或模板路由问题。
- 不新增普通状态快照携带的授权结论。

## 实现

1. `reportwait.Service.GetStatus` 和 `Wait` 在任何状态读取前调用统一的 `authorize`。
2. Authorizer 复用现有 `QueryService.GetMyAssessment(testeeID, assessmentID)`：
   - 查询报错：保持原错误语义；
   - 返回 nil：统一为 `ErrAssessmentAccess`；
   - 查询服务缺失：fail closed，不读取 cache。
3. 等待循环只在入口授权一次；后续轮询仍按原逻辑读取 cache / DB，避免每次 tick 重复授权。
4. behavior facade 已有机制类型校验，继续保留；WS Resolver 已有前置授权，允许暂时重复验证以避免旁路。
5. 观测：`report_status_assessment_ownership_total{result=allowed|denied|error|misconfigured}` + duration histogram。

## 行为边界

- 允许改变：未授权请求即使 Redis 命中，也必须失败。
- 保持不变：已授权请求的状态映射、轮询、signal、DB fallback、TTL 与错误透传。
- 安全不变量：授权失败时 cache `Get` 调用次数必须为 0。

## 测试

- `GetStatus` / `Wait`：foreign Assessment + Redis terminal hit → `ErrAssessmentAccess`，且不读取 cache。
- foreign × Redis miss / unavailable；query error / query nil → 均不读取 cache。
- own × Redis unavailable → 授权后走 DB fallback。
- 已授权 Redis hit 与原 DB fallback 测试保持通过。
- collection-server application 包集回归通过。

## 剩余验收

- 生产历史访问日志审计（跨 Testee 状态观察是否发生）。
- 高峰 ownership 查询延迟与 DB 压力；必要时另建受控 ownership projection。
- REST / WebSocket 端到端联调抽样（单测已覆盖共享服务契约）。
