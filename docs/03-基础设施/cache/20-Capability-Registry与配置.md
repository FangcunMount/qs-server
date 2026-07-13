# Capability Registry 与配置

## 1. 结论

Capability 是 Cache 的最小治理单位。apiserver 的 [`catalog.Spec`](../../../internal/apiserver/cache/catalog/catalog.go) 同时声明 identity、owner、kind、layer、family、配置路径、行为默认值与 legacy metric label；[`cache.Registry`](../../../internal/pkg/cache/registry.go) 发布进程内唯一的 effective Policy snapshot。

业务 adapter 不再保存静态 Policy，也不读取 Options。每次操作开始时以固定 capability ID 调用 `PolicyProvider.Resolve`，从而让在线行为、status、reload 和观测投影共享同一事实源。

## 2. Canonical capability registry

### 2.1 apiserver

下表的“代码默认 TTL”来自 `Spec.Defaults`，“生产 TTL”来自 [`configs/apiserver.prod.yaml`](../../../configs/apiserver.prod.yaml)。生产配置是 override，并不改变代码默认值。

| Capability | Owner | Kind | Layer | Family | 代码默认 TTL | 生产 TTL | Metric label |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `survey.questionnaire` | survey | cache | L2 | `static_meta` | 12h | 2h | `questionnaire` |
| `modelcatalog.published_model` | modelcatalog | cache | L2 | `static_meta` | 24h | 2h | `published_model` |
| `evaluation.assessment_detail` | evaluation | cache | L2 | `object_view` | 2h | 1h | `assessment_detail` |
| `evaluation.assessment_list` | evaluation | cache | L1+L2 | `query_result` | 10m | 10m | `assessment_list` |
| `actor.testee` | actor | cache | L2 | `object_view` | 30m | 30m | `testee` |
| `plan.detail` | plan | cache | L2 | `object_view` | 2h | 12h | `plan` |
| `statistics.query` | statistics | cache | L2 | `query_result` | 5m | 15m | `stats_query` |
| `report_status` | interpretation | operational_state | runtime | `ops_runtime` | 48h | 48h | `report_status` |

`evaluation.assessment_list` 和 `statistics.query` 的 version token 使用 `meta_hotset`，但 capability family 仍分别投影为 `query_result`；version token 是查询缓存的支撑元数据，不是第二个业务 capability。

### 2.2 collection-server

collection-server 的 Registry 是静态 snapshot，能力由 [`internal/collection-server/cache/subsystem.go`](../../../internal/collection-server/cache/subsystem.go) 构造：

| Capability | Layer | Family | 生产配置 | 回源 |
| --- | --- | --- | --- | --- |
| `catalog.questionnaire` | L1 | `local` | TTL 180s、max 256、singleflight、signal evict | apiserver questionnaire gRPC |
| `catalog.typology` | L1 | `local` | TTL 180s、max 256、singleflight、signal evict | assessment-model catalog gRPC |
| `report_status` | runtime | `ops_runtime` | TTL 172800s | report workflow |

collection 的 capability ID 不跟随 apiserver 的业务前缀重命名，因为它们描述的是 BFF 自己持有的 DTO L1 和生命周期。

## 3. Spec 是什么

`Spec` 是代码级静态目录：

```go
type Spec struct {
    ID          cache.Capability
    Owner       string
    Kind        cache.CapabilityKind
    Layer       cache.Layer
    Family      cachemodel.Family
    ConfigPath  string
    MetricLabel string
    Defaults    cache.Policy
}
```

它保存“这个能力是谁、走哪一层、没有配置时怎样工作”，不保存 Redis client、Repository、loader、codec、signal watcher 或 warmup executor。后者由业务 adapter 和 subsystem 显式装配。

`MetricLabel` 暂时保留迁移前的低基数 `policy` label。Registry ID 已按业务 owner canonicalize，但 Prometheus 时间序列没有同时改名；治理投影负责把两者连接起来。

## 4. Policy 模型与继承

共享 [`cache.Policy`](../../../internal/pkg/cache/policy.go) 包含：

```text
TTL
NegativeTTL
Negative       = inherit | enabled | disabled
Compress       = inherit | enabled | disabled
Singleflight   = inherit | enabled | disabled
JitterRatio
```

Policy 合并顺序固定为：

```text
capability override
→ family defaults
→ global defaults
→ Spec.Defaults
```

对应代码：

```go
effective := override.MergeWith(
    familyDefault.MergeWith(
        globalDefault.MergeWith(specDefault),
    ),
)
```

越靠前优先级越高。最终 effective Policy 的三态开关不应残留 `inherit`；`status.effective_registry` 同时保留 `spec_default / global_default / family_default / override / effective`，便于解释一个值为什么生效。

`JitterRatio` 当前只把 TTL 延长随机的 `[0, ttl*ratio]`，不会向下缩短 TTL。现有 entry 的 expiry 在写入时确定，Policy 变化不会追溯修改。

## 5. Registry snapshot

[`cache.Registry`](../../../internal/pkg/cache/registry.go) 使用 `atomic.Pointer[RegistrySnapshot]`：

- 初始 snapshot version 为 1；
- capability 按 ID 排序；
- `Resolve/All/Snapshot` 返回值或 slice 副本；
- publisher 以 `expected_version` 做 CAS；
- candidate 与当前内容完全相同时 `changed=false`，version 不增加；
- 发布成功且确有变化时 version 加一；
- version conflict 或 candidate 校验失败时保留完整旧 snapshot。

这种模型保证并发读只能看到完整的旧版本或完整的新版本，不会在一次操作内混用两套 Policy。

## 6. 配置合同

### 6.1 apiserver

```yaml
cache:
  capabilities:
    survey:
      questionnaire: {}
    modelcatalog:
      published_model: {}
    evaluation:
      assessment_detail: {}
      assessment_list: {}
    actor:
      testee: {}
    plan:
      detail: {}
    statistics:
      query: {}
    report_status:
      ttl_seconds: 172800
  defaults:
    compress_payload: false
    ttl_jitter_ratio: 0.2
    static: {}
    object: {}
    query: {}
  governance: {}
```

普通 capability 可配置：

```text
enabled
ttl
negative_ttl
ttl_jitter_ratio
compress
singleflight
negative
```

`redis_runtime` 只配置 family → Redis profile/namespace/fallback/availability；它不能出现 TTL、negative、compression 或 singleflight。`report_status` 只使用 operational-state TTL，不继承普通 cache policy。

### 6.2 collection-server 与 worker

- collection-server 使用 `cache.capabilities.catalog.questionnaire`、`catalog.typology` 和 `report_status`；
- worker 只消费 `cache.capabilities.report_status`；
- 三进程 production `report_status.ttl_seconds` 都是 `172800`，由 config contract test 防止漂移；
- signal 的 prefix/channel/buffer 属于 `signaling.redis`，不与 report status TTL 混放。

未知 capability 或未知字段由 raw-settings validator 拒绝。仓库只维护当前 schema，不双读已经删除的旧字段。

## 7. apiserver 动态 reload

动态 reload 复用 system-governance action：

```http
POST /internal/v1/system-governance/actions/cache.reload_policy/runs
```

```json
{
  "confirm": true,
  "input": {
    "expected_version": 1
  }
}
```

动作只允许 `qs:admin`，要求显式确认和正整数 `expected_version`。process 使用新的 Viper 实例重读启动配置源，执行 unknown-field 和 typed validation，再构造完整 candidate Registry；它不会修改当前 Options 或全局 Viper。

可 reload：

- 七个普通 capability 的 `ttl/negative_ttl/ttl_jitter_ratio/compress/singleflight/negative`；
- global 与 static/object/query family defaults 中相同的 Policy 维度。

不可 reload：

- `enabled`；
- capability `family/layer/kind/owner/source/metric label`；
- `cache.governance`；
- `report_status`；
- collection-server Registry。

成功 reload 只影响后续操作和新写入。关闭 compression 后旧 gzip payload 仍由 decoder 自动识别；开启后只压缩新 payload。singleflight/negative 的变化从下一次操作开始生效。reload 不创建或拆除 decorator，也不扫描或删除旧 entry。

## 8. Status 投影

Cache governance status 在原有 runtime/warmup 字段外提供：

```json
{
  "effective_registry": {
    "snapshot_version": 2,
    "catalog_version": "v2",
    "generated_at": "...",
    "capabilities": [],
    "reload": {
      "last_attempt_at": "...",
      "last_success_at": "...",
      "last_failure_at": "...",
      "last_error": ""
    }
  }
}
```

`source` 必须与真实配置路径一致。后台页面和运维诊断应展示 effective 值，不应仅展示 YAML override；否则会遗漏 family/global/default 的继承结果。

## 9. 新 capability 的登记规则

新增普通 capability 必须在同一变更中声明：

- canonical ID、owner、kind、layer、family；
- `Spec.Defaults` 与配置 source；
- legacy/new metric label 策略；
- key、payload/codec、loader；
- negative、singleflight、TTL/jitter 语义；
- 失效、预热和 degraded 行为；
- Registry、配置、adapter 与 architecture contract test。

只有配置、没有 runtime consumer 的条目不得进入 production YAML。
