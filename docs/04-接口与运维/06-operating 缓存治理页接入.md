# Operating 缓存治理页接入

**本文回答**：这篇文档说明 operating 后台如果要接入 `qs-server` 的缓存治理能力，应该如何分工 `Grafana`、operating BFF 和 `qs-apiserver internal REST`，以及页面最小可用信息架构应该长什么样。

## 30 秒结论

如果只记一件事，记下面这句：

> **Grafana 负责时序趋势，operating 页面负责治理详情；浏览器不要直连 `qs-apiserver` internal API。**

推荐接入方式固定为：

- `operating-frontend` -> `operating-backend/BFF` -> `qs-apiserver internal REST`
- `Grafana` 继续通过 Prometheus 指标负责趋势、历史与告警
- operating 页只展示：
  - 当前 family 状态
  - 最近一次 warmup
  - hotset top-N 预览
  - 跳到 Grafana 的深链接

## 为什么不直接在 operating 页面里重画监控图

`qs-server` 当前已经把缓存治理拆成了两类数据面：

- **时序观测面**：Prometheus `/metrics`
- **当前状态面**：internal REST

如果 operating 页面自己去重做时序图，会重复建设：

- PromQL 聚合
- 时间范围、分桶、变量、告警联动
- worker 与 apiserver 的跨进程观测

因此 v1 固定分工为：

- **Grafana**：`qs_cache_*`、`qs_query_cache_version_total`、`qs_cache_lock_*`
- **Operating 页面**：`/internal/v1/cache/governance/status`、`/internal/v1/cache/governance/hotset`

## BFF 接口建议

Operating BFF 建议暴露 4 个页面专用接口：

- `GET /ops/api/cache-governance/status`
- `GET /ops/api/cache-governance/hotset?kind=...&limit=...`
- `POST /ops/api/cache-governance/warmup-targets`
- `GET /ops/api/cache-governance/links`

职责固定为：

- `status`
  - 调 `GET /internal/v1/cache/governance/status`
  - 透传并做轻量整形
- `hotset`
  - 调 `GET /internal/v1/cache/governance/hotset`
  - 白名单校验 `kind`
  - 限制 `limit <= 100`
- `warmup-targets`
  - 调 `POST /internal/v1/cache/governance/warmup-targets`
  - 由 BFF 做 `kind/scope/org` 白名单校验
  - 浏览器不直接拿 internal token
- `links`
  - 返回 Grafana 深链接

浏览器不应直接拿到 `qs-apiserver` internal 凭证。

## `qs-apiserver` 可直接复用的接口

### 1. `GET /internal/v1/cache/governance/status`

当前响应已适合 UI/BFF 直接消费，包含：

- `generated_at`
- `summary`
  - `family_total`
  - `available_count`
  - `degraded_count`
  - `unavailable_count`
  - `ready`
- `families[]`
  - `family`
  - `profile`
  - `namespace`
  - `allow_warmup`
  - `configured`
  - `available`
  - `degraded`
  - `mode`
  - `last_error`
  - `last_success_at`
  - `last_failure_at`
  - `consecutive_failures`
  - `updated_at`
- `warmup`
  - `enabled`
  - `startup`
  - `hotset`
  - `latest_runs[]`

### 2. `GET /internal/v1/cache/governance/hotset?kind=...&limit=...`

当前响应已包含：

- `family`
- `kind`
- `limit`
- `available`
- `degraded`
- `message`
- `items[]`
  - `scope`
  - `score`

`kind` 当前候选值以缓存治理代码注册的 warmup kind 为准，页面建议固定白名单：

- `static.scale`
- `static.questionnaire`
- `static.scale_list`
- `query.stats_system`
- `query.stats_questionnaire`
- `query.stats_plan`

kind、scope、family 的现行模型以 [internal/apiserver/cachetarget/target.go](../../internal/apiserver/cachetarget/target.go) 为准，不要在 BFF 中维护第二套含义不同的解析规则。

| Kind | Scope 格式 | Family | 说明 |
| ---- | ---------- | ------ | ---- |
| `static.scale` | `scale:{code}` | `static_meta` | 单量表静态缓存 |
| `static.questionnaire` | `questionnaire:{code}` | `static_meta` | 问卷静态缓存 |
| `static.scale_list` | `published` | `static_meta` | 已发布量表列表 |
| `query.stats_system` | `org:{orgID}` | `query_result` | 机构级统计查询 |
| `query.stats_questionnaire` | `org:{orgID}:questionnaire:{code}` | `query_result` | 问卷统计查询 |
| `query.stats_plan` | `org:{orgID}:plan:{planID}` | `query_result` | 计划统计查询 |

## 页面最小信息架构

页面建议拆成 4 个区：

### 1. 全局摘要区

展示：

- `generated_at`
- family 总数
- degraded 数
- unavailable 数
- warmup 是否启用
- hotset 是否启用

同时放三个主按钮：

- `查看 Grafana 缓存总览`
- `查看 Grafana Worker 锁治理`
- `手工预热`

### 2. Family 状态表

建议列：

- `family`
- `profile`
- `namespace`
- `mode`
- `available`
- `degraded`
- `configured`
- `last_success_at`
- `last_failure_at`
- `consecutive_failures`
- `last_error`

### 3. Warmup 状态区

展示：

- `enabled`
- `startup.static`
- `startup.query`
- `hotset.enable`
- `hotset.top_n`
- `hotset.max_items_per_kind`

以及 `latest_runs[]` 表格。

### 4. Hotset 预览区

- `kind` 下拉框
- `limit=20`
- 表格列：`scope`、`score`
- 顶部显示 `available / degraded / message`

## 刷新与交互规则

建议交互规则固定为：

- `status`：每 `30s` 自动刷新
- `hotset`：首次进入、切换 `kind`、手动刷新时才拉取
- 顶部提供统一 `刷新` 按钮

错误态：

- `status` 失败：页面主体显示错误态，hotset 一并置灰
- `hotset` 失败：仅 hotset 卡片报错，不影响 family/warmup
- `meta_hotset` degraded：显示“热点预览可能不完整”

## Grafana 深链接建议

Operating 页面不直接嵌图，而是给出固定深链接：

- `overview`：缓存总体面板
- `family`：family availability / degraded
- `warmup`：warmup runs / items / duration
- `hotset`：hotset record / size
- `query_version`：version token error / bump
- `worker_lock`：worker lock degraded / contention

Grafana 侧只保留低基数变量，如：

- `component`
- `family`
- `policy`

不要把 `scope/userID/orgID/planID/questionnaireCode` 这类高基数对象放进 dashboard 变量。

## 权限与边界

Operating 页固定为“读为主、命令受控”的治理页：

- 不暴露 `repair-complete`
- 不暴露 invalidate / delete 之类破坏性缓存操作
- 允许暴露受控的 `手工预热`

## 手工预热接入方式

Operating 后台如需直接接入手工预热，推荐固定为：

- `operating-frontend`
  - 只负责选择目标、展示结果
- `operating-backend/BFF`
  - 负责把页面目标转换成 `warmup-targets` 请求
  - 负责带 internal token 调 `qs-apiserver`
- `qs-apiserver`
  - 执行真实 warmup，并返回逐 target 结果

浏览器不要直接调 `qs-apiserver /internal/v1/cache/governance/warmup-targets`。

### BFF 建议暴露的命令接口

- `POST /ops/api/cache-governance/warmup-targets`

请求体建议直接沿用 `qs-apiserver` 契约：

```json
{
  "targets": [
    { "kind": "static.scale", "scope": "scale:S-001" },
    { "kind": "static.questionnaire", "scope": "questionnaire:Q-001" },
    { "kind": "query.stats_system", "scope": "org:1" }
  ]
}
```

当前 `qs-apiserver` 支持的 `kind` 白名单：

- `static.scale`
- `static.questionnaire`
- `static.scale_list`
- `query.stats_system`
- `query.stats_questionnaire`
- `query.stats_plan`

### BFF 需要做的最小校验

- `targets` 不能为空
- `kind` 必须在白名单内
- `query.*` 的 `scope` 必须属于当前 operating 操作者所在机构；orgID 解析规则与 [cachetarget.WarmupTarget.OrgID](../../internal/apiserver/cachetarget/target.go) 保持一致
- `static.*` 允许跨机构视图触发，因为它们对应静态资源缓存

### 返回结果如何直接驱动页面

`qs-apiserver` 返回结构已经适合页面直接展示：

- `trigger`
- `started_at`
- `finished_at`
- `summary`
  - `target_count`
  - `ok_count`
  - `skipped_count`
  - `error_count`
  - `result`
- `items[]`
  - `family`
  - `kind`
  - `scope`
  - `status`
  - `message`

页面建议把 `items[]` 直接渲染成命令执行结果表，而不是只展示一个总状态。

worker 继续没有 internal 缓存治理面板；其 Redis 状态只能通过：

- Grafana
- `/metrics`
- `GET /governance/redis`
- 结构化日志

查看。
