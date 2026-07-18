# qs-server

`qs-server` 是面向心理、医学与人格测评场景的 Go 后端。系统把作答事实、测评模型资产、执行结果和报告成品拆成独立边界，并通过三进程协作完成前台接入、主业务处理与异步执行。

## 1. 30 秒认识系统

```text
客户端
  -> collection-server：前台 BFF、身份投影、限流与提交保护
  -> qs-apiserver：业务事实、领域用例、REST/gRPC、持久化与 Outbox
  -> qs-worker：消费 MQ，通过 internal gRPC 驱动异步评估与报告流程
```

核心业务链路是：

```text
Questionnaire / AnswerSheet
  -> Published Assessment Model
  -> Evaluation Outcome
  -> Interpretation Report
```

同步请求保存作答事实；后续评估与报告生成通过可靠事件异步推进。当前事件名称、投递语义和 Topic 映射以 [`configs/events.yaml`](./configs/events.yaml) 为准。

## 2. 当前业务边界

业务模块注册事实以 [`internal/apiserver/container/modules/registry.go`](./internal/apiserver/container/modules/registry.go) 为准：

| 模块 | 责任 |
| --- | --- |
| `survey` | 问卷定义、作答契约、答卷事实 |
| `modelcatalog` | 测评模型资产、DefinitionV2、绑定与发布运行时快照 |
| `evaluation` | Assessment、执行路由、运行尝试与 Outcome |
| `interpretation` | 从冻结 Outcome 生成、存储和查询报告 |
| `actor` | 受试者、医生、操作员、测评入口与访问上下文 |
| `plan` | 测评计划、任务生命周期与调度 |
| `statistics` | 行为投影、读模型、统计查询与重建 |

`platform` 与 `iam` 是组合根中的集成层，不属于上述业务主链。

## 3. 文档入口

从 [文档中心](./docs/README.md) 开始。推荐阅读：

1. [系统地图](./docs/00-总览/01-系统地图.md)
2. [核心业务链路](./docs/00-总览/03-核心业务链路.md)
3. [三进程协作](./docs/01-运行时/00-三进程协作总览.md)
4. [业务模块地图](./docs/02-业务模块/README.md)
5. [接口与运维入口](./docs/04-接口与运维/README.md)

现行文档是经过筛选的 truth layer。重建前的旧树位于 `docs/_archive/`，只用于历史检索，不能作为当前实现依据。

## 4. 事实来源

发生冲突时按下面顺序判断：

1. 源码和运行时行为；
2. `api/`、`configs/`、migration 等机器可读契约；
3. `docs/00-05` 现行文档；
4. `docs/06-宣讲` 派生材料；
5. `docs/_archive` 历史快照。

## 5. 常用校验

```bash
make docs-hygiene
make docs-facts
git diff --check
```

涉及 REST 契约生成与对比时执行：

```bash
make docs-verify
```

完整工程质量门禁见 `Makefile` 中的 `verify` 目标。
