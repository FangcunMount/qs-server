# Capability Registry 与配置

## 1. 结论

Capability 是 Cache 的最小治理单位。apiserver 的 [`catalog.Spec`](../../../internal/apiserver/cache/catalog/catalog.go) 同时声明 identity、owner、kind、layer、family、配置路径、行为默认值与 legacy metric label；[`cache.Registry`](../../../internal/pkg/cache/registry.go) 发布进程内唯一的 effective Policy snapshot。

业务 adapter 不再保存静态 Policy，也不读取 Options。每次操作开始时以固定 capability ID 调用 `PolicyProvider.Resolve`，从而让在线行为、status、reload 和观测投影共享同一事实源。

## 2. Canonical capability registry

### 2.1 apiserver

下表的“代码默认 TTL”来自 `Spec.Defaults`，“生产 TTL”来自 [`configs/cache/apiserver.prod.yaml`](../../../configs/cache/apiserver.prod.yaml)。生产 policy 是 override，并不改变代码默认值。

| Capability | Owner | Kind | Layer | Family | 代码默认 TTL | 生产 TTL | Metric label |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `survey.questionnaire` | survey | cache | L2 | `static_meta` | 12h | 2h | `questionnaire` |
| `modelcatalog.published_model` | modelcatalog | cache | L1+L2 | `static_meta` | 24h | 2h | `published_model` |
| `evaluation.assessment_access` | evaluation | cache | L2 | `object_view` | 5m | 5m | `assessment_access` |
| `evaluation.assessment_detail` | evaluation | cache | L2 | `object_view` | 2h | 1h | `assessment_detail` |
| `actor.testee` | actor | cache | L2 | `object_view` | 30m | 30m | `testee` |
| `plan.detail` | plan | cache | L2 | `object_view` | 2h | 12h | `plan` |
| `statistics.query` | statistics | cache | L2 + bounded L1 stale | `query_result` | 26h | 26h | `stats_query` |
| `report_status` | interpretation | operational_state | runtime | `ops_runtime` | 48h | 48h | `report_status` |

`modelcatalog.published_model` 只有不可变的 `exact_by_ref` 运行快照进入 apiserver 进程内 L1，命中后直接复用已解码的 `DefinitionV2`；latest-by-code、questionnaire/list/algorithms 等可变目录桶仍使用 L2。Active admission 继续绕过两级缓存读取 Mongo，以重新校验 release status。L1 有界为 512 条，TTL 沿用 capability 的 effective TTL，并通过 `qs_apiserver_l1_cache_*` 指标暴露 hit/miss、entries 与 eviction。

`statistics.query` 的 generation/hotset 元数据使用 `meta_hotset`，但 capability family 仍投影为 `query_result`；支撑元数据不是第二个业务 capability。`evaluation.assessment_list` 因读路径从未接入缓存而已退役，system-governance 不保留虚假的 disabled row。

### 2.2 collection-server

collection-server 的 Registry 是静态 snapshot，能力由 [`internal/collection-server/cache/subsystem.go`](../../../internal/collection-server/cache/subsystem.go) 构造：

| Capability | Layer | Family | 生产配置 | 回源 |
| --- | --- | --- | --- | --- |
| `catalog.questionnaire` | L1 | `local` | TTL 180s、max 256、singleflight、signal evict | apiserver questionnaire gRPC |
| `catalog.published_model` | L1 | `local` | 生产已启用；TTL 180s、jitter 0.2、每 bucket max 64、singleflight、signal evict | apiserver published-model capability；exact version 为 L1+L2，其余目录桶为 L2 |
| `catalog.typology` | L1 | `local` | TTL 180s、max 256、singleflight、signal evict | assessment-model catalog gRPC |
| `evaluation.assessment_access` | L1 | `local` | 生产已启用；TTL 60s、jitter 0.2、max 1024、singleflight；仅正向 ownership token | apiserver assessment-access L2 |
| `evaluation.assessment_detail` | L1 | `local` | 生产已启用；TTL 180s、jitter 0.2、max 256、singleflight；仅 `evaluated` DTO | apiserver assessment-detail L2 |
| `report_status` | runtime | `ops_runtime` | TTL 172800s | report workflow |

collection catalog capability 使用 consumer-owned `catalog.*` ID；evaluation 两个意图保持业务名称一致。collection L1 与 apiserver cache 仍是不同进程 Registry 里的独立 entry、policy 和生命周期；apiserver 内部的 immutable exact-by-ref L1 不改变这一跨进程边界。

published-model 与 evaluation access/detail L1 在 dev/prod policy 中均已启用；主配置只引用独立 policy：

```yaml
cache:
  policy_file: cache/collection-server.prod.yaml
```

实际 capability 值位于 [`configs/cache/collection-server.prod.yaml`](../../../configs/cache/collection-server.prod.yaml)。

启用或回滚 `enabled` 都需要滚动重启 collection-server；当前不支持动态重接线。

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
- snapshot 同时原子保存 capability 集合与 `PolicySource(component/schema_version/path/policy_sha256)`；
- 发布成功且确有变化时 version 加一；
- version conflict 或 candidate 校验失败时保留完整旧 snapshot。

这种模型保证并发读只能看到完整的旧版本或完整的新版本，不会在一次操作内混用两套 Policy。

## 6. 配置合同

### 6.1 主配置与 policy 文件

```yaml
cache:
  policy_file: cache/apiserver.prod.yaml

runtime_state:
  report_status:
    ttl_seconds: 172800
```

apiserver 与 collection-server 各自维护 dev/prod policy：

```text
configs/cache/apiserver.{dev,prod}.yaml
configs/cache/collection-server.{dev,prod}.yaml
```

policy envelope 固定为 `version: "1.0"`，`component` 必须与进程匹配；capability 集合必须完整、不得包含未知项，每个 capability 必须显式提供 `enabled`。collection policy 还必须显式提供 TTL、容量、singleflight 和 Signal eviction。路径相对主配置文件目录解析，不依赖进程工作目录。

优先级固定为：代码默认值 `<` policy 文件 `<` 环境变量 `<` 显式 CLI。未显式传入的 flag default 不覆盖 policy。完成 schema 校验和全部覆盖后，规范化 effective policy JSON 生成 SHA-256；注释、YAML 键顺序和格式变化不会改变 hash。

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

`redis_runtime` 只配置 family → Redis profile/namespace/fallback/availability；它不能出现 TTL、negative、compression 或 singleflight。`report_status` 位于 `runtime_state.report_status`，只使用 operational-state TTL，不继承普通 cache policy，也不参与 cache policy reload。

发布时，cache policy 独立化与 `report_status` 归位作为同一个原子配置/镜像切换直接迁移，不设置额外观察窗口；回滚同样必须同时恢复匹配的配置与镜像。

### 6.2 collection-server 与 worker

- collection-server 的 policy 使用 `capabilities.catalog.questionnaire`、`catalog.published_model` 与 `catalog.typology`；`catalog.published_model` 的启停是启动时接线，修改后必须滚动重启；
- worker 没有 `CacheOptions` 或 `cache:` 配置段，只消费 `runtime_state.report_status`；
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

动作只允许 `qs:admin`，要求显式确认和正整数 `expected_version`。process 只重读启动时已经固定真实路径的 apiserver cache policy 文件，再按启动时相同的默认值、环境变量和显式 CLI 合并链构造 candidate；它不会重读主配置、切换 policy 文件、修改当前 Options 或全局 Viper。

可 reload：

- 七个普通 capability 的 `ttl/negative_ttl/ttl_jitter_ratio/compress/singleflight/negative`；
- global 与 static/object/query family defaults 中相同的 Policy 维度。

不可 reload：

- `enabled`；
- capability `family/layer/kind/owner/source/metric label`；
- policy 文件中的 `governance`；
- `report_status`；
- collection-server Registry。

成功 reload 只影响后续操作和新写入。关闭 compression 后旧 gzip payload 仍由 decoder 自动识别；开启后只压缩新 payload。singleflight/negative 的变化从下一次操作开始生效。reload 不创建或拆除 decorator，也不扫描或删除旧 entry。

## 8. Status 投影

Cache governance status 在原有 runtime/warmup 字段外提供：

```json
{
  "effective_registry": {
    "snapshot_version": 2,
    "catalog_version": "v3",
    "generated_at": "...",
    "policy_source": {
      "component": "qs-apiserver",
      "schema_version": "1.0",
      "path": "/app/configs/cache/apiserver.prod.yaml",
      "policy_sha256": "..."
    },
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
