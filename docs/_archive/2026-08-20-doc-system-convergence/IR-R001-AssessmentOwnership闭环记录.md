# IR-R001：Assessment Ownership 授权前置闭环记录

> 状态：**已关闭**。本文件同步 2026-08-06 Interpretation 总表结论；原“已实现待验收”计划已经结束，不再作为开放任务或版本阻断项。

## 1. 关闭结论

medical、typology、behavior 的 `report-status` / `wait-report` 以及 WebSocket 链路，在读取报告状态 cache、进入 DB fallback 或注册 notifier 前，必须先证明两层关系：

```text
IAM User -> active ProfileLink -> Testee
TesteeID + AssessmentID -> AuthorizeAssessment -> allowed
```

当前实现已把 Assessment ownership 固化为 `reportwait.Service.GetStatus` 与 `Wait` 的前置步骤；cache hit、miss、unavailable 不再改变授权顺序。

## 2. 当前实现

1. REST/WS 的 User → Testee 边界由 `testeeaccess.Authorizer` 统一处理；依赖缺失、关闭或查询失败不会降级为放行。
2. `reportwait.Service.authorize` 在任何状态读取前调用独立 `AuthorizeAssessment` gRPC；`NotFound` 与 `PermissionDenied` 收敛为 `ErrAssessmentAccess`。
3. 授权拒绝时不调用 status cache，也不会注册 notifier；查询服务缺失时 fail closed。
4. 授权通过后，terminal cache 可以直接返回；非终态 cache 会按刷新窗口与持久 read model 对账。
5. Proto、collection client method 清单与生产 default-deny ACL 都包含 `AuthorizeAssessment`。

## 3. 观测与回归防线

- `report_status_testee_access_total{result}`、`report_status_testee_access_duration_seconds`：User → Testee；
- `report_status_assessment_ownership_total{result}`、`report_status_assessment_ownership_duration_seconds`：Testee → Assessment；
- `report_events_subscribe_denied_total{reason}`：WebSocket 固定低基数拒绝分类；
- `service_test.go` 覆盖 ownership 拒绝时 cache `Get` 为 0、cache hit/miss/unavailable、依赖错误和 DB fallback；
- gRPC ACL identity matrix 防止新增/移动 RPC 后遗漏生产身份授权。

## 4. 当前证据入口

| 责任 | 事实源 |
| --- | --- |
| cache/DB 前置 ownership | `internal/collection-server/application/reportwait/service.go` |
| ownership 回归测试 | `internal/collection-server/application/reportwait/service_test.go` |
| User → Testee 授权 | `internal/collection-server/application/testeeaccess/authorizer.go` |
| collection gRPC client | `internal/collection-server/infra/grpcclient/evaluation_client.go` |
| Proto 契约 | `api/grpc/proto/evaluation/evaluation.proto` |
| 生产 ACL | `configs/grpc-acl.prod.yaml` |
| ownership 指标 | `internal/pkg/reportstatus/metrics.go` |
| 当前模块状态 | [Interpretation 设计问题与重构清单](./90-设计问题与重构清单.md) |

## 5. 历史计划边界

2026-07-23 的实施记录曾把预发联调、容量对比与发布观察列为剩余验收；那是当日快照，不再覆盖总表的“已发布 / 已关闭”结论。若 ownership RPC、cache 顺序、ACL 或客户端调用链以后发生实质变化，应新建具名风险项和新的验收证据，不得把本闭环记录静默改回开放计划。
