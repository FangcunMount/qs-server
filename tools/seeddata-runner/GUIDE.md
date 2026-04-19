# Seeddata Guide

## 模块定位

`tools/seeddata-runner` 是仓库内独立 module，负责模拟真实用户行为，不属于 `qs-server` 主模块运行时。

它启动后固定并发跑两条后台任务：

1. daily simulation
2. plan open-task submit

## 1. daily simulation

用途：

- 每天自动生成一批新用户
- 完成扫码、注册、建档、分配 clinician、填写答卷
- 按旅程目标模拟只注册、只到 testee、到 entry 但不提交等中途停止场景

实现上按职责链拆成固定阶段：

1. `guardian_account`
2. `child_profile`
3. `testee_profile`
4. `assessment_entry`
5. `answersheet_submit`

职责链允许在中途阶段提前停止，便于构造更真实的漏斗数据。

## 2. plan open-task submit

用途：

- 常驻扫描指定 plan 当前已经进入 `opened` 的 task
- 只负责提交答卷，模拟“用户完成 task”
- 不负责 plan 生命周期推进

说明：

- `worker` 负责 `schedule/open/expire/complete`
- `seeddata` 只负责把已经 open 的 task 假装成用户完成填写

## 3. 推荐运行方式

```bash
cd tools/seeddata-runner
./scripts/run_seeddata_daemon.sh
```

或：

```bash
cd tools/seeddata-runner
go run ./cmd/seeddata --config ./configs/seeddata.yaml
```

## 4. 配置原则

`configs/seeddata.yaml` 只保留：

- `global`
- `api`
- `iam`
- `dailySimulation`
- `planSubmit`

其中：

- `planSubmit.planIds` 是 open-task submit daemon 的目标 plan 集合
- `planSubmit.workers` 控制 open-task submit 并发
- `dailySimulation.*` 的默认值由配置加载阶段统一补齐，daemon 不再各自做零散 normalize

如果 daily simulation 需要限制医生范围，优先使用 `dailySimulation.clinicianIds`。

如果需要模拟不同深度的用户旅程，使用 `dailySimulation.journeyMix`：

- `registerOnlyWeight`
- `createTesteeWeight`
- `resolveEntryWeight`
- `submitAnswerWeight`

如果不想把 IAM 凭据写在配置文件中，可以在启动前注入：

- `IAM_USERNAME`
- `IAM_PASSWORD`

当 `api.token` 为空时，runner 会先读取这两个环境变量，再回退到配置文件里的 `iam.username` / `iam.password`。
