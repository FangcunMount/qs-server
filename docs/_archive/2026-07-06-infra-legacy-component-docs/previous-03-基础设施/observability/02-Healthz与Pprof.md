# Healthz 与 Pprof

**本文回答**：qs-server 当前 HTTP 健康检查和 pprof 如何暴露；`/healthz` 的语义边界是什么；它和 Redis/Resilience governance status 有什么区别；pprof 适合排查哪些问题，生产环境应该如何限制。

---

## 30 秒结论

| 能力 | 当前行为 |
| ---- | -------- |
| `/healthz` | GenericAPIServer 在 Healthz=true 时注册，返回 `{"status":"ok"}` |
| 启动自检 | GenericAPIServer Run 会在 10 秒内反复 ping `/healthz`，成功后认为 router deployed |
| `/version` | 通用版本 endpoint |
| `/metrics` | EnableMetrics=true 时 gin-prometheus 挂载 |
| `/debug/pprof` | EnableProfiling=true 时注册 gin pprof |
| ready 语义 | 当前通用 `/healthz` 更偏 router/liveness，不等于所有依赖 ready |
| dependency status | Redis family、Resilience degraded 等应从 governance/status 入口看 |
| pprof 风险 | 生产访问必须受控，不能公网裸露 |

一句话概括：

> **/healthz 证明 HTTP router 活着，governance/status 解释依赖是否 degraded，pprof 用于受控性能剖析。**

---

## 1. GenericAPIServer Healthz

在 `InstallAPIs` 中：

```go
if s.healthz {
  s.GET("/healthz", ...)
}
```

响应：

```json
{
  "status": "ok"
}
```

默认 Config 中：

```text
Healthz = true
```

---

## 2. 启动自检

GenericAPIServer Run：

1. 启动 HTTP/HTTPS server。
2. 如果 healthz enabled，调用 `ping(ctx)`。
3. 10 秒内循环请求 `/healthz`。
4. 若 StatusCode == 200，记录 router deployed。
5. 超时则返回 error。

如果 bind address 是 `0.0.0.0`，ping 会改用：

```text
127.0.0.1:{port}/healthz
```

---

## 3. Healthz 边界

当前 `/healthz` 代表：

- HTTP router 可以响应。
- 进程 HTTP server 启动成功。
- 基础通用 server 存活。

它不代表：

- MySQL 可用。
- Mongo 可用。
- Redis family 全部 ready。
- MQ publisher 可用。
- IAM 可用。
- worker 消费正常。
- cache warmup 完成。
- event outbox 无积压。

这些要看具体 governance/status 或模块指标。

---

## 4. Readyz 设计建议

如果后续新增 `/readyz`，建议区分：

| Endpoint | 语义 |
| -------- | ---- |
| `/healthz` | liveness：进程和 HTTP router 是否活着 |
| `/readyz` | readiness：是否可以接收业务流量 |
| governance status | dependency/detail：哪些 family/保护点 degraded |

Readyz 不应做重操作。可以读取缓存的 snapshot，不应每次实时 ping 所有下游。

---

## 5. Pprof

当 EnableProfiling=true：

```go
ginpprof.Register(s.Engine, "/debug/pprof")
```

常用入口：

| 路径 | 用途 |
| ---- | ---- |
| `/debug/pprof/` | index |
| `/debug/pprof/profile` | CPU profile |
| `/debug/pprof/heap` | heap |
| `/debug/pprof/goroutine` | goroutine |
| `/debug/pprof/block` | block |
| `/debug/pprof/mutex` | mutex |

默认 Config 中：

```text
EnableProfiling = true
```

这在生产要特别注意访问控制。

---

## 6. Pprof 使用场景

| 现象 | 优先 pprof |
| ---- | ---------- |
| CPU high | `/debug/pprof/profile?seconds=30` |
| 内存持续增长 | heap profile |
| goroutine 数量异常 | goroutine profile |
| 锁竞争严重 | mutex profile |
| 阻塞严重 | block profile |
| 请求卡住 | goroutine + block |

---

## 7. Pprof 风险

Pprof 可能暴露：

- goroutine stack。
- function names。
- query/path 片段。
- 内存对象信息。
- 性能敏感信息。

因此生产环境建议：

- 仅内网可访问。
- 经网关鉴权。
- 或默认关闭 profiling。
- 排障时短期开启。
- 采集后及时关闭或限制访问。

---

## 8. Healthz / Pprof 与 Docker/K8s

### 8.1 Docker healthcheck

可用 `/healthz` 作为基础 liveness。

但不要把 `/healthz` 误当 full readiness。

### 8.2 Kubernetes

建议：

- livenessProbe -> `/healthz`。
- readinessProbe -> 独立 `/readyz`，如后续实现。
- pprof 不通过 public ingress 暴露。

---

## 9. 常见误区

### 9.1 “/healthz 返回 ok 就代表业务可用”

不一定。它当前不检查 DB/Redis/IAM/MQ。

### 9.2 “readyz 应该每次 ping DB/Redis/IAM”

不建议。高频探针做重操作会放大故障。

### 9.3 “pprof 可以一直公开”

不应。pprof 是排障工具，不是公共 API。

### 9.4 “CPU high 只看日志就够了”

不够。CPU/goroutine/memory 问题通常需要 pprof。

---

## 10. 修改指南

### 10.1 新增 Readyz

必须：

1. 明确 readiness 语义。
2. 使用 cached snapshot。
3. 区分 fatal unavailable 和 degraded。
4. 不做重网络操作。
5. 补 tests/docs。

### 10.2 修改 pprof 默认值

必须：

1. 评估生产安全。
2. 更新配置文档。
3. 更新部署/运维说明。
4. 补测试。

---

## 11. Verify

```bash
go test ./internal/pkg/server
```
