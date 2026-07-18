# Plan

Plan 管测评计划、任务生成、任务生命周期与调度；它不执行测评算法，也不拥有答卷或报告。

## 1. 领域模型

`internal/apiserver/domain/plan` 的核心对象包括：

- `AssessmentPlan`：计划定义与状态；
- `AssessmentTask`：面向受试者的具体任务；
- `PlanLifecycle`、`TaskLifecycle`：状态迁移规则；
- `PlanEnrollment`：计划与受试者纳入；
- `TaskGenerator`：依据计划生成任务；
- `PlanValidator`：跨字段业务校验。

计划状态、调度类型和任务状态以 `domain/plan/types.go` 为准。

## 2. 应用服务

`internal/apiserver/application/plan` 包含 command/lifecycle、enrollment、query、task scheduler、assessment resolver、notification context 等用例。application service 负责 repository、事务、事件发布和外部 port；状态转换仍由领域对象决定。

## 3. 关键路径

```text
CreatePlan
  -> 校验计划与模型/问卷引用
  -> 保存 AssessmentPlan
  -> Enrollment 生成或对账 AssessmentTask
  -> Scheduler 打开/过期任务
  -> 生成 AssessmentEntry 或解析任务测评上下文
  -> 发布 task.* 事件
```

任务事件当前为 best-effort 通知，清单见 `configs/events.yaml`。业务状态不能依赖“消息一定到达”。

## 4. 边界

- ModelCatalog/Survey 提供可用内容引用，Plan 不复制其定义。
- Actor 提供受试者与访问范围。
- Evaluation/Survey 消费任务上下文，但 Plan 不推进其状态机。
- Statistics 读取任务事实形成完成率等读模型。

## 5. 证据与验证

- domain：`internal/apiserver/domain/plan`。
- application：`internal/apiserver/application/plan`。
- 装配：`internal/apiserver/container/modules/plan`。
- 验证：plan domain/application/container 以及 scheduler/enrollment 定向测试。

状态：`已实现`（本轮核对到聚合、服务与主链；精确状态转移表待补证据）。
