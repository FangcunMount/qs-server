# qs-server 性能自测速记

## 火焰图（pprof）

- 开关：`configs/apiserver.*.yaml` 里的 `server.profiling: true`（默认开启）。
- 路由：`http://<host>:<http-port>/debug/pprof`，适用于 CPU/Heap/阻塞/互斥等。
- 采样 CPU 并看火焰图：

  ```bash
  go tool pprof -http=:8081 "http://127.0.0.1:18082/debug/pprof/profile?seconds=30"
  # 浏览器打开 http://127.0.0.1:8081 查看 flame graph
  ```

- 采样堆内存：

  ```bash
  go tool pprof -http=:8081 "http://127.0.0.1:18082/debug/pprof/heap"
  ```

## HTTP QPS 压测

- 指标：`server.metrics: true`（默认）时暴露 `/metrics`，Prometheus 指标里包含 HTTP QPS、延迟分位。
- 快速压测脚本：`scripts/perf/k6-qs.js`

  ```bash
  # 安装 k6 后运行（默认打 /api/v1/public/info）
  k6 run scripts/perf/k6-qs.js \
    --env BASE_URL=http://127.0.0.1:18082 \
    --env PATH=/api/v1/public/info \
    --env RPS=200 \
    --env DURATION=2m

  # 若接口需要认证，可传 TOKEN 环境变量：
  # --env TOKEN="Bearer <jwt>"
  ```

- 梯度压测：调整 `RPS`/`VUS`/`DURATION` 多次运行，观察 QPS 与 P95/P99 延迟拐点，并结合 `/metrics`、pprof 定位热点。
