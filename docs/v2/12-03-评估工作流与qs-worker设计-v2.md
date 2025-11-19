# 12-03 评估工作流与 qs-worker 设计（V2）

> 版本：V2.0
> 目标：描述“提交答卷 → 异步评估 → 生成报告”的端到端工作流，以及 qs-worker 的职责。

---

## 1. 总体思路

* **同步提交，异步评估**：

  * 提交答卷时只做校验 & 保存 AnswerSheet、创建 Assessment、发送事件；
  * 不在 HTTP 请求内执行评估。
* **qs-worker 专职评估**：

  * 消费 AssessmentSubmittedEvent；
  * 调用 scale.Evaluator；
  * 写回 AssessmentScore、InterpretReport、Assessment 状态；
  * 发布 AssessmentInterpretedEvent。

---

## 2. Assessment 生命周期

状态：

```go
const (
    StatusPending     Status = "pending"
    StatusSubmitted   Status = "submitted"
    StatusInterpreted Status = "interpreted"
    StatusFailed      Status = "failed"
)
```

流转：

1. 初始化：pending（可选）；
2. 提交答卷：submitted；
3. 评估成功：interpreted；
4. 评估失败：failed。

---

## 3. 领域事件

```go
type AssessmentSubmittedEvent struct {
    AssessmentID AssessmentID
    TesteeID     user.TesteeID
    OccurredAt   time.Time
}

type AssessmentInterpretedEvent struct {
    AssessmentID AssessmentID
    TesteeID     user.TesteeID
    OccurredAt   time.Time
}
```

* SubmittedEvent：由 qs-apiserver 发布，qs-worker 消费；
* InterpretedEvent：由 qs-worker 发布，通知/统计服务消费。

---

## 4. 消息中间件设计

推荐：

* Topic：`assessment_events`
* Channel：

  * `evaluation`：评估（qs-worker）
  * `notification`：通知（可选）

消息内容包含：

* event_type（submitted/interpreted）
* assessment_id
* testee_id
* occurred_at

---

## 5. qs-apiserver 流程

提交答卷用例：

1. 加载 Questionnaire；
2. 构造 AnswerSheet，填充答案；
3. 使用 AnswerSheetValidator 做领域校验；
4. 提交 & 保存 AnswerSheet；
5. 获取/创建 Testee；
6. 创建 Assessment（Status=submitted）；
7. 保存 Assessment；
8. 发布 AssessmentSubmittedEvent；
9. 返回 AssessmentID 给前端。

---

## 6. qs-worker 设计与实现

### 6.1 角色

* 工作者进程，监听 MQ；
* 核心组件：

  * Consumer：绑定到 Topic/Channel；
  * Handler：处理 AssessmentSubmittedEvent；

### 6.2 处理逻辑

1. 加载 Assessment；
2. 若没有 MedicalScaleID（问卷模式）：直接返回；
3. 加载 Questionnaire / AnswerSheet / MedicalScale；
4. 调用 Evaluator.Evaluate()；
5. 更新 Assessment（总分、风险、状态等）；
6. 写入 AssessmentScore；
7. 通过 ReportFactory 生成 InterpretReport；
8. 保存 InterpretReport；
9. 发布 AssessmentInterpretedEvent。

---

## 7. 前端交互

* 提交成功后拿到 AssessmentID；
* 前端进入“解析中”页面；
* 使用短轮询 `GET /assessments/{id}`：

  * `status=submitted` → 返回解析中；
  * `status=interpreted` → 返回报告；
  * `status=failed` → 返回错误信息。

高并发场景下的查询策略详见 12-04 文档。

---

## 8. 小结

* 提交路径保持轻量；
* 评估任务通过 qs-worker 异步执行；
* 利用 MQ 做缓冲和解耦；
* 通过 Assessment 状态 + 主键查询保证高并发下的可用性和可维护性。
