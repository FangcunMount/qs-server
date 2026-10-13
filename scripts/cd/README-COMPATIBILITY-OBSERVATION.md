# 兼容契约退役观测

本工具只读检查调用和采样证据，不删除契约或数据。四项新计量已准备源码；生产部署、连续观察与客户端／数据负责人退出确认仍需分别完成，14 天窗口尚未开始。五个已退役指标退出清单，现用 legacy admission 指标保留为补充证据；该汇总不证明各Worker源码或结清待处理／人工可重放责任。

## 四项计量

| 契约 | 指标 | 计数位置与语义 |
| --- | --- | --- |
| 已验签 Claims.Extra 身份别名 | `qs_auth_claim_source_total{field,source}` | REST 验证成功后，只计 UserID、AccountID、OrgID 真正选中的来源；未选中的 Extra 不算依赖。AccountID 继续优先 Extra，UserID／OrgID 继续优先 canonical；OrgID 合法可选，已验签且未选出 Org 的原分支计为 missing。 |
| Authorization 裸 token | `qs_auth_token_source_total{source,result}` | 区分 bearer、bare、query、cookie、missing、unavailable，accepted 仅表示认证验证成功，rejected 不计已选身份声明；下游授权结果另计。query／cookie 保留原语义。 |
| Collection 旧 Task token 路由 | `qs_plan_entry_route_total{route,result}` | 在原 IAM 链之前观察已匹配的两个固定 GET 模板，包含认证拒绝。canonical／legacy_token 均继续执行原鉴权，2xx 为 accepted，其余为 rejected。 |
| Definition 写入的衍生字段 | `qs_model_definition_write_total{input,result}`、`qs_model_definition_input_field_total{field,presence,result}` | 仅 REST UpdateDefinition 读取原 binder 消费的字节；两个固定字段 DecisionSpec／InterpretationAssets 的 presence 包括 null、空对象和 binder 接受的大小写形式。嵌套同名键不计；canonical 表示两个字段均未提供。MaterializeLayers、共享 domain JSON 解码和 Mongo 读取不产生计数。 |

以上固定组合均注册零样本，标签没有用户、Task、路径、token、claims 或模型正文。三个 `*_compatibility_observation_info` Gauge 绑定 capability=1 和程序的编译 source_sha；仅有源码或零值 series 不证明路径已接收业务流量。

共享 REST middleware 的实际消费者是 APIServer 和 Collection。当前 prod／dev 的 gRPC `auth.enabled=false`，内部 RPC 走 mTLS／ACL；本批没有新挂载 JWT 拦截器，也没有改变 service_id／username 扩展。可选 gRPC JWT 代码的外部配置消费者需另行确认，本页的 REST 观测不为它签发零调用证明。

## 预检查与窗口检查

`scripts/cd/audit-compatibility-observation.sh` 默认 `OBSERVATION_MODE=preflight`。执行前显式给出 `EXPECTED_SOURCE_SHA` 的完整已审 SHA，并确认 `EXPECTED_APISERVER_CONTAINERS` 和 `EXPECTED_COLLECTION_CONTAINERS` 清单；默认是 qs-apiserver 与 qs-collection-server-1、qs-collection-server-2。清单变化需要重新核对实际部署，不能少列实例来绕过检查。

预检查将运行容器的 scrape labels、网络 IP／端口与 Prometheus targets 唯一匹配，要求全部实例 up、样本新鲜、编译 source 能力和所有固定零值组合齐全。成功也只输出 `observation_window=not_started`，不启动计时，不为四项新契约输出零调用候选。

授权部署、正式客户端和完整采样后，登记批准的UTC `OBSERVATION_STARTED_AT`（例如 `2026-11-01T00:00:00Z`，示例不是实际T0）。`WINDOW_DAYS`绑定W=max(14天、相关最长有效token周期、受支持消息重投周期)。`OBSERVATION_MODE=window`要求T0后已完整经过W日；审核逻辑窗口为固定 [T0,E]，E 在本次审计开始时捕获，历史读取耗时不推动 E；不截成最近14或30天。人工可重放责任单独结清。

预检查也要求Prometheus实际保留时间至少max(30天、W+7天)，但不读取历史range/increase、不启动窗口。窗口审核使用带显式 `time` 的 `/query` 原始 range vector，不使用 `query_range` 的网格重采样或 lookback 填点。每块最多六小时并保留相邻重叠与跨块状态，内存只保留本块与有界接缝证据；历史审计耗时不属于数据库维护窗口。检查要求：

- 当前部署清单、编译 source 与全部固定标签组合相符；实例必须早于基线启动。每个实例使用自己的实际 scrape interval，支持 Prometheus 的 `1m` 等 duration；interval 在15–60秒内，timeout小于该 interval，当前 freshness 按当前读取时间判断。
- 固定 interval 的 phase 必须在整个窗口保持稳定。容忍的抖动 J=min(1秒、interval/10)，要求每个连续实际 scrape 对应恰好一个 phase slot；丢失、额外抓取、interval/phase变化或超出 J 都使窗口失效，应重新核对配置和批准 T0。此门槛不声称所有生产 Prometheus 抖动都满足条件，也不从观察到的最大 gap 学习容忍值。
- 从 T0−2×interval−J 起读取至少两条 T0 前实际成功抓取作为基线。完整 source／所有固定 counter 子序列必须与每条真实 `up` 时间戳在毫秒精度一一对应；完整标签集跨块不变，任何缺点、down、未知标签、来源切换、非有限值或 counter reset 均失败。单个 `up` 或子序列缺样不能靠旧值插值补齐。
- 历史读取后再固定独立 closing snapshot C，要求每个实例及每条子序列都有第一条 E 后实际成功抓取，闭合窗口尾部；没有该样本则 incomplete。历史审计过长、使 closing 读取超过六小时有界块时同样 incomplete；不扩大查询范围冒充完整证据。C 不改变逻辑 E。基线或跨 E 边界的兼容增长保守保持 `active_compatibility`，并输出 `boundary_ambiguous`；这只排除零候选，不证明命中必定发生在 [T0,E] 内。
- 每个实例都有 accepted bearer、UserID 与 AccountID canonical 来源的真实正流量，以及各自服务的 TaskID 路由／无衍生字段 Definition 写入。只接受两个端点均位于 [T0,E] 的 counter 正增量；仅首条窗内抓取相对 T0 前基线的 canonical 增长仍属证据不足，E 后操作不补充窗内流量。OrgID 合法可选：原逻辑已验签选出的 canonical 或 missing 来源可满足正流量。用户或登录身份缺失、Org两者都零、只在基线增长均失败。
- Extra 真实选中、bare 请求（含拒绝）、旧 Task 路由（含拒绝）、带衍生字段写入（含拒绝）任一出现，保留 `active_compatibility`。Definition 写入或固定字段 presence 的 `unreadable` 增长单独记为 `unknown_input` 并使审核失败：无法证明该请求未携带旧字段，也不能把损坏 JSON 推断成旧客户端命中。四项全零且无上述缺口也只得到候选，仍需客户端、数据负责人和业务退出确认。

`active_compatibility` 表示成功读取到了仍应保留的兼容证据，脚本 exit 0 不代表退役验收通过；只有四项均为 `zero_window_candidate` 才可进入独立退出审批。现用 legacy admission 汇总读取也固定到 E，但仍是补充证据，不能代替 Worker 实例、持续采样或未结责任验收。

IAM 当前新会话使用空 BusinessContext，OrgID 为零时 JWT 的 `org_id,omitempty` 不输出组织声明。因此正常 fresh 身份流量可以全部记为 Org missing，无需为观察门禁新增组织 claim。Org missing 只证明已验签后的来源选择被观察到，不证明业务组织授权、权限范围或实际业务流程验收；旧 `Extra.org_id`／`Extra.organization_id` 真实选中仍属于 `active_compatibility`，不计入上述合法 Org 正流量，也不能成为零调用候选。

工作流沿用现有固定SSH初始化、ServerA身份核对和非交互sudo；密钥与sudo密码不输出。人工调用明确提供mode/source/W/T0。每天北京02:00的调用使用production环境中已批准的 `QS_COMPAT_OBSERVATION_MODE`、`QS_COMPAT_SOURCE_SHA`、`QS_COMPAT_WINDOW_DAYS`、`QS_COMPAT_STARTED_AT`、`QS_COMPAT_APISERVER_CONTAINERS`、`QS_COMPAT_COLLECTION_CONTAINERS`；实例清单也由人工调用沿用这两个配置，缺失时失败。不从观测到的SHA自动批准自身，不更改上述环境变量或保留配置。未批准T0时只允许默认预检查，不能启动观察计时。

## 验证入口

- `go test -race ./internal/pkg/authobservability ./internal/pkg/middleware ./internal/collection-server/transport/rest ./internal/apiserver/transport/rest/handler`：真实签名／SDK 验证、实际选中来源、载体优先级、认证拒绝、旧路由顺序、输入 presence 与原 body／错误行为。
- `bash scripts/cd/test-compatibility-observation.sh`：一次完整14日合成 raw 窗口、预检查和实例／source／重启／配置失败关闭检查；当前 target 在固定 E 后读取仍新鲜，历史查询保持显式时间。另调用 `test-compatibility-raw-window.py` 的短序列矩阵保护首窗命中、闭合尾部、真实漏点、重置、标签／source变更、混合 interval、合法有界 jitter、跨块接缝、canonical／可选 Org 与 unreadable 分类。合成矩阵不证明生产已有历史样本。
- `M6_QUERY_ARTIFACT=/tmp/qs-compat-queries.jsonl bash scripts/cd/test-compatibility-observation.sh` 后运行 `python3 scripts/cd/test-compatibility-prometheus.py /tmp/qs-compat-queries.jsonl`：在隔离原生Prometheus解析／执行实际表达式，并用三个实际 HTTP scrape targets 与 Prometheus 生成的 `up` 验证 raw 时间戳、首窗 baseline 命中、真实缺 counter／up 样本、source／标签／reset拒绝和 E 后闭合。原生测试使用1秒 interval的短序列加速反例，不更改生产15–60秒门槛，不冒充14天生产历史。

PromQL 的矩阵与 timestamp／counter 函数语义依据 [Prometheus HTTP API](https://prometheus.io/docs/prometheus/latest/querying/api/) 和 [range selector 边界](https://prometheus.io/docs/prometheus/latest/querying/basics/#range-vector-selectors)；原生parser/API验证与生产采样覆盖是不同证据。
