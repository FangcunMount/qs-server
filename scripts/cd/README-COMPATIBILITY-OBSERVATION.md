# 兼容契约退役观测

本工具只读检查调用和采样证据，不删除契约或数据。四项新计量已准备源码；生产部署、连续观察与客户端／数据负责人退出确认仍需分别完成，14 天窗口尚未开始。五个已退役指标退出清单，现用 legacy admission 指标保留为补充证据；该汇总不证明各Worker源码或结清待处理／人工可重放责任。

## 四项计量

| 契约 | 指标 | 计数位置与语义 |
| --- | --- | --- |
| 已验签 Claims.Extra 身份别名 | `qs_auth_claim_source_total{field,source}` | REST 验证成功后，只计 UserID、AccountID、OrgID 真正选中的来源；未选中的 Extra 不算依赖。AccountID 继续优先 Extra，UserID／OrgID 继续优先 canonical。 |
| Authorization 裸 token | `qs_auth_token_source_total{source,result}` | 区分 bearer、bare、query、cookie、missing、unavailable，accepted 仅表示认证验证成功，rejected 不计已选身份声明；下游授权结果另计。query／cookie 保留原语义。 |
| Collection 旧 Task token 路由 | `qs_plan_entry_route_total{route,result}` | 在原 IAM 链之前观察已匹配的两个固定 GET 模板，包含认证拒绝。canonical／legacy_token 均继续执行原鉴权，2xx 为 accepted，其余为 rejected。 |
| Definition 写入的衍生字段 | `qs_model_definition_write_total{input,result}`、`qs_model_definition_input_field_total{field,presence,result}` | 仅 REST UpdateDefinition 读取原 binder 消费的字节；两个固定字段 DecisionSpec／InterpretationAssets 的 presence 包括 null、空对象和 binder 接受的大小写形式。嵌套同名键不计；canonical 表示两个字段均未提供。MaterializeLayers、共享 domain JSON 解码和 Mongo 读取不产生计数。 |

以上固定组合均注册零样本，标签没有用户、Task、路径、token、claims 或模型正文。三个 `*_compatibility_observation_info` Gauge 绑定 capability=1 和程序的编译 source_sha；仅有源码或零值 series 不证明路径已接收业务流量。

共享 REST middleware 的实际消费者是 APIServer 和 Collection。当前 prod／dev 的 gRPC `auth.enabled=false`，内部 RPC 走 mTLS／ACL；本批没有新挂载 JWT 拦截器，也没有改变 service_id／username 扩展。可选 gRPC JWT 代码的外部配置消费者需另行确认，本页的 REST 观测不为它签发零调用证明。

## 预检查与窗口检查

`scripts/cd/audit-compatibility-observation.sh` 默认 `OBSERVATION_MODE=preflight`。执行前显式给出 `EXPECTED_SOURCE_SHA` 的完整已审 SHA，并确认 `EXPECTED_APISERVER_CONTAINERS` 和 `EXPECTED_COLLECTION_CONTAINERS` 清单；默认是 qs-apiserver 与 qs-collection-server-1、qs-collection-server-2。清单变化需要重新核对实际部署，不能少列实例来绕过检查。

预检查将运行容器的 scrape labels、网络 IP／端口与 Prometheus targets 唯一匹配，要求全部实例 up、样本新鲜、编译 source 能力和所有固定零值组合齐全。成功也只输出 `observation_window=not_started`，不启动计时，不为四项新契约输出零调用候选。

授权部署、正式客户端和完整采样后，登记批准的UTC `OBSERVATION_STARTED_AT`（例如 `2026-11-01T00:00:00Z`，示例不是实际T0）。`WINDOW_DAYS`绑定W=max(14天、相关最长有效token周期、受支持消息重投周期)。`OBSERVATION_MODE=window`要求T0后已完整经过W日；审核从T0到当前的全部采样，不截成最近14或30天。人工可重放责任单独结清。

预检查也要求Prometheus实际保留时间至少max(30天、W+7天)，但不读取历史range/increase、不启动窗口。每个range请求最多六小时，保留15–60秒scrape步长，避免Prometheus单series查询点上限。检查要求：

- 当前部署清单和全部 source SHA 相符；实例未在窗口内部署或重启。
- 每个实际 scrape interval 的查询点均覆盖全部实例、source 能力与固定 series；样本 age 小于两个 interval。支持 15–60 秒 interval；缺点、down、陈旧样本、来源切换、部分部署或 counter reset 均失败，不能视作零。
- 每个实例都有实际 accepted canonical 流量：bearer、三个身份字段的 canonical 来源，以及各自服务的 TaskID 路由／无衍生字段 Definition 写入。零流量不是兼容退出证据。
- Extra 真实选中、bare 请求（含拒绝）、旧 Task 路由（含拒绝）、带衍生字段写入（含拒绝）任一出现，保留 `active_compatibility`。四项全零也只得到候选，仍需客户端、数据负责人和业务退出确认。

工作流沿用现有固定SSH初始化、ServerA身份核对和非交互sudo；密钥与sudo密码不输出。人工调用明确提供mode/source/W/T0。每天北京02:00的调用使用production环境中已批准的 `QS_COMPAT_OBSERVATION_MODE`、`QS_COMPAT_SOURCE_SHA`、`QS_COMPAT_WINDOW_DAYS`、`QS_COMPAT_STARTED_AT`、`QS_COMPAT_APISERVER_CONTAINERS`、`QS_COMPAT_COLLECTION_CONTAINERS`；实例清单也由人工调用沿用这两个配置，缺失时失败。不从观测到的SHA自动批准自身，不更改上述环境变量或保留配置。未批准T0时只允许默认预检查，不能启动观察计时。

## 验证入口

- `go test -race ./internal/pkg/authobservability ./internal/pkg/middleware ./internal/collection-server/transport/rest ./internal/apiserver/transport/rest/handler`：真实签名／SDK 验证、实际选中来源、载体优先级、认证拒绝、旧路由顺序、输入 presence 与原 body／错误行为。
- `bash scripts/cd/test-compatibility-observation.sh`：完整窗口、真实命中及实例遗漏、版本不符、重启、reset、断档、无 canonical 流量等失败关闭矩阵。合成矩阵不证明生产已有历史样本。
- `M6_QUERY_ARTIFACT=/tmp/qs-compat-queries.jsonl bash scripts/cd/test-compatibility-observation.sh` 后运行 `python3 scripts/cd/test-compatibility-prometheus.py /tmp/qs-compat-queries.jsonl`：隔离原生Prometheus实际解析／执行vector与range表达式，核对固定series和真实counter increase；不冒充14天生产历史。

PromQL 的矩阵与 timestamp／counter 函数语义依据 [Prometheus HTTP API](https://prometheus.io/docs/prometheus/latest/querying/api/) 和 [查询函数](https://prometheus.io/docs/prometheus/latest/querying/functions/)；原生parser/API验证与生产采样覆盖是不同证据。
