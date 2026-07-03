# qs-server 文档中心

> 本文是 `docs/` 的根入口。
>
> 它只负责三件事：说明文档分层、给出稳定阅读入口、明确事实源优先级。
>
> 它不替代各目录 README，也不重复维护每个子模块的完整细节。

---

## 1. 30 秒结论

| 维度 | 结论 |
| --- | --- |
| 文档目标 | 先让读者知道从哪里进入，再进入具体目录 |
| 真值层 | `00-05` 是现行真值层，`06-宣讲` 是讲解层，`_archive` 是历史层 |
| 事实优先级 | 源码与机器契约优先于 prose 文档 |
| 业务模块入口 | 统一从 `02-业务模块/README.md` 进入 |
| 核心业务模块 | `survey / assessmentmodel / evaluation / report` |
| 执行主线 | Survey 提供答卷事实，Assessment Model 提供发布模型资产，Evaluation 执行测评并产出结果，Report 保存最终解读报告 |
| 事件入口 | 统一从 `03-基础设施/01-事件系统.md` 与 `03-基础设施/event/README.md` 进入 |
| Redis 入口 | 统一从 `03-基础设施/12-Redis文档中心.md` 进入 |
| Resilience 入口 | 限流、队列、背压、锁、幂等和降级统一从 `03-基础设施/resilience/README.md` 进入 |
| Security 入口 | JWT、IAM、authz snapshot、capability、service auth、mTLS/ACL 统一从 `03-基础设施/security/README.md` 进入 |
| Data Access 入口 | MySQL、Mongo、migration、read model 统一从 `03-基础设施/data-access/README.md` 进入 |
| 外部集成入口 | WeChat、OSS、通知适配统一从 `03-基础设施/integrations/README.md` 进入 |
| 横切能力矩阵 | 当前入口见 `03-基础设施/resilience/07-能力矩阵.md` |

---

## 2. 当前文档主线

qs-server 当前文档的主线是：

```text
00-总览        先理解系统地图、代码边界和核心链路
01-运行时      再理解 qs-apiserver / collection-server / qs-worker 如何协作
02-业务模块    再深入 survey / scale / assessmentmodel / evaluation / report 等业务模块
03-基础设施    再理解事件、存储、Redis、安全、韧性、外部集成等横切能力
04-接口与运维  再查看 REST / gRPC / 部署 / 运维入口
05-专题分析    最后理解关键设计判断和系统级权衡
06-宣讲        用于对外讲解、技术分享、面试表达
_archive       仅保留历史文档，不作为当前事实源
```

业务链路的核心表达是：

```text
Survey
    管问卷定义和答卷提交事实

Assessment Model
    管测评模型资产：Kind / Snapshot / Binding / Payload

Scale / MBTI / BigFive
    作为具体模型资产和 Evaluation 执行插件存在

Evaluation
    管一次测评执行：Assessment / Result / Retry / Events

Report
    管最终报告聚合和模型报告 adapter
```

当前已重建的重点文档是：

```text
survey              作答事实层
scale               医学量表解释模型
assessmentmodel     测评模型资产层
evaluation          通用测评执行层
report              最终报告聚合
```

---

## 3. 事实来源优先级

阅读或维护文档时，默认按下面的优先级判断真值：

```text
1. 源码
   internal/、cmd/、pkg/

2. 机器契约与配置
   api/rest/、proto/、configs/events.yaml、configs/*.yaml

3. docs/00-05 现行正文

4. docs/06-宣讲

5. docs/_archive
```

如果 prose 文档与代码、契约冲突，以代码和契约为准。

文档可以解释设计意图，但不能反向覆盖源码事实。

---

## 4. 根目录地图

| 目录 | 解决什么问题 | 什么时候进入 |
| --- | --- | --- |
| [00-总览](./00-总览/) | 系统地图、代码边界、主链路、本地开发入口 | 第一次读仓库时 |
| [01-运行时](./01-运行时/) | 三进程职责、调用方向、运行时协作 | 需要确认谁调谁、怎么跑时 |
| [02-业务模块](./02-业务模块/) | 业务模块职责、模型、链路、状态机和事实源 | 需要看领域设计和业务实现时 |
| [03-基础设施](./03-基础设施/) | 事件、存储、Redis、缓存限流、IAM、配置等横切能力 | 需要看机制、配置和代码挂载点时 |
| [04-接口与运维](./04-接口与运维/) | REST / gRPC 契约、端口部署、调度任务、事故复盘 | 需要看机器契约和运维入口时 |
| [05-专题分析](./05-专题分析/) | 设计判断、边界拆分、异步评估、保护层 | 需要看系统级设计权衡时 |
| [06-宣讲](./06-宣讲/) | 对外讲解、技术分享、答辩材料 | 需要把项目讲清楚时 |

---

## 5. 推荐阅读入口

### 5.1 第一次读仓库

推荐顺序：

```text
00-总览/README.md
00-总览/01-系统地图.md
00-总览/02-代码组织与边界.md
00-总览/03-核心业务链路.md
01-运行时/README.md
02-业务模块/README.md
```

目标是先建立：

```text
系统有哪些进程；
核心链路如何流转；
代码按什么边界组织；
业务模块之间如何协作；
哪些目录是事实源。
```

---

### 5.2 需要理解业务模块

从这里进入：

```text
02-业务模块/README.md
```

当前重点业务模块：

```text
survey/
scale/
evaluation/
report/
```

建议顺序：

```text
survey
    先理解问卷定义和答卷提交事实

scale
    再理解 MedicalScale / Factor / ScoringSpec / InterpretationRules

assessmentmodel
    再理解 Kind / Snapshot / Binding / Payload 抽象

evaluation
    再理解 Assessment / Model Execution / Result / Retry / Events

report
    最后理解 InterpretReport / personality adapter / score-based adapter
```

---

## 6. 核心业务模块入口

### 6.1 Survey：作答事实层

入口：

```text
02-业务模块/survey/README.md
```

Survey 负责：

```text
Questionnaire；
SubmissionSpec；
Question / Option；
AnswerSheet；
AnswerValue；
答卷提交链路；
答卷提交事件；
Outbox 出站。
```

Survey 不负责：

```text
医学量表计分；
MBTI 类型解析；
Assessment 状态机；
测评报告保存。
```

---

### 6.2 Scale：医学量表解释模型

入口：

```text
02-业务模块/scale/README.md
```

Scale 文档包括：

```text
01-Scale模型--MedicalScale-Factor-Interpretion 模型设计.md
02-Scale 维护链路--生命周期-因子维护-问卷绑定.md
03-Scale 查询链路--查询服务与读模型.md
04-Scale 测评链路--Scale与Evaluation联动详解.md
05-Scale模块分层架构与事实源索引.md
```

Scale 负责：

```text
MedicalScale；
Factor；
ScoringSpec；
InterpretationRules；
RiskLevel；
QuestionnaireRef；
ScaleChangedEvent。
```

Scale 不负责：

```text
AnswerSheet；
Assessment；
EvaluationRun；
FactorScore；
InterpretReport。
```

---

### 6.3 Assessment Model：测评模型资产层

入口：

```text
internal/apiserver/domain/assessmentmodel
```

Assessment Model 当前事实源包括：

```text
domain/assessmentmodel
domain/assessmentmodel/scale/definition
domain/assessmentmodel/scale/snapshot
domain/assessmentmodel/mbti
domain/assessmentmodel/sbti
port/assessmentmodel
```

Assessment Model 负责：

```text
Kind；
Snapshot；
QuestionnaireBinding；
DecisionKind；
PayloadFormat；
具体模型 payload；
发布模型目录端口。
```

Assessment Model 不负责：

```text
MedicalScale 内部规则；
MBTI 内部维度规则；
Assessment 状态机；
AnswerSheet 提交流程。
```

---

### 6.4 Evaluation：通用测评执行引擎

入口：

```text
02-业务模块/evaluation/README.md
```

Evaluation 文档包括：

```text
01-Evaluation模型--Assessment-EvaluationRun-Result-Report模型设计.md
02-Evaluation执行链路--从AnswerSheet提交到Assessment完成.md
03-Evaluation引擎链路--模型解析-规则加载-执行-报告生成.md
04-Evaluation失败重试链路--幂等-错误状态-补偿处理.md
05-Evaluation事件链路--答卷提交-测评完成-报告生成.md
06-Evaluation模块分层架构与事实源索引.md
```

Evaluation 负责：

```text
Assessment；
AssessmentStatus；
EvaluationRun；
EvaluationEngine；
EvaluationResult；
ScoreResult / InterpretationResult / ProfileResult；
FailureReason；
RetryPolicy；
AssessmentInterpretedEvent；
InterpretReportGeneratedEvent。
```

Evaluation 不负责：

```text
Questionnaire 定义；
AnswerSheet 提交事实；
MedicalScale 规则维护；
MBTI 规则维护；
InterpretReport 领域聚合。
```

---

### 6.5 Report：最终报告聚合

入口：

```text
internal/apiserver/domain/report
```

Report 负责：

```text
InterpretReport；
DimensionInterpret；
Suggestion；
ModelExtra；
personality adapter；
score-based adapter。
```

Report 不负责：

```text
AnswerSheet 提交流程；
Assessment 状态机；
模型资产 payload；
模型执行计算。
```

---

## 7. 业务链路阅读路线

### 7.1 从用户提交答卷开始

推荐阅读：

```text
02-业务模块/survey/03-答卷提交链路分析.md
02-业务模块/evaluation/02-Evaluation执行链路--从AnswerSheet提交到Assessment完成.md
02-业务模块/evaluation/05-Evaluation事件链路--答卷提交-测评完成-报告生成.md
```

要理解的问题：

```text
AnswerSheet 如何提交；
AnswerSheetSubmittedEvent 如何产生；
Worker 如何驱动 Evaluation；
Assessment 如何创建和完成；
报告如何生成；
事件如何通知下游。
```

---

### 7.2 从医学量表规则开始

推荐阅读：

```text
02-业务模块/scale/01-Scale模型--MedicalScale-Factor-Interpretion 模型设计.md
02-业务模块/scale/02-Scale 维护链路--生命周期-因子维护-问卷绑定.md
02-业务模块/scale/04-Scale 测评链路--Scale与Evaluation联动详解.md
02-业务模块/evaluation/03-Evaluation引擎链路--模型解析-规则加载-执行-报告生成.md
```

要理解的问题：

```text
MedicalScale 如何定义规则；
Factor 如何绑定题目；
ScoringSpec 如何表达计分；
InterpretationRules 如何表达解释；
Evaluation 如何消费 Scale 规则。
```

---

### 7.3 从多解释模型扩展开始

推荐阅读：

```text
internal/apiserver/domain/assessmentmodel
internal/apiserver/domain/assessmentmodel/mbti
internal/apiserver/domain/assessmentmodel/sbti
internal/apiserver/domain/evaluation/mbti
internal/apiserver/domain/evaluation/sbti
02-业务模块/evaluation/03-Evaluation引擎链路--模型解析-规则加载-执行-报告生成.md
```

要理解的问题：

```text
为什么 Scale 与 MBTI 同级；
为什么不能把 MBTI 塞进 Scale；
ModelRef / Snapshot / evaluator registry 如何抽象；
Evaluation 如何避免硬编码 scale / mbti；
新增模型需要改哪些代码和文档。
```

---

### 7.4 从失败重试和可靠性开始

推荐阅读：

```text
02-业务模块/evaluation/04-Evaluation失败重试链路--幂等-错误状态-补偿处理.md
02-业务模块/evaluation/05-Evaluation事件链路--答卷提交-测评完成-报告生成.md
03-基础设施/event/README.md
03-基础设施/event/02-Publish与Outbox.md
03-基础设施/event/03-Worker消费与AckNack.md
```

要理解的问题：

```text
重复事件如何幂等；
失败如何落库；
重试为什么不能使用 latest model；
结果已保存但报告失败如何补偿；
Outbox 如何保证事件可靠出站。
```

---

## 8. 基础设施入口

### 8.1 事件系统

推荐入口：

```text
03-基础设施/01-事件系统.md
03-基础设施/event/README.md
03-基础设施/event/02-Publish与Outbox.md
03-基础设施/event/03-Worker消费与AckNack.md
configs/events.yaml
```

重点问题：

```text
事件如何定义；
Outbox 如何落库；
Worker 如何消费；
Ack / Nack 如何处理；
事件如何与业务状态保持一致。
```

---

### 8.2 Redis

推荐入口：

```text
03-基础设施/12-Redis文档中心.md
03-基础设施/redis/README.md
03-基础设施/06-Redis使用情况.md
03-基础设施/13-Redis缓存业务清单.md
```

重点问题：

```text
哪些地方用了 Redis；
缓存 key 如何设计；
TTL 如何设置；
哪些缓存不是事实源；
缓存如何失效和重建。
```

---

### 8.3 Resilience Plane

推荐入口：

```text
03-基础设施/resilience/README.md
03-基础设施/resilience/00-整体架构.md
03-基础设施/resilience/05-观测降级与排障.md
03-基础设施/resilience/06-新增高并发治理能力SOP.md
03-基础设施/resilience/07-能力矩阵.md
```

重点问题：

```text
限流；
队列；
背压；
锁；
幂等；
降级；
观测；
排障。
```

---

### 8.4 Security Plane

推荐入口：

```text
03-基础设施/security/README.md
03-基础设施/security/00-整体架构.md
03-基础设施/security/01-Principal与OrgScope.md
03-基础设施/security/02-AuthzSnapshot与CapabilityDecision.md
03-基础设施/security/03-ServiceIdentity与mTLS-ACL.md
```

重点问题：

```text
Principal；
OrgScope；
AuthzSnapshot；
CapabilityDecision；
ServiceIdentity；
mTLS / ACL；
IAM 集成。
```

---

### 8.5 Data Access Plane

推荐入口：

```text
03-基础设施/data-access/README.md
03-基础设施/data-access/00-整体架构.md
03-基础设施/data-access/01-MySQL仓储与UnitOfWork.md
03-基础设施/data-access/02-Mongo文档仓储.md
03-基础设施/data-access/05-新增持久化能力SOP.md
```

重点问题：

```text
MySQL 仓储；
Mongo 文档仓储；
UnitOfWork；
Migration；
ReadModel；
持久化能力新增 SOP。
```

---

### 8.6 Integrations Plane

推荐入口：

```text
03-基础设施/integrations/README.md
03-基础设施/integrations/00-整体架构.md
03-基础设施/integrations/01-WeChat适配器.md
03-基础设施/integrations/02-ObjectStorage适配器.md
03-基础设施/integrations/04-新增外部集成SOP.md
```

重点问题：

```text
WeChat 适配；
ObjectStorage；
通知适配；
外部集成端口；
适配器边界。
```

---

## 9. 接口与运维入口

需要看接口契约：

```text
04-接口与运维/
api/rest/apiserver.yaml
api/rest/collection.yaml
api/grpc/gen
```

需要看运行部署：

```text
01-运行时/README.md
04-接口与运维/
cmd/qs-apiserver/apiserver.go
cmd/collection-server/main.go
cmd/qs-worker/main.go
```

需要看事件配置：

```text
configs/events.yaml
03-基础设施/event/README.md
```

---

## 10. 宣讲层入口

需要准备技术分享、面试表达或对外介绍时，从这里进入：

```text
06-宣讲/README.md
06-宣讲/09-30分钟技术分享脚本.md
06-宣讲/10-架构图素材索引.md
06-宣讲/11-面试追问证据索引.md
```

宣讲层回答：

```text
如何讲清楚项目；
如何组织 30 秒 / 3 分钟 / 30 分钟表达；
如何准备架构图；
如何准备追问证据。
```

宣讲层不承担机器契约和实现真值。

---

## 11. 真值层、宣讲层与历史层

### 11.1 现行真值层

```text
00-总览
01-运行时
02-业务模块
03-基础设施
04-接口与运维
05-专题分析
```

这些目录回答：

```text
代码现在是什么；
架构边界是什么；
模块如何协作；
运行时如何工作；
接口和配置如何定义。
```

---

### 11.2 宣讲层

```text
06-宣讲
```

这一层回答：

```text
怎么把项目讲清楚。
```

它可以引用真值层，但不替代真值层。

---

### 11.3 历史层

```text
_archive
```

这一层只保留历史背景。

不进入现行阅读主路径。

不作为当前事实来源。

---

## 12. archive 政策

`docs/_archive` 的定位是长期保留的历史层。

它不是：

```text
临时垃圾桶；
第二真值层；
现行文档备份目录。
```

使用规则：

```text
现行文档默认不依赖 _archive；
从 _archive 回迁内容前，必须重新核对源码和契约；
make docs-hygiene 默认不检查 _archive。
```

archive 的具体规则见：

```text
docs/_archive/README.md
```

---

## 13. 维护入口

文档写作规则：

```text
CONTRIBUTING-DOCS.md
```

提交前校验：

```bash
make docs-hygiene
```

根仓快速开始：

```text
../README.md
```

---

## 14. 代码与契约锚点

REST：

```text
api/rest/apiserver.yaml
api/rest/collection.yaml
```

gRPC Proto：

```text
api/grpc/gen
```

事件：

```text
configs/events.yaml
```

三进程入口：

```text
cmd/qs-apiserver/apiserver.go
cmd/collection-server/main.go
cmd/qs-worker/main.go
```

核心代码目录：

```text
internal/apiserver/domain/
internal/apiserver/application/
internal/apiserver/infra/
internal/apiserver/interface/
internal/worker/
```

---

## 15. 最终原则

维护 qs-server 文档时，始终坚持三条原则：

```text
第一，源码和机器契约优先于 prose 文档；
第二，根 README 只做入口和导航，不重复维护二级目录细节；
第三，业务模块文档必须服务于当前代码事实和下一阶段演进，而不是停留在历史设计稿。
```

当前业务模块文档的核心演进方向是：

```text
Survey 保持作答事实层；
Assessment Model 收敛模型资产和发布快照；
Scale / MBTI / SBTI 作为模型资产与执行插件存在；
Evaluation 保持通用测评执行层；
Report 保持最终报告聚合。
```
