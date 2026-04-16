# Seeddata 指南

这份指南解释 `/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata` 里的 seeddata 工具现在到底是怎么工作的、每个 step 是干什么的、每个主要脚本负责什么，以及你在什么场景下应该怎么用。

## 1. 先建立心智模型

当前 seeddata 已经不是“随便往几张表里塞点测试数据”的工具，而是围绕最新业务模型组织的：

- **资源侧数据**
  - staff
  - clinician
  - testee 分配关系
  - assessment entry
- **行为足迹**
  - `footprint.entry_opened`
  - `footprint.intake_confirmed`
  - `footprint.testee_profile_created`
  - `footprint.care_relationship_established`
  - `footprint.care_relationship_transferred`
  - `footprint.answersheet_submitted`
  - `footprint.assessment_created`
  - `footprint.report_generated`
- **测评服务过程**
  - `assessment_episode`
- **派生分析视图**
  - `analytics_projection_org_daily`
  - `analytics_projection_clinician_daily`
  - `analytics_projection_entry_daily`

所以现在的 seeddata 有一个核心原则：

**优先走真实业务接口生成真实业务事实。**

只有少数“历史时间修正”或“统计重建”步骤会直接连本地数据库做离线修补。

## 2. 这个工具里“step”和“脚本文件”是什么关系

你运行 seeddata 时，真正选择的是 `--steps`。  
入口在 [main.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/main.go)。

`main.go` 里当前注册的 step 有：

- `staff`
- `clinician`
- `assign_testees`
- `testee_fixup_created_at`
- `actor_fixup_timestamps`
- `assessment_entries`
- `assessment_entry_flow`
- `assessment_by_entry`
- `daily_simulation`
- `assessment`
- `plan`
- `plan_create_tasks`
- `plan_process_tasks`
- `plan_fixup_timestamps`
- `statistics_backfill`

源码文件和 step 不是一一对应的，但大多数 step 都有自己的主文件。

## 3. 先看最重要的几个 step

如果你只需要记住最常用的一组，先记这 6 个：

1. `staff,clinician`
   - 建员工和临床医师
2. `assign_testees`
   - 把现有 testee 分配给 clinician
3. `assessment_entries`
   - 给 clinician 创建入口
4. `assessment_entry_flow`
   - 生成“入口打开 + intake”行为足迹
5. `assessment_by_entry`
   - 从入口结果继续推进到真实测评与报告
6. `statistics_backfill`
   - 按最新模型重建统计投影

如果你只想补“测评服务过程”，一般重点是：

```bash
assessment_entry_flow -> assessment_by_entry -> statistics_backfill
```

## 4. 每个 step 是干什么的

### `staff`

用途：
- 创建、复用或更新员工账号

典型场景：
- 新环境初始化
- 给 clinician 绑定 operator 前，先确保 staff 存在

依赖：
- `apiserver`
- IAM 或有效 API token

实现入口：
- [seed_actor.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_actor.go)

特点：
- 按配置幂等执行
- 已存在 staff 会做 drift 修正，而不是盲目新建

### `clinician`

用途：
- 创建、复用或更新临床医师

典型场景：
- 初始化临床用户池
- 批量生成演示 clinician

依赖：
- `apiserver`
- 如果使用 `operatorRef`，则依赖 `staff`

实现入口：
- [seed_actor.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_actor.go)
- 批量生成逻辑在 [seed_clinician_generator.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_clinician_generator.go)

特点：
- 可引用 staff
- 可按 generator 批量生成 clinician + staff

### `assign_testees`

用途：
- 给现有 testee 建 clinician 关系

典型场景：
- 你已经有一批 testee，但还没有分配给医生
- 你想构造 round-robin 或 random 的分配池

依赖：
- `apiserver`

实现入口：
- [seed_testee_assignment.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_testee_assignment.go)

支持策略：
- `explicit`
- `round_robin`
- `random`

注意：
- 这里只建关系，不创建 testee

### `testee_fixup_created_at`

用途：
- 把已有 testee 的 `created_at` 按长时间区间重新分布

典型场景：
- 你的测试数据都挤在近期，想做“多年沉淀”的假历史

依赖：
- 本地 MySQL

实现入口：
- [seed_testee_fixup.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_testee_fixup.go)

特点：
- 只改 `testee` 本身
- 后续最好再跑 `actor_fixup_timestamps`

### `actor_fixup_timestamps`

用途：
- 把 actor 侧关系时间回填成更拟真的历史时间

典型场景：
- 你已经修了 `testee.created_at`
- 希望 clinician / staff / relation 的时间也跟着变得合理

依赖：
- 本地 MySQL

实现入口：
- [seed_actor_fixup.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_actor_fixup.go)

特点：
- 只修 relation / clinician / staff 时间
- 不负责入口、答卷、报告

### `assessment_entries`

用途：
- 给已分配 testee 的 clinician 批量创建测评入口

典型场景：
- 你已经有 clinician 和 testee 关系
- 想让这些医生有可被打开的 entry

依赖：
- `apiserver`
- 本地 MySQL

实现入口：
- [seed_assessment_entry.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment_entry.go)

特点：
- 只创建 clinician 共享入口
- 会按 testee 的最早 `created_at` 回填 entry 时间

### `assessment_entry_flow`

用途：
- 从已有 entry 和已分配 testee 中，真实调用公开入口 API，生成入口打开和 intake 行为

典型场景：
- 你已经建好了 entry
- 想生成 `footprint.entry_opened` / `footprint.intake_confirmed`

依赖：
- `apiserver`

实现入口：
- [seed_assessment_entry_flow.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment_entry_flow.go)

它实际做的事：
- 找 active clinician
- 找 clinician 的 active entry
- 找 active access relation 的 testee
- 调公开入口接口：
  - resolve
  - intake

它不会做的事：
- 不直接写旧 `resolve_log`
- 不直接 patch relation 时间
- 不直接建 assessment

### `assessment_by_entry`

用途：
- 基于 `assessment_entry_flow` 已经接入成功的人，继续走真实答卷提交链路，形成测评服务过程

典型场景：
- 你已经有 intake 成功的 entry/testee
- 想生成：
  - `footprint.answersheet_submitted`
  - `footprint.assessment_created`
  - `footprint.report_generated`
  - `assessment_episode`

依赖：
- `apiserver`
- `collection-server`
- 本地 MySQL

实现入口：
- [seed_assessment_by_entry.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment_by_entry.go)

它实际做的事：
- 找 `source_type=assessment_entry` 的 creator relation
- 推导入口 target 对应的问卷/量表
- 走真实管理员提交答卷接口
- 等待 assessment 落库

它不会做的事：
- 不本地补建 assessment
- 不手工修 answersheet / assessment / report 时间

### `daily_simulation`

用途：
- 模拟“每天新增一批用户并完成接入/填报”的全链路过程

典型场景：
- 演示环境每日自动补数
- 想让增长、接入和测评数据都持续变化

依赖：
- `apiserver`
- `collection-server`
- IAM REST/gRPC

实现入口：
- [seed_daily_simulation.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_daily_simulation.go)

它大致会模拟：
- guardian 账号
- child/profile
- testee
- entry resolve/intake
- answersheet
- assessment

这是最接近“日常运营造数”的步骤。

### `assessment`

用途：
- 针对现有 testee，直接批量生成测评

典型场景：
- 你不关心 entry/intake，只想快速补 assessment/report
- 想让 evaluation 数据变多

依赖：
- `apiserver`
- `collection-server`

实现入口：
- [seed_assessment.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment.go)
- 答卷构建在 [seed_answersheet_builder.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_answersheet_builder.go)
- 提交重试在 [seed_answersheet_submit.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_answersheet_submit.go)

特点：
- 这是“直接做测评”的工具，不依赖 entry
- 更适合补 evaluation 量，不适合补接入链

### `plan_create_tasks`

用途：
- 为 plan 批量创建/补齐任务

依赖：
- 本地 MySQL
- 本地 MongoDB
- 本地 Redis

实现入口：
- [seed_plan_create.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_create.go)
- 本地 runtime/gateway 在 [plan_seed_gateway.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/plan_seed_gateway.go)

特点：
- 这是 plan 本地 runtime 模式
- 会拉起 apiserver container 依赖组件，但只在本地运行时里用

### `plan_process_tasks`

用途：
- 后台持续消费和处理 plan backlog task

依赖：
- `apiserver` API

实现入口：
- [seed_plan_process.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_process.go)
- 共用调度与限速逻辑在 [seed_plan_shared.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_shared.go)

特点：
- API-only
- 适合常驻 `tmux`
- 不再需要本地 MySQL/Mongo/Redis

### `plan`

用途：
- 兼容旧入口，等价于：
  - `plan_create_tasks`
  - 再接一次 one-shot `plan_process_tasks`

实现入口：
- [seed_plan_create.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_create.go)

适合：
- 想一次跑完 plan create + process

不适合：
- 长期常驻处理 backlog

### `plan_fixup_timestamps`

用途：
- 在 plan 跑完以后，把 task/assessment/report 时间修成“按 planned_at 回放”的历史时间

依赖：
- 本地 MySQL
- 本地 MongoDB

实现入口：
- [seed_plan_fixup.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_fixup.go)

特点：
- 这是离线时间修正
- 不是实时业务生成步骤

### `statistics_backfill`

用途：
- 按最新模型直接重建统计投影

依赖：
- `apiserver`
- 本地 MySQL

实现入口：
- [seed_statistics_backfill.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_statistics_backfill.go)

当前模型下它做的是：
- 等 projector 基本空闲
- 从 `behavior_footprint + assessment_episode` 重建：
  - `analytics_projection_org_daily`
  - `analytics_projection_clinician_daily`
  - `analytics_projection_entry_daily`
- 预热统计接口

它不会做的事：
- 不再调旧 internal sync API

## 5. 旧步骤为什么删除了

现在已经删除并禁用：

- `assessment_entry_fixup_timestamps`
- `assessment_fixup_timestamps`

原因：
- 最新模型下，真正的统计真相来自：
  - `behavior_footprint`
  - `assessment_episode`
  - `analytics_projection_*`
- 旧 fixup 会直接改旧时间字段，导致新分析模型失真

入口仍然保留在 [main.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/main.go)，但会直接报错并提示你改走真实步骤。

## 6. 每个主要源码文件是做什么的

下面这张表更偏“代码维护视角”，不是运行视角。

| 文件 | 作用 |
| --- | --- |
| [main.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/main.go) | CLI 入口，解析 flags，按 step 分发 |
| [config.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/config.go) | `seeddata.yaml` 配置结构定义与加载 |
| [api_client.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/api_client.go) | 调 apiserver / collection-server 的 HTTP 客户端 |
| [seed_actor.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_actor.go) | `staff` / `clinician` 创建、复用、更新 |
| [seed_clinician_generator.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_clinician_generator.go) | clinician 批量生成器 |
| [seed_testee_assignment.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_testee_assignment.go) | `assign_testees` |
| [seed_testee_fixup.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_testee_fixup.go) | `testee_fixup_created_at` |
| [seed_actor_fixup.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_actor_fixup.go) | `actor_fixup_timestamps` |
| [seed_assessment_entry.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment_entry.go) | `assessment_entries` |
| [seed_assessment_entry_flow.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment_entry_flow.go) | `assessment_entry_flow` |
| [seed_assessment_by_entry.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment_by_entry.go) | `assessment_by_entry` |
| [seed_daily_simulation.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_daily_simulation.go) | `daily_simulation` |
| [seed_assessment.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_assessment.go) | `assessment` 主流程 |
| [seed_answersheet_builder.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_answersheet_builder.go) | 生成答卷内容 |
| [seed_answersheet_submit.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_answersheet_submit.go) | 提交答卷与重试策略 |
| [plan_seed_gateway.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/plan_seed_gateway.go) | plan 本地 runtime gateway |
| [seed_plan_create.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_create.go) | `plan_create_tasks` / `plan` create 部分 |
| [seed_plan_process.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_process.go) | `plan_process_tasks` |
| [seed_plan_fixup.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_fixup.go) | `plan_fixup_timestamps` |
| [seed_plan_shared.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_plan_shared.go) | plan 共用并发、限速、节流逻辑 |
| [seed_statistics_backfill.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_statistics_backfill.go) | `statistics_backfill` |
| [seed_time_rules.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_time_rules.go) | 各种历史时间推导规则 |
| [seed_step_options.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_step_options.go) | 各 step 运行参数结构 |
| [seed_progress.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/seed_progress.go) | 进度条与命令行进度输出 |

## 7. 我该怎么选 step

### 场景 A：新环境初始化，先把基础人和关系造出来

推荐：

```bash
staff,clinician,assign_testees
```

适合：
- 初始化员工
- 初始化医生
- 把现有 testee 分给医生

### 场景 B：想让每个医生都有入口

推荐：

```bash
assessment_entries
```

前置：

```bash
staff,clinician,assign_testees
```

### 场景 C：想补“入口打开 / intake”链路

推荐：

```bash
assessment_entry_flow
```

前置：

```bash
assessment_entries
```

### 场景 D：想补完整的测评服务过程

推荐：

```bash
assessment_entry_flow,assessment_by_entry,statistics_backfill
```

如果你不关心 entry，只想快速把测评做出来：

```bash
assessment,statistics_backfill
```

### 场景 E：演示环境每天自动长数据

推荐：

```bash
daily_simulation
```

如果想让统计投影和首页更快稳定：

```bash
daily_simulation
statistics_backfill
```

### 场景 F：只处理 plan 业务

创建任务：

```bash
plan_create_tasks
```

持续处理 backlog：

```bash
plan_process_tasks
```

把历史时间修得更拟真：

```bash
plan_fixup_timestamps
```

### 场景 G：只想重建统计

```bash
statistics_backfill
```

## 8. 推荐命令模板

### 8.1 最小公共环境变量

```bash
export CFG=./configs/seeddata.yaml
export MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/qs?charset=utf8mb4&parseTime=True&loc=Local'
export MONGO_URI='mongodb://127.0.0.1:27017'
export MONGO_DB='qs'
export REDIS_ADDR='127.0.0.1:6379'
export REDIS_USERNAME='default'
export REDIS_PASSWORD=''
export REDIS_DB=0
```

### 8.2 典型一：完整 entry -> episode -> analytics

```bash
go run ./cmd/tools/seeddata --config "$CFG" --steps "staff,clinician"
go run ./cmd/tools/seeddata --config "$CFG" --steps "assign_testees"
go run ./cmd/tools/seeddata --config "$CFG" --steps "assessment_entries" --local-mysql-dsn "$MYSQL_DSN"
go run ./cmd/tools/seeddata --config "$CFG" --steps "assessment_entry_flow"
go run ./cmd/tools/seeddata --config "$CFG" --steps "assessment_by_entry" --local-mysql-dsn "$MYSQL_DSN"
go run ./cmd/tools/seeddata --config "$CFG" --steps "statistics_backfill" --local-mysql-dsn "$MYSQL_DSN"
```

### 8.3 典型二：快速补评估量

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "assessment" \
  --assessment-min 3 \
  --assessment-max 10 \
  --assessment-workers 10 \
  --assessment-submit-workers 10
```

然后：

```bash
go run ./cmd/tools/seeddata --config "$CFG" --steps "statistics_backfill" --local-mysql-dsn "$MYSQL_DSN"
```

### 8.4 典型三：plan create + process

```bash
go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_create_tasks" \
  --plan-id 614186929759466030 \
  --local-mysql-dsn "$MYSQL_DSN" \
  --local-mongo-uri "$MONGO_URI" \
  --local-mongo-database "$MONGO_DB" \
  --local-redis-addr "$REDIS_ADDR" \
  --local-redis-username "$REDIS_USERNAME" \
  --local-redis-password "$REDIS_PASSWORD" \
  --local-redis-db "$REDIS_DB"

go run ./cmd/tools/seeddata \
  --config "$CFG" \
  --steps "plan_process_tasks" \
  --plan-id 614186929759466030
```

## 9. 常见坑

### 坑 1：把 `assessment` 当成“完整接入链”

不是。  
`assessment` 更像“直接造测评”，它不会替你补完整 entry/intake 业务语义。

### 坑 2：忘了 `statistics_backfill`

现在统计读的是 `analytics_projection_*`。  
如果你跑了大量 seed 之后想立即看稳定结果，记得补一遍：

```bash
statistics_backfill
```

### 坑 3：还想用旧 fixup 步骤

现在已经不支持：

- `assessment_entry_fixup_timestamps`
- `assessment_fixup_timestamps`

这是故意删掉的，不是漏实现。

### 坑 4：plan 步骤依赖和普通 API 步骤不一样

`plan_create_tasks` / `plan_fixup_timestamps` 需要本地 runtime 依赖：
- MySQL
- MongoDB
- Redis

`plan_process_tasks` 则是 API-only。

### 坑 5：`assessment_entry_flow` 不再帮你 patch 旧日志时间

它现在只做真实业务 API 调用。  
如果你还期待它顺便改旧 `resolve_log`，那是旧模型思路，不适用了。

## 10. 如果我要重新梳理整个 seeddata，推荐的阅读顺序

如果你要从代码角度快速理解这个工具，我建议按下面顺序读：

1. [main.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/main.go)
   - 先看有哪些 step
2. [config.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/config.go)
   - 再看配置项长什么样
3. [api_client.go](/Users/yangshujie/workspace/golang/src/github.com/fangcun-mount/qs-server/cmd/tools/seeddata/api_client.go)
   - 理解它主要走哪些 API
4. `seed_actor.go -> seed_testee_assignment.go -> seed_assessment_entry.go`
   - 理解资源侧构建
5. `seed_assessment_entry_flow.go -> seed_assessment_by_entry.go -> seed_assessment.go`
   - 理解行为足迹和 episode 生成
6. `seed_statistics_backfill.go`
   - 理解统计投影怎么重建
7. `seed_plan_*.go`
   - 最后再看 plan 这条独立子系统

## 11. 一句话总结

现在的 seeddata 可以理解成三类工具：

- **资源构建工具**：`staff`、`clinician`、`assign_testees`、`assessment_entries`
- **业务事实生成工具**：`assessment_entry_flow`、`assessment_by_entry`、`daily_simulation`、`assessment`
- **离线修正 / 重建工具**：`testee_fixup_created_at`、`actor_fixup_timestamps`、`plan_fixup_timestamps`、`statistics_backfill`

如果你不知道该跑什么，优先问自己一句：

**你是要造“资源”，还是要造“行为和测评过程”，还是要“修历史/重建统计”？**

答案不同，对应的 step 也完全不同。
