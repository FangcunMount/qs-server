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

Operating BFF 建议只暴露 3 个页面专用接口：

- `GET /ops/api/cache-governance/status`
- `GET /ops/api/cache-governance/hotset?kind=...&limit=...`
- `GET /ops/api/cache-governance/links`

职责固定为：

- `status`
  - 调 `GET /internal/v1/cache/governance/status`
  - 透传并做轻量整形
- `hotset`
  - 调 `GET /internal/v1/cache/governance/hotset`
  - 白名单校验 `kind`
  - 限制 `limit <= 100`
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
  - `warmup_enabled`
  - `hotset_enabled`
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

同时放两个主按钮：

- `查看 Grafana 缓存总览`
- `查看 Grafana Worker 锁治理`

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

Operating 页固定为只读治理页：

- 不暴露 `repair-complete`
- 不暴露手动 warmup / invalidate
- 不暴露任何 mutate 动作

worker 继续没有 internal 面板；其缓存治理状态只能通过：

- Grafana
- `/metrics`
- 结构化日志

查看。
