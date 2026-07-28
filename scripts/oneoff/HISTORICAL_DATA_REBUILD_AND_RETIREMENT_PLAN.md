# P. 代码分析报告：历史数据重建与回填能力退场计划

> 状态：规划改造  
> 适用仓库：`iam` 、`qs-server` 、`seeddata-runner`  
> 目标数据范围：`2025-01-01..2026-07-27`（Asia/Shanghai，含首尾）  
> 文档目标：先精确删除旧的可归属模拟数据，再使用一个全新批次重建历史业务事实，最后将一次性回填控制面从在线系统退场。

## 1. 结论

这项工作不能被当作“删表 + 重跑 seeddata + 删代码”的一次发布。安全边界是三个彼此隔离的发布列车：

1. **旧数据清理**：使用现有 `seed_backfill_stage` 、runner manifest 和精确资源 ID 证明归属；先清 QS，后清 IAM。
2. **新批次重建**：使用新 batch ID 重新生成 IAM 身份和 QS 业务事实，并完成 Statistics 修复、验证和发布。
3. **一次性能力退场**：先关闭开关和轮换密钥，再删在线传输、编排与诊断代码，最后用新的 forward migration 删除 5 张回填控制表。

不允许把第 2 步产生的 AnswerSheet、Assessment、Outcome、Report、Plan、Entry 和 Statistics fact 当成“历史回填复杂度”删除。它们在验收后就是普通业务事实，必须继续支持 Statistics 重建和正常报告查询。

IAM 不需要进行生产代码级的“历史时间退场”：当前历史批次元数据只出现在 EnsureMockConsumer 契约测试和请求 Meta 中，IAM 没有回填专用表，也没有回写 `created_at/linked_at`。EnsureMockConsumer、Profile/Meta 传输和 ProfileLink 是普通 seeddata daemon 仍在使用的通用能力，不应随历史回填一起删除。

## 2. 分析目标、范围和边界

### 2.1 目标

- 删除当前能够被证明归属于旧 seeddata/历史批次的 IAM 与 QS 数据。
- 使用新批次完整重建 `2025-01-01..2026-07-27` 的业务事实。
- 证明 Statistics 能从重建后的主事实恢复出正确投影。
- 删除一次性回填的在线入口、传输协议、编排、幂等账本、回滚控制面和秘钥配置。
- 保留普通 daemon、REST/gRPC、K6、AnswerSheet 可靠受理和业务时间事实的正常语义。

### 2.2 不在本计划内

- 不清理无法证明归属的旧 IAM User/Profile 或 QS Testee。
- 不按整个 Org、模糊日期区间、昵称、手机号前缀或自增 ID 区间扩大删除。
- 不删除 IAM 通用 mock consumer 入口，除非另立项并同时退役普通 seeddata daemon。
- 不重写已执行 migration 59–62；表退场必须使用新的 forward migration。
- 不为了减少代码而丢掉 MySQL + Mongo + Outbox/Worker 的普通业务链路 E2E 保护。

## 3. 当前责任链与代码判断

### 3.1 当前入口链

```text
seeddata-runner historical-backfill
  -> IAM EnsureMockConsumer
  -> Collection CreateTestee / Profile / ProfileLink
  -> Entry resolve / intake
  -> Plan enrollment / task open / task complete
  -> AnswerSheet durable accepted + Mongo outbox
  -> Assessment + MySQL outbox
  -> Outcome
  -> Report
  -> seed_backfill_stage / attempt
  -> Statistics repair / validate / publish
```

退场时需要沿反向责任链处理依赖：先确认没有待处理事件，再删除 QS 业务对象，最后才允许删 IAM 身份对象。

### 3.2 qs-server 的临时复杂度

当前检出约 89 个与 `historicalseed` / `HistoricalExecutionContext` / `seed_backfill` 相关的源码、测试、配置和脚本文件，跨越 apiserver、collection-server、worker、proto、migration 和 CI。主要边界是：

- 签名与时间线：[`internal/pkg/historicalseed`](../../internal/pkg/historicalseed)。
- apiserver/collection-server 配置：[`internal/apiserver/options/historical_seed.go`](../../internal/apiserver/options/historical_seed.go) 与 [`internal/collection-server/options/historical_seed.go`](../../internal/collection-server/options/historical_seed.go)。
- REST/gRPC 传输和内部阶段查询：[`internal/apiserver/transport/rest/routes_historical_seed.go`](../../internal/apiserver/transport/rest/routes_historical_seed.go) 与 [`api/grpc/proto/common/historical.proto`](../../api/grpc/proto/common/historical.proto)。
- 完成账本与 attempt：[`internal/apiserver/port/historicalseedstage/stage.go`](../../internal/apiserver/port/historicalseedstage/stage.go) 与 [`internal/apiserver/infra/mysql/historicalseedstage/repository.go`](../../internal/apiserver/infra/mysql/historicalseedstage/repository.go)。
- 精确回滚：[`scripts/oneoff/cleanup_perf_testee_data/historical_rollback.go`](../../scripts/oneoff/cleanup_perf_testee_data/historical_rollback.go)。
- 跨存储 E2E：[`internal/apiserver/integration/historicalbackfill/production_closure_integration_test.go`](../../internal/apiserver/integration/historicalbackfill/production_closure_integration_test.go)。

现有 5 张回填控制表由 migration 59、61、62 引入：

- `seed_backfill_stage`
- `seed_backfill_stage_attempt`
- `seed_backfill_rollback_operation`
- `seed_backfill_rollback_resource`
- `seed_backfill_rollback_phase_attempt`

migration 60 引入的 `assessment_task.business_created_at` **不属于可直接删除的控制面**。Statistics collector 使用 `COALESCE(business_created_at, created_at)` 重建 `task_created` fact。如果删除该列，以后的 Statistics repair 会把历史任务投影到回填执行日，破坏可重建性，因此需要长期保留。

### 3.3 IAM 的当前判断

IAM 的 EnsureMockConsumer 是 seeddata-only 的通用内部入口，Profile/Meta 保存在 `auth_login_identities.profile_json/meta_json`。相同 provider/realm/identifier 重试会复用原 LoginIdentity，不覆盖原 Profile/Meta。

当前 IAM 中明确的 `seeddata_historical` / `seed_batch_id` / `seed_scenario_id` 契约仅位于测试；没有历史专用生产表、历史中间件或时钟。因此 IAM 的主要任务是“精确清理旧批次身份数据”，而不是删除正常生产模块。

### 3.4 seeddata-runner 的当前判断

runner 的临时复杂度分布在约 17 个文件，包括：

- `historical-backfill` / `historical-verify` / `historical-manifest` 三个 CLI。
- `HistoricalDaySnapshot`、manifest、checkpoint、冻结版本和阶段级 resume。
- HMAC 历史上下文和 qs-server stage 查询客户端。
- journey 中的批次 Meta、时间线和 Plan 子场景恢复分支。

只删 IAM + qs-server 的服务端能力会留下一个无法工作的 runner CLI，因此 runner 必须参与最终退场。普通 daemon 的 Journey 状态机、可靠提交和 `accepted_pending` 语义保留。

## 4. 不可破坏的契约

### 4.1 数据契约

- 新批次产生的业务事实在退场后仍可被普通 Statistics repair 重建。
- `entry_resolved_at <= entry_intake_at <= enrollment_joined_at <= answersheet_filled_at <= assessment_created_at <= assessment_submitted_at <= evaluated_at <= report_generated_at`。
- IAM `users.created_at`、`auth_login_identities.linked_at/created_at`、`profiles.created_at` 始终是实际创建时间，不伪造历史时间。
- 新批次的每个 Assessment 场景有且只有一个 AnswerSheet、Assessment、Outcome 和 generated Report。
- Plan 子场景中的 `task_id`、`origin_ref=plan_task/<task_id>` 和 task completion 一致。
- 不应用 stage/attempt 表代替业务事实；stage 表退场不得影响业务查询。

### 4.2 兼容契约

- 普通 REST、gRPC、daemon 和 K6 从未要求历史头，退场后行为不变。
- proto 删除 `historical_context` 时必须 `reserved` 原字段号和名称，禁止复用 11/2 等线上字段号。
- 旧 outbox/event 在退场前必须全部 drain；新 consumer 需有“忽略旧 JSON 的 `historical_context`”兼容测试。
- 已执行 migration 文件不修改、不删除、不重排编号。

### 4.3 安全契约

- 没有 manifest + 服务端完成账本 + 实际资源 ID 的交叉证据，不进行批量删除。
- 发现 Testee、Profile、User 或 LoginIdentity 有批次外引用时，整个删除 operation 在任何业务删除前失败。
- 密钥不进 YAML、manifest、receipt 或日志。
- 退场不允许依赖 `down.sql` 返回历史 migration 版本。

## 5. 当前可重构性评估

| 指标 | 判定 | 代码依据与含义 |
|---|---|---|
| 入口清晰度 | 绿 | runner 三个 CLI、QS 历史头和内部 stage API 都可枚举。 |
| 数据归属性 | 黄 | 新批次有 stage + manifest；旧的无批次 Meta 数据可能不能安全删除。 |
| 行为边界 | 黄 | 历史分支与普通服务共享 Entry、Plan、Survey、Evaluation、Interpretation 链路。 |
| 变更放大系数 | 红 | QS 相关文件约 89 个，且包含 proto/generated/OpenAPI/CI 派生变更。 |
| 测试保护 | 绿 | 已有历史签名、阶段幂等、resume、rollback 和跨存储 E2E。 |
| 可观测性 | 绿 | stage、attempt、rollback operation/resource/phase 可作为退场前验收证据。 |
| 安全抽离性 | 黄 | 可先删在线入口后删表，但 migration 60 的业务时间列必须保留。 |
| 回退性 | 黄 | 表未 drop 时可重发已封存的 backfill build；表 drop 后必须设置新的版本回退底线。 |

## 6. 目标状态

### 6.1 退场后保留

| 仓库 | 长期保留 |
|---|---|
| IAM | EnsureMockConsumer 通用内部入口、Profile/Meta SDK 传输、LoginIdentity 幂等复用、ProfileLink 授权契约、普通时钟。 |
| qs-server | AnswerSheet 可靠受理、两类 Outbox/Worker、Assessment/Outcome/Report、Entry/Plan 业务事实、Statistics repair/validate/publish、`assessment_task.business_created_at`。 |
| seeddata-runner | 普通 daemon、Journey 状态机、IAM mock consumer 客户端、可靠提交 ledger 与 daemon `accepted_pending` 语义。 |
| CI | 不带历史上下文的 AnswerSheet -> Assessment -> Outcome -> Report 跨存储必跑 E2E。 |

### 6.2 退场后删除

| 类别 | 删除对象 |
|---|---|
| 网络入口 | `X-QS-Historical-*` 中间件、历史签名验证、内部 stage REST API。 |
| 服务传输 | proto 中的 `HistoricalExecutionContext`、event payload 的 `historical_context`、collection/worker 传递逻辑。 |
| 应用分支 | `historicalseed.OccurredAt`、batch/scenario 幂等 replay、stage/attempt lifecycle、历史 Plan scheduler 分支。 |
| 配置与密钥 | apiserver/collection-server `historical_seed` 配置块与 `QS_HISTORICAL_CONTEXT_SECRET`。 |
| 一次性工具 | runner 历史三 CLI、历史 manifest/resume；QS 的 batch rollback 模式、`verify_historical_statistics`、`historical-backfill` Statistics 编排模式。 |
| 控制表 | migration 59/61/62 引入的 5 张表，在保留期结束后由新 migration drop。 |

## 7. 实施总体流程

```text
归属盘点
  -> 冻结旧批次
  -> QS dry-run / foreign-reference / backup / apply
  -> IAM dry-run / revoke / backup / apply
  -> 采集清理后 Statistics 基线
  -> 新 batch 的 Staging + Production pilot
  -> 新 batch 全量回填
  -> Statistics repair / validate / publish / K6
  -> 封存证据并关闭历史开关
  -> 删 runner 历史编排
  -> 删 QS 在线历史控制面
  -> 观察期结束
  -> forward migration drop 回填控制表
```

## 8. 阶段 A：旧数据归属盘点与冻结

### A-1. 建立唯一清理清单

输出一份不可变的 `old-seed-scope.json`，至少包含：

- 环境、Org ID、旧 batch ID 列表。
- 三库 HEAD、构建物 digest、数据库 migration 水位。
- runner manifest/checkpoint/ledger 的 SHA-256。
- IAM User、LoginIdentity、Profile、ProfileLink ID。
- QS Testee、Entry log/relation、Enrollment、Task、AnswerSheet、Assessment、Outcome、Report、Outbox/event 的精确 ID。
- 每个资源的归属来源：server stage、runner manifest、IAM identity Meta 或人工签名 allowlist。

归属判定优先级为：

1. QS `seed_backfill_stage` 的 completed 资源 + runner manifest 一致。
2. IAM `auth_login_identities.meta_json.seed_batch_id` + runner manifest 一致。
3. 对无 batch Meta 的旧数据，只接受明确 ID 的人工审批 allowlist。

任何只能通过日期、Org 或命名模式猜测归属的数据不进入清理范围。

### A-2. 冻结与基线

- 停止普通/历史 seeddata runner，不再创建新 mock 数据。
- 阻止这些 mock 身份继续产生 QS 业务写入。
- 采集清理前 MySQL、MongoDB、Redis 和 Statistics 基线。
- 对 IAM 和 QS 分别生成同一时点可恢复备份。
- 检查 MySQL/Mongo outbox、retry hold、runtime checkpoint，不允许有未完成的旧批次事件。

### A-3. 停止门禁

- manifest/stage 的资源 ID 不一致。
- 旧批次存在 running attempt 或 rollback operation 未完成。
- 任一 Testee/Profile/User 存在批次外业务或身份引用。
- 无法取得可恢复备份或无法证明备份可读。

## 9. 阶段 B：精确删除当前 QS 历史数据

### B-1. 使用现有持久化回滚操作

对每个旧 batch 先运行 `cleanup_perf_testee_data --seed-batch-id ...` dry-run，并提供完整 runner manifest。必须验收：

- receipt 为 v2，manifest hash 和 scope hash 与审批清单一致。
- rollback resource 已物化 MySQL 主键和 Mongo `_id`。
- MySQL 的 Testee-bearing 表和 Mongo collections 的 foreign-reference 检查均为 0。
- dry-run 中的 Testee、AnswerSheet、Assessment、Outcome、Report、Plan 数量与 manifest 符合。

apply 必须在受控维护窗口内执行，按现有 operation 水位自动恢复：

```text
scope 复核
  -> foreign-reference 再检查
  -> MySQL/Mongo 备份
  -> 按 Mongo _id 删除
  -> 按 MySQL 主键删除业务事实
  -> Statistics repair / validate / latest-complete-day publish
  -> 删 stage attempt
  -> 删 stage
  -> operation completed
```

业务删除范围包含 AnswerSheet/idempotency/outbox、Assessment/score/task、Outcome、Report/generation/run/artifact、Plan enrollment/task、Entry resolve/intake log、relation、runtime/retry/outbox 和受影响 Statistics facts/projections。删除完成账本和 attempt 必须是 QS 跨存储操作的最后一步。

### B-2. 清理后验收

- operation 达到 `completed`，每个 phase 有 completed attempt。
- 精确 ID 在 MySQL/Mongo 中不再存在，批次外控制组未变。
- Statistics 受影响日期已 repair/validate，publish 水位等于执行时上海时区的最新完整日。
- 回滚 operation/resource/phase attempt、receipt 和备份仍保留，不在本步删表。

## 10. 阶段 C：精确删除当前 IAM mock 身份数据

IAM 必须在 QS 清理完成后处理，因为 QS Testee 和报告授权仍引用 IAM Profile/ProfileLink。

### C-1. 新增临时 one-off 清理工具，不新增生产 API

在 IAM `scripts/oneoff` 中实现一个只读 dry-run/显式 apply 工具，输入必须是签名审批的 `old-seed-scope.json`。不新增 User/Profile 硬删 REST/gRPC 接口。

dry-run 需要物化并校验：

- `users`、`auth_login_identities`、`auth_credentials`、`auth_token_audit`。
- `profiles`、`profile_links`。
- Redis 中与 User/LoginIdentity 相关的活跃 session/revocation 状态。
- User 是否还有非批次 LoginIdentity，Profile 是否被非批次 ProfileLink 使用，ProfileLink 是否与 manifest ID 一致。

如果某个旧身份因 Ensure 复用而没有新 Meta，不能把“Meta 缺失”自动解释为可删；必须由 runner manifest 中的 User/LoginIdentity ID 和人工 allowlist 证明归属。

### C-2. IAM 删除顺序

```text
撤销 User/LoginIdentity 活跃 session
  -> 备份精确行并产生 scope hash receipt
  -> auth_token_audit
  -> auth_credentials
  -> auth_login_identities
  -> profile_links
  -> profiles
  -> users
  -> 按 ID 复核 + 控制组复核
```

每个删除是按明确主键执行的幂等操作。一次执行中断后只能用同一 receipt/scope hash 恢复，不能重新扫描并扩大范围。备份与 receipt 至少保留 90 天。

### C-3. IAM 停止门禁

- 批次 User 还有非批次 LoginIdentity、ProfileLink 或审计依赖。
- Profile 被批次外 User 链接。
- QS 中仍存在对该 Profile/User/Testee 的引用。
- session 无法撤销或备份恢复演练失败。

## 11. 阶段 D：使用 seeddata 重建历史数据

### D-1. 必须使用新 batch ID

不得复用已删除的 `hist-20250101-20260727-v1`。新批次建议使用经审批的唯一 ID，例如：

```text
hist-20250101-20260727-v2
```

原因：

- IAM 身份命名、QS stage 唯一键、runner checkpoint/manifest 和 rollback receipt 都以 batch 为身份边界。
- 旧 rollback operation 和证据尚在 90 天保留期内，复用 batch 会混淆新旧归属。
- 新 batch 命名空间保证 IAM Ensure 只复用同一新场景，不依赖更新旧 Identity Meta。

pilot 必须使用另一 batch ID，不能与正式 v2 共用账本。

### D-2. 回填前准备

- 采集“旧数据删除后、新数据写入前”的 Statistics 基线，作为新批次增量对账起点。
- 冻结全部 Plan、Questionnaire、Assessment Model 的精确已发布版本，写入 manifest。
- 重新验证 HMAC 组织、日期、时区和新鲜度边界，密钥只由环境变量注入。
- 停止普通 seeddata daemon 和 Plan scheduler；保持 Evaluation/Interpretation worker 和 Outbox relay 运行。
- 确认 migration 59–62 均已应用，因为它们在本次重建完成前仍是幂等、诊断和回滚依据。

### D-3. 执行次序

1. 本地/CI：3 天，四种 journey 和 Plan 子场景都有样本。
2. Staging pilot：3 天，注入一次中断并验证 `--resume`。
3. Staging 整月：`2025-01-01..2025-01-31`，完成 Statistics repair/validate/publish 和一次精确 rollback 演练。
4. Production pilot：独立 batch 的 3 天，完整验收后精确回滚。
5. Production full：新的正式 batch，`2025-01-01..2026-07-27`，每日验证后才推进 checkpoint。

正式命令必须使用已发布且封存 digest 的 runner build：

```bash
seeddata historical-backfill \
  --config configs/seeddata.yaml \
  --from 2025-01-01 \
  --to 2026-07-27 \
  --batch-id hist-20250101-20260727-v2
```

中断后仅允许用同一 build、配置、batch 和冻结版本执行 `--resume`。

### D-4. 日级和批次级验收

每个业务日都必须验证：

- 确定性人数、journey mix、场景数和 60% Plan 任务完成选择。
- 本地 ledger、runner manifest 和 QS completed stage 的 payload hash、business_at、resource type/ID 一致。
- Assessment 场景具备 submitted AnswerSheet、Assessment created/submitted、Outcome committed 和 Report generated。
- 时间线不倒序、不跨上海自然日、不落入未来。
- 日级验证成功后才写 `DailyCounts`、Terminal 和 `CompletedThrough`。

全量完成后：

1. 重新运行 batch verify，不复用日级缓存结论。
2. 对 `2025-01-01..2026-07-27` 按不超过 31 天分窗 repair/validate。
3. 对截止日之后到执行时最新完整上海自然日的缺口继续分窗 repair/validate。
4. 只以执行时最新完整日 publish，并校验水位。
5. 使用清理后基线逐日对比 Statistics 增量。
6. 运行 `perf-preflight` 和 `perf-smoke`，必须自动发现真实 Report 样本，不得为 no-report degraded。

## 12. 阶段 E：回填证据封存与能力关闭

这一阶段在数据验收后立即执行，但不立即 drop 表。

- 关闭 apiserver/collection-server `historical_seed.enabled`。
- 恢复 Plan scheduler 和普通 seeddata daemon。
- 轮换并移除 `QS_HISTORICAL_CONTEXT_SECRET`，确认所有实例均无旧密钥。
- 标记和封存三库回填 build/tag、容器 digest、配置 hash 与 migration 水位。
- 封存 runner manifest/checkpoint/ledger、QS stage/attempt 导出、Statistics 基线/验证结果、K6 结果、rollback receipt 和备份。
- 建立至少 90 天的可恢复观察期；期间出现数据错误时，使用封存 build 和现存账本 fix-forward，不边删代码边修数据。

进入代码退场前必须证明：

- 没有未完成的 historical outbox/retry/hold/runtime checkpoint。
- 新 batch 没有 failed/running attempt。
- Statistics 连续两次全量验证结果一致。
- 封存备份已在隔离环境完成一次读取/恢复演练。

## 13. 阶段 F：qs-server 在线历史复杂度退场

采用 expand/contract，分批发布，不在同一版本中同时删代码和表。

### F-1. 删除外部与传输入口

- 删除 REST historical middleware、stage 查询路由/handler 及 OpenAPI 操作。
- 删除 apiserver/collection-server 的 `historical_seed` options、flags、YAML 配置和组装逻辑。
- 删除 `X-QS-Historical-Context/Requested-At/Signature` 验证与 `internal/pkg/historicalseed/signature.go`。
- 从 answersheet/evaluation/interpretation proto 删除 optional `historical_context`，对原 field number 和 name 增加 `reserved`，再重新生成 Go 代码。
- 部署前确认没有旧版 runner/collection/worker 仍发送历史上下文。

### F-2. 删除应用与 Worker 传播

- 删除 context 在 Collection -> apiserver -> outbox -> Worker -> gRPC 的传递。
- 删除 event payload 中的 `historical_context`，保留对旧 JSON 多余字段的兼容测试。
- 删除 Entry、Plan、AnswerSheet、Assessment、Outcome、Report 中的 batch/scenario/stage/attempt 分支。
- 删除历史模式的 Plan scheduler pause 和 task replay/strict completion 分支，保持普通请求原有语义。
- 删 `historicalseed.OccurredAt`；如某个通用 `...At` 领域方法已用于正常业务时钟和测试注入，则保留该通用方法，只删 historical context 判定。
- 保留 `assessment_task.business_created_at` 及 Statistics collector 的 fallback，确保历史任务可永久重建。

### F-3. 删除阶段账本在线依赖

- 删除 `historicalseedstage` port/repository 与所有 container 注入。
- 删除 stage reader API 和 `include_attempts=true` 诊断入口。
- 保留表本身在观察期内的只读取证价值；在线进程已不得读写它们。
- 连续至少一个完整发布观察窗口确认数据库监控中无这 5 张表的应用读写。

### F-4. 一次性工具与 CI 收敛

- 从 `cleanup_perf_testee_data` 中删除 `--seed-batch-id` 和 persisted historical rollback 模式，保留其他明确仍有用的手工 Testee 清理能力。
- 删除 `verify_historical_statistics`；`rebuild_statistics` 保留通用 repair/validate/publish，删除只服务于回填的 `historical-backfill` mode。
- 当前 `Historical Backfill Cross-Store E2E` 不能直接从 CI 消失。将它改写为不带历史上下文的必跑 `AnswerSheet Production Closure E2E`，继续覆盖 MySQL + Mongo Replica Set + Redis + Outbox/Worker + Statistics。
- 历史特有的签名、stage replay、attempt 注入失败测试随代码删除；普通链路的唯一性和 Worker 重试测试保留。

## 14. 阶段 G：seeddata-runner 历史编排退场

该批应当早于 QS proto/服务端字段删除发布，保证没有活跃客户端继续依赖它们。

- 删除 `historical-backfill`、`historical-verify`、`historical-manifest` CLI 及参数。
- 删除 `internal/historicalseed`、`historical_backfill.go`、HistoricalDaySnapshot、manifest/checkpoint、冻结版本和阶段级 resume。
- 删除 seed API/client 的签名历史头和 QS stage 查询。
- 删除 Journey 中的 `source=seeddata_historical`、batch/scenario Meta、历史时间线和 Plan 子场景恢复分支。
- 保留共享的 journey 停止语义、origin_ref、AnswerSheet durable accepted、daemon accepted_pending 和通用幂等键。
- 在删除前以 tag/容器 digest 封存最终 backfill build，但不继续在 `main` 中维护一次性 CLI。

## 15. 阶段 H：IAM 代码和临时工具收尾

- 删除阶段 C 新增的 one-off 身份清理工具，或将其连同 receipt/schema 冻结到受控运维归档；不把它演变成生产删除 API。
- 将 IAM 契约测试中的 `seeddata_historical` 示例改为中性 `seeddata` Meta，但保留 Profile/Meta 首次写入、重试不覆盖、SDK 传输和 ProfileLink 授权契约。
- 不删 EnsureMockConsumer、Signup 领域服务、LoginIdentity Profile/Meta 列或 ProfileLink。
- 不新增、不保留 IAM 历史时钟或硬删 API。

IAM 最终预期是“生产逻辑无回填特例，仅保留普通 seeddata 契约”，而不是删掉整个 mock consumer 能力。

## 16. 阶段 I：控制表退场

只有同时满足以下条件才能创建新的 qs-server forward migration：

- 新批次验收和至少 90 天可恢复观察期结束。
- 备份、manifest、stage/attempt 导出、rollback 证据的保留策略已转移到数据库外的受控存储。
- 生产所有 qs-server/worker/collection 版本已不读写 5 张表。
- 数据库审计或 query telemetry 在至少一个完整发布窗口中证明无访问。
- 已声明新的 application rollback floor：drop 表后不允许回退到仍依赖历史表的旧版本。

新 migration（实施时使用当时最新编号，不在本文档预占固定号）按外键与诊断价值从低到高删除：

```text
seed_backfill_rollback_phase_attempt
  -> seed_backfill_rollback_resource
  -> seed_backfill_rollback_operation
  -> seed_backfill_stage_attempt
  -> seed_backfill_stage
```

down migration 只能重建空表结构，不能伪装能恢复已删除的诊断数据。生产回退依赖备份和新的 fix-forward，不依赖 down migration。

## 17. 分批交付与部署顺序

| 批次 | 仓库 | 交付内容 | 前置门禁 |
|---|---|---|---|
| R0 | 三库 | 封存当前 HEAD，产生旧数据归属清单与备份 | 无模糊归属 |
| R1 | IAM | 临时精确身份清理工具及 characterization tests | dry-run/foreign-reference/中断恢复通过 |
| R2 | 数据操作 | QS 旧 batch cleanup，然后 IAM cleanup | 双库备份可恢复 |
| R3 | seeddata + QS + IAM | Staging/Production pilot，正式新 batch 回填 | 每日门禁、Statistics/K6 通过 |
| R4 | 运维 | 关闭历史开关、轮换密钥、封存证据 | 业务事实与投影对账一致 |
| R5 | seeddata-runner | 删历史 CLI/编排/上下文，保留 daemon | 封存 backfill build 可取得 |
| R6 | qs-server | 删历史入口/proto/event/应用分支/stage 在线依赖 | 无待处理历史事件 |
| R7 | qs-server | 把跨存储 E2E 改为普通链路 required job | 普通链路唯一性/重试已覆盖 |
| R8 | IAM | 退场临时清理工具，中性化历史测试文案 | 备份/receipt 已归档 |
| R9 | qs-server | 观察期后新 migration drop 5 张表 | 无应用访问，回退底线生效 |
| R10 | 文档 | 将旧 runbook 标为历史资料，更新架构索引 | 代码、配置、表均已退场 |

每个仓库使用 fix-forward 提交，不 force-push、不改写已发布回填历史。结构删除、行为修复、generated 更新和 migration 必须分开提交。

## 18. 测试与验收计划

### 18.1 数据清理

- QS historical rollback：scope hash 漂移、manifest 不一致、foreign reference、Mongo/MySQL/Statistics 分阶段中断恢复、控制组不受影响。
- IAM one-off：Meta + manifest 交叉归属、Identity 复用且 Meta 缺失、非批次 Identity/ProfileLink 拒绝、session 撤销、重复 apply 幂等。
- 对 MySQL/Mongo 备份进行实际恢复演练，不仅检查备份文件存在。

### 18.2 历史重建

- 保留 runner 的 573 日确定性、journey 终态矩阵、60% Plan、版本漂移、每阶段“服务端成功/客户端未落账” resume 测试。
- 保留 QS 历史签名、stage replay/conflict、attempt lifecycle、rollback 和跨存储 E2E，直到正式批次完成。
- Staging 和 Production pilot 均必须完成一次真实回滚演练。

### 18.3 代码退场

- 架构检查：全库不再出现 `historicalseed`、`HistoricalExecutionContext`、`X-QS-Historical-`、`seed_backfill_stage` 在线引用；只允许 migration 历史和归档文档中留存。
- proto 兼容：原字段号/名称已 reserved，旧消息的未知字段可被安全忽略。
- event 兼容：旧 payload 多余字段不影响普通 consumer，且无待处理历史事件。
- Statistics 回归：从主事实重建 `2025-01-01..2026-07-27` 仍与退场前基线一致，包括 task business time。
- 普通 daemon、REST/gRPC、K6 的行为、错误和性能门禁不变。

最终必跑门禁：

```text
iam:             go test ./... + 关键契约包 race
qs-server:       go test ./... + 相关 race + docs-verify + perf-verify
seeddata-runner: go test ./... + go test -race ./...
cross-store:     MySQL 8 + Mongo Replica Set + Redis + Outbox/Worker required E2E
workspace:       git diff --check + 工作区无未说明改动
environment:     Statistics validate + perf-preflight + perf-smoke non-degraded
```

## 19. 立即停止条件

任一条触发时，不进入下一个数据日、删除 phase 或发布批次：

- batch/manifest/scope/payload/version 不一致。
- 旧数据不能通过精确 ID 证明归属。
- IAM 或 QS foreign-reference 检查非 0。
- 备份不可读、不可恢复或 receipt/scope hash 漂移。
- 任一必需 stage/resource ID 缺失，Report 未 generated，Plan task 未 completed。
- 时间越界、倒序、落入未来或 Statistics fact conflict。
- 退场前仍有旧版实例、待处理历史 event 或对控制表的读写。
- 跨存储 E2E、Statistics validate 或 non-degraded K6 失败。

## 20. 规模预估

| 工作项 | 预估 |
|---|---:|
| 旧数据归属盘点、备份和 IAM one-off 清理工具 | 3–5 工程日 |
| 旧 QS/IAM 数据清理与恢复演练 | 2–4 工程日 |
| Staging/pilot/正式重建与 Statistics/K6 验收 | 4–8 工程日，实际运行时间另计 |
| seeddata-runner 历史编排退场 | 2–4 工程日 |
| qs-server 传输/应用/账本/工具退场 | 6–10 工程日 |
| IAM 收尾与契约中性化 | 0.5–1 工程日 |
| 控制表 forward migration、发布和回退底线 | 1–2 工程日 |
| **合计** | **18.5–34 工程日 + 90 天观察保留期** |

## 21. 完成定义

只有同时满足以下条件，才可声明历史数据重建与能力退场完成：

- 旧 batch 可归属 IAM/QS 数据已精确删除，不可归属数据被明确保留。
- 新唯一 batch 已完成 573 个业务日的所有预期场景。
- IAM 身份与 QS 业务事实的资源 ID、时间顺序和归属证据完整。
- Statistics 可从主事实重建，最终 publish 水位等于执行时最新完整上海自然日。
- perf preflight/smoke 能找到真实 Report 样本且不 degraded。
- 历史开关已关闭，密钥已轮换移除，无待处理历史事件。
- runner 不再包含历史 CLI，QS 不再包含历史在线入口、传输、stage/attempt 依赖。
- IAM 无历史特例，但普通 EnsureMockConsumer/Profile/Meta/ProfileLink 契约仍在。
- 90 天保留期结束后，5 张回填控制表已通过 forward migration 删除，新版本回退底线已生效。
- 跨存储普通 AnswerSheet -> Report E2E 仍是 CI required job。

## 22. 实施前的下一步

本文档定义了目标和门禁，不授权立即删除生产数据。第一个实施批次只做两件事：

1. 生成旧数据 `old-seed-scope.json` 和三库/三存储基线，全程只读。
2. 为 IAM 临时 one-off 清理工具补 characterization tests，先证明可精确枚举、可拒绝 foreign reference、可恢复，再实现 apply。

完成这两项并人工审批清单后，才进入 QS -> IAM 的实际数据清理。
