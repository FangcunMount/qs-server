# 04-接口与运维 阅读地图

**本文回答**：`04-接口与运维/` 这一组文档如何阅读；REST、gRPC、配置、部署、调度、健康检查、排障、容量规划分别去哪里看；它与 `01-运行时/`、`02-业务模块/`、`03-基础设施/` 的边界是什么。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| 本组定位 | 负责 qs-server 的**机器契约与运维入口**：REST/gRPC 契约、配置、端口部署、调度、健康检查、排障、容量档位 |
| 契约真值 | REST 以 `api/rest/*.yaml` 与 `transport/rest` 注册为准；gRPC 以 proto 与 `transport/grpc/registry.go` 为准 |
| 运维真值 | 配置以 `configs/*.yaml` 为准；端口映射以 `build/docker/*.yml` 为准；运行行为以 process/runtime/container 代码为准 |
| 不负责 | 不重复业务领域模型，不替代基础设施深讲，不写历史专题复盘 |
| 重点边界 | collection-server 是前台 BFF；qs-apiserver 是后台/内部主服务；worker 不暴露业务 HTTP/gRPC |
| 当前变化 | 旧文档中事故复盘、代码质量报告、operating 接入等专题不纳入新版主目录，可移入专题或 archive |
| 推荐读法 | 先看契约总览，再读 apiserver REST / collection REST / gRPC，最后看配置部署、调度、观测、排障、容量 |

一句话概括：

> **04-接口与运维回答“系统对外/对内暴露什么机器入口，以及生产中怎么配置、部署、观察、排障”。**

---

## 1. 新版目录

```text
04-接口与运维/
├── README.md
├── 00-接口契约总览.md
├── 01-apiserver-REST.md
├── 02-collection-REST.md
├── 03-gRPC契约.md
├── 04-internal-gRPC.md
├── 05-配置与环境变量.md
├── 06-部署与端口.md
├── 07-调度任务.md
├── 08-健康检查与观测.md
├── 09-常见排障.md
└── 10-QPS容量档位与资源配置建议.md
```

---

## 2. 阅读顺序

| 顺序 | 文档 | 先回答什么 |
| ---- | ---- | ---------- |
| 1 | [00-接口契约总览.md](./00-接口契约总览.md) | REST/gRPC 契约真值、双 REST 面、内部入口边界 |
| 2 | [01-apiserver-REST.md](./01-apiserver-REST.md) | apiserver 的公开/protected/internal REST 分组和中间件 |
| 3 | [02-collection-REST.md](./02-collection-REST.md) | collection-server 的前台 BFF REST、限流、提交队列、公开读接口 |
| 4 | [03-gRPC契约.md](./03-gRPC契约.md) | proto、服务注册、collection/worker client 矩阵 |
| 5 | [04-internal-gRPC.md](./04-internal-gRPC.md) | InternalService 的 worker 回调定位与边界 |
| 6 | [05-配置与环境变量.md](./05-配置与环境变量.md) | 配置文件、环境变量、配置链路、敏感项 |
| 7 | [06-部署与端口.md](./06-部署与端口.md) | dev/prod 端口、Docker Compose 映射、TLS/mTLS |
| 8 | [07-调度任务.md](./07-调度任务.md) | apiserver 内建 scheduler、worker 事件后台、internal REST 手工触发 |
| 9 | [08-健康检查与观测.md](./08-健康检查与观测.md) | healthz/readyz/metrics/pprof/governance/status |
| 10 | [09-常见排障.md](./09-常见排障.md) | 接口、gRPC、部署、调度、观测常见问题 |
| 11 | [10-QPS容量档位与资源配置建议.md](./10-QPS容量档位与资源配置建议.md) | 100-1000 QPS 容量档位、容器资源、压测验收 |

---

## 3. 与其它文档组的分工

| 文档组 | 负责 |
| ------ | ---- |
| `01-运行时/` | 三进程业务运行时、请求链路、进程间协作 |
| `02-业务模块/` | Survey / Scale / Evaluation / Actor / Plan / Statistics 的业务语义 |
| `03-基础设施/` | Event、DataAccess、Redis、Resilience、Security、Integrations、Runtime、Observability 的机制深讲 |
| `04-接口与运维/` | REST/gRPC 契约、配置部署、端口、调度、健康检查、排障、容量 |
| `05-专题分析/` | 事故复盘、质量报告、专项分析、历史专题 |

---

## 4. 关键真值索引

| 类型 | 真值 |
| ---- | ---- |
| apiserver REST | `internal/apiserver/transport/rest` |
| collection REST | `internal/collection-server/transport/rest/router.go` |
| REST 导出 | `api/rest/apiserver.yaml`、`api/rest/collection.yaml` |
| gRPC proto | `internal/apiserver/interface/grpc/proto` |
| gRPC 注册 | `internal/apiserver/transport/grpc/registry.go` |
| 配置 | `configs/*.yaml` |
| 端口部署 | `build/docker/*.yml` |
| HTTP 通用能力 | `internal/pkg/server/genericapiserver.go` |
| gRPC 通用能力 | `internal/pkg/grpc/server.go` |
| scheduler | `internal/apiserver/runtime/scheduler` |

---

## 5. 维护原则

1. 契约文档不能脱离机器文件。
2. 业务语义不要重复到接口运维文档里。
3. internal REST 和 public REST 必须分清。
4. gRPC service 注册条件必须写清 nil-skip 行为。
5. 配置文档必须说明配置链路和敏感项。
6. 端口文档必须区分配置监听、容器端口、宿主机映射。
7. 调度文档必须区分 scheduler 和 MQ worker。
8. 排障文档必须能从现象指向模块深讲。

---

## 6. Verify

```bash
make docs-rest
make docs-verify
go test ./internal/apiserver/transport/rest
go test ./internal/collection-server/transport/rest
go test ./internal/apiserver/transport/grpc
go test ./internal/pkg/server
```

文档检查：

```bash
make docs-hygiene
git diff --check
```
