> 历史资料：2026-10-07 整理前快照。原文件 `docs/05-决策记录/README.md`；源码基线 `2ccc2de44bbd45e26d29e7e130da518cc32426f0`。文中“当前”和验证结果仅适用于原记录时点。
> 快照来源提交：`2ccc2de44bbd45e26d29e7e130da518cc32426f0`；原文 SHA256：`18338b9d867af33f229f5fc34d28c7b7f39d65b278a178f43ee50fcbce9be893`。只调整引用位置，历史结论仍按原记录时点解释。

# 决策记录

本层记录当前仍成立的架构取舍，以及已经确认方向、可独立交付实施的重构分析。实现细节回链业务模块或基础设施，不在这里重复。

## 1. 当前决策

见 [架构决策总表](../../../../../05-决策记录/01-架构决策总表.md)。每项决策至少包含：背景、结论、约束、证据入口和重新评估条件。

## 2. 已确认并实施的重构决策

- [同步保存答卷，异步执行评估与报告](../../../../../05-决策记录/01-架构决策总表.md#2-同步保存答卷异步执行评估与报告)：`202 Accepted` 表示 `AnswerSheet + 幂等事实 + Outbox` 已可靠持久化，Assessment 通过 Worker 异步生成；
  完整实现与失败窗口回链 Survey 和 Concurrency canonical 文档。
- [seeddata 历史能力退役](../../../../../05-决策记录/01-架构决策总表.md#7-seeddata-历史能力永久退役)：永久拒绝历史请求，删除批次控制面和废弃存储对象，只保留当前时间业务链路与每日 mock。

已经实施完毕的完整重构分析退出 active 层；当前取舍只在决策总表保留，历史推演由 archive 和 Git 追溯。当前实现冲突时，以决议节、当前代码和机器契约为准。

## 3. 不收录什么

- 尚未确认、无法形成实施边界的目标架构；
- 已被当前代码推翻的历史方案；
- 纯组件介绍或接口字段清单；
- 无法指向当前代码/契约的长篇推演。

历史专题已随重建前文档树进入 `_archive`，不能直接引用为当前决策。

## 4. 仓库代码生命周期审计

2026-10-02，以 `c9d1045e2b11` 为审计前基线，完成全仓入口和包依赖扫描，并实施第一批无运行调用的残留代码删除。
2026-10-03 继续处理 clinician 与 Outbox 适配重复，并将审计分支快进到 main `308a0d966529`（补齐 50 个提交）。
当前问题主要是未使用的包装层、仅测试调用的预留实现和局部复制；没有发现旧 AI 引擎或旧 Outbox writer 重新进入正式组合根。

### 4.1 范围与判断方法

基线包含 451 个 Go 包、31 个可执行入口。其中 `cmd` 下有 3 个正式服务和 7 个维护、验收或联调工具；
其余入口位于 `scripts`。包依赖图以所有可执行入口为根，函数可达性还包含测试、接口绑定和反射分析。
同时检查默认平台与 Linux amd64，并在 Linux 下加载 `integration`、`reliable_messaging`、
`reliable_messaging_m4`、`reliable_messaging_m4_integration`、`reliable_messaging_m5` 标签。

基线默认运行入口扫描报告 180 个不可达函数；包含测试后为 40 个，Linux 集成标签下也是 40 个，交集为 39 个。
这些数字是审计候选，不能直接作为删除数量。测试替身的接口方法、接口约束方法、Swagger 注释入口和不同构建标签的调用均需要人工排除。
例如 `BaseInfo.UpdateTitle` 在默认扫描中不可达，但集成证明仍使用它，本轮保留。

全仓重复扫描使用既有 maintainability 配置中的 150 节点阈值，共报告 41 条提示，其中 18 条位于测试文件。
重复提示可能双向报告同一对代码，也可能来自必要的领域转换，不等于 41 个废弃实现。

### 4.2 已实施清理

| 项目 | 调用及替代证据 | 本轮结果 |
| --- | --- | --- |
| 微信适配层的旧端口别名 | 所有可执行入口都不引用旧 `infra/wechatapi/port`；适配器直接使用 [wechatmini 端口](../../../../../../internal/apiserver/port/wechatmini) | 删除旧别名文件 |
| Swagger Go 嵌入层 | 两个 [Dockerfile](../../../../../../build/docker)复制静态目录；两个 REST router 通过目录提供页面，没有 Go 包调用 | 删除 `web/swagger-ui/embed.go`，保留发行资源和部署复制 |
| AI 输出契约 Go 包装 | 初次审计无调用；main 后续新增 [MBTI 输出校验](../../../../../../internal/apiserver/application/aibridge/mbti_output.go)，直接调用 V2 嵌入契约 | 同步 main 时撤回整文件删除，保留最新嵌入包与 v1/v2 JSON 契约 |
| 通用 HTTP 授权快照中间件 | 没有注册方；正式 apiserver 使用 [进程专属中间件](../../../../../../internal/apiserver/transport/rest/middleware/authz_snapshot_middleware.go) | 删除通用层残留及其未使用的 Gin key |
| IAM 授权快照旧适配 | `NewAuthzSnapshotReader` 及其 port 只相互引用，没有客户端 | 删除实现和对应两个接口，保留正式 snapshot loader 与 action checker |
| 无调用的便捷入口 | 默认及 Linux 集成标签均不可达；事件创建已使用事务前固定的 metadata，L1 已使用分桶配置，Outbox 状态已使用事件类型版本 | 删除旧事件构造器、L1 便捷构造器、旧状态构造器、闲置错误包装和 interval helper |
| 旧契约测试辅助代码 | AI/OpenAPI 的十个函数只自调用或没有调用，另有一个未使用常量；实际测试通过其余 helper 编译和执行 | 删除辅助代码，保留实际契约断言 |
| Operator 工具数据库连接 | prepare/recover/retire 三份实现除目的错误消息外相同，返回的 SQL handle 均被丢弃 | 共用 [maintenance.OpenMySQL](../../../../../../internal/apiserver/maintenance/mysql.go)，保持池大小、时区、静默日志、失败消息和关闭责任 |

第二批已完成：

- 删除 `application/interpretation/clinician` 的闲置实现；后台报告仍走 Administration，旧医生路由仍返回 410。
  以真实服务的详情/列表矩阵承接受限投影保护，补列表拒绝不读取测试，并修正文档与 closure 来源。
- MySQL/Mongo `ReplayLedger` 保留端口方法和事务实现，共用 [outboxreplay](../../../../../../internal/apiserver/infra/outboxreplay/adapter.go)
  执行请求转换、错误分类和结果不明回查。补输入冲突、确认丢失、回查成功/失败、取消后限时回查与只读 pending 恢复测试；
  保持顺序、reason 原文、错误码、nil/empty 形态及三秒回查窗口。

### 4.3 后续独立整理清单

| 优先级 | 位置 | 已知事实与建议 | 实施前需要补齐的边界 |
| --- | --- | --- | --- |
| 中 | [AI 管理 handler](../../../../../../internal/apiserver/transport/rest/handler) | Profile/Suite、SemanticDraft/Solution 有重复的权限上下文和错误处理，全部都有路由注册；可提取局部公共处理 | Profile/Suite 上限为 256 KiB/16 KiB；SemanticDraft/Solution 上限为 240 KiB/256 KiB，参数分别为 revision/cursor，不能统一掉这些差异 |
| 中 | [版本化查询缓存](../../../../../../internal/pkg/cache/query/versioned.go) | `NewVersioned` 仅测试构造，Get/Invalidate 没有调用；属于预留实现候选，应确定采用它还是删除整组实现 | 保留当前 version token 和实际 L1/L2 缓存路径；不能因为名称类似就移动现有缓存规则 |
| 中 | [taskperformance 元数据](../../../../../../internal/apiserver/domain/modelcatalog/taskperformance) | 包只有自身测试使用，文档将其作为后续任务模型扩展空间；目前不支撑真实入口 | 判断是否仍有明确采用计划，再删除或接入真实调用；当前 cognitive registry 和报告 Builder 不受此包控制 |
| 中 | [AI 联调 CLI](../../../../../../cmd/qs-ai-bridge) | 正式进程已拥有投递和结果接收，CLI 仍随镜像发布；它另有独立 runtime-index-backfill 操作 | 先明确联调与历史回填的发布载体，再拆分镜像；保留命令协议和跨仓联调引用 |
| 低 | [cmd 入口](../../../../../../cmd) | 七个工具与三个服务并列，正式构建已明确只构建三个服务 | 目录移动需要同时修订 Docker、CI、脚本和文档引用；本轮保持二进制名称和路径 |

上述清单仅列尚未实施的候选；clinician 与公共重放适配已在第二批完成。

### 4.4 有保留理由的历史代码

- [兼容观察工作流](../../../../../../.github/workflows/compatibility-observation.yml)检查旧 practitioners 路由、Statistics validate_only、
  assessment 命名的报告 RPC、旧幂等查询和旧模型绑定。路由/数据仍有明确读取或拒绝用途，删除需要完整观察窗口、调用方和数据责任确认。
  本次没有取得生产观测证据，不能以静态扫描替代退出判断。
- [SDK legacy envelope](../../../../../../internal/pkg/eventing/runtime/publisher.go)仍用于当前生产 wire 协议。
  名称包含 legacy 不等于废弃 writer；正式组合根只接受 standard profiles，回退依赖保留镜像。
- [历史迁移](../../../../../../internal/pkg/migration/migrations)保留新环境初始化与升级能力，本轮未改动数据库对象或迁移序列。
- AI JSON 输出契约、Swagger 静态资源、生成的 protobuf/OpenAPI、测试夹具和用于说明边界的 doc-only 包不按普通运行函数删除。
- conclusionKind 等接口约束方法即使扫描报告不可达，也需要保留接口满足关系；WebSocket Swagger 占位函数参与文档生成，同样保留。

### 4.5 验证与限制

第一批删除前已运行受影响包的测试，删除后重新运行；全仓默认测试和 Linux 集成标签编译通过，Operator 工具与维护包测试通过。
受影响包的 race 检查、分层边界检查、文档 hygiene/facts 与差异空白检查通过，文档数量维持 165 篇。
标准 lint 的既有问题由 30 条降为 18 条，
消除了全部 12 条 unused 提示；剩余为 12 条 errcheck 和 6 条 staticcheck，没有新增诊断。
剩余 staticcheck 中的 Mongo `NewClient` 用于创建未连接的测试客户端；`PlanEntryService` 仍传递 deprecated token，
实际 resolver 已忽略它。前者应保持测试无网络前提，后者可在明确客户端与协议退出边界后删除内部传递。
Linux 使用执行替身仅验证编译，不执行测试，也不构成数据库或 Broker 验收。

第二批在 main `308a0d966529` 上：全仓默认测试（452 个包）、相关调用链测试、race、Linux 全仓集成标签编译与分层边界检查通过；
标准 lint 与当前 main 的 30 条诊断对比为 18 条，未新增诊断。共享重放逻辑与提取前实现逐段比较一致，
两种数据库的事务文件没有改动。文档事实检查确认 165 篇文档与 131 个 gRPC RPC。
第一批的入口、函数和重复扫描统计保留原审计基线，不作为新 main 的扫描总数。

上述结果属于清理阶段的本地验证。发布须分别确认 PR/main CI、目标 SHA 的部署和逐实例健康证据；
真实 MySQL/Mongo/NSQ 故障专项与生产兼容窗口未在清理阶段重放，本地通过不替代这些验收。
