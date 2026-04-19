# qs-server

问卷与量表测评后端：**前台收集答卷**、**领域事件驱动异步评估**、**报告与统计**；实现上采用 **DDD + 六边形架构**，主业务集中在 **qs-apiserver**，**collection-server** 为前台 BFF，**qs-worker** 消费消息并回调内部 gRPC。

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## 功能特性

- **三进程分工**：`qs-apiserver`（领域、REST/gRPC、持久化、发事件）、`collection-server`（REST 收集端、IAM/监护前置、gRPC 调 apiserver）、`qs-worker`（订阅 MQ、internal gRPC 推进计分/测评/报告等）。
- **核心业务**：问卷与答卷（Survey）、量表规则（Scale）、测评与报告（Evaluation）、受试者与计划（Actor / Plan）、统计（Statistics）；详见 [docs/02-业务模块](docs/02-业务模块/)。
- **事件驱动**：领域事件路由以 [configs/events.yaml](configs/events.yaml) 为单一事实来源。
- **接口形态**：对外 REST（OpenAPI）、进程间 gRPC；IAM 以 SDK 嵌入，非第四进程。

| 服务 | 职责摘要 | HTTP 端口（`ENV=dev`，默认） |
| ---- | -------- | ------------------------------ |
| **qs-apiserver** | 核心 API、领域模块、gRPC、事件发布 | `18082` |
| **collection-server** | 小程序/收集端 REST、访问控制前置 | `18083` |
| **qs-worker** | 异步评估、报告与统计等（无固定对外 HTTP） | - |

**技术栈（概要）**：Go · MySQL · MongoDB · Redis · 消息队列（配置见各环境 yaml）· REST + gRPC。横切能力部分依赖 [github.com/FangcunMount/component-base](https://github.com/FangcunMount/component-base)（详见 [docs/00-总览/02-代码组织与边界.md](docs/00-总览/02-代码组织与边界.md)）。

---

## 软件架构

运行时上，**同步请求**经 collection 或直连 apiserver 的 REST/gRPC 进入领域；**异步链路**由 apiserver 发事件 → worker 消费 → **internal gRPC** 回到 apiserver 执行业务步骤。更完整的地图与边界见 [docs/00-总览/01-系统地图.md](docs/00-总览/01-系统地图.md)。

### 仓库目录（骨架）

```text
qs-server/
├── api/
│   └── rest/                           # OpenAPI 契约（apiserver / collection）
├── cmd/
│   ├── qs-apiserver/
│   ├── collection-server/
│   ├── qs-worker/
│   └── tools/                          # 辅助工具（如 seeddata）
├── configs/                            # 各进程 *.dev.yaml / *.prod.yaml、events.yaml 等
├── docs/                               # 设计文档（入口 docs/README.md）
├── internal/
│   ├── apiserver/                      # application / domain / infra / interface / container
│   ├── collection-server/
│   ├── worker/
│   └── pkg/                            # 三进程共享（grpc、middleware、migration…）
├── pkg/                                # 根级可复用库（与 internal/pkg 不同）
├── scripts/
├── build/docker/
├── web/
├── Makefile
└── go.mod
```

约定与 **component-base** 分工见 [docs/00-总览/02-代码组织与边界.md](docs/00-总览/02-代码组织与边界.md)。

### 分层示意（apiserver 内）

```text
┌─────────────────────────────────────────────────────────────┐
│                      Interface 层                            │
│                   REST / gRPC 入站                           │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Application 层                           │
│                 用例编排、应用服务                           │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      Domain 层                              │
│  ┌─────────┐ ┌─────────┐ ┌────────────┐ ┌───────────────┐  │
│  │ Survey  │ │  Scale  │ │ Evaluation │ │ Actor / Plan /│  │
│  │         │ │         │ │            │ │ Statistics    │  │
│  └─────────┘ └─────────┘ └────────────┘ └───────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                   Infrastructure 层                         │
│           MySQL / MongoDB / Redis / MQ / …                  │
└─────────────────────────────────────────────────────────────┘
```

---

## 快速开始

### 依赖检查

- **Go**：与 [go.mod](go.mod) 中 `go` 版本一致（当前为 1.24.x）。
- **运行时依赖**（本地联调常见）：MySQL、MongoDB、Redis、消息队列等；以各环境 [configs/](configs/) 为准。
- 一键检查（需本机已装客户端工具，脚本见 [scripts/](scripts/)）：

```bash
make check-infra        # 全部
make check-mysql
make check-redis
make check-mongodb
make check-nsq
```

### 构建

```bash
git clone https://github.com/FangcunMount/qs-server.git
cd qs-server

make build              # 全部二进制
make build-apiserver    # 仅 qs-apiserver
```

### 运行

默认 **`ENV=dev`**（见 [Makefile](Makefile)）：选用 `configs/*.dev.yaml`，HTTP 端口为 **apiserver 18082**、**collection 18083**；`ENV=prod` 时使用 `*.prod.yaml` 与另一组端口。

```bash
make run-apiserver
make run-worker
make run-collection
# 或
make run-all
```

**健康检查**（dev）：

```bash
curl -sS http://127.0.0.1:18082/healthz   # apiserver
curl -sS http://127.0.0.1:18083/healthz   # collection
make health-check                         # 含 worker 进程检测（见 Makefile）
```

**热重载开发**（需已安装 [air](https://github.com/air-verse/air) 等，见 Makefile 目标）：

```bash
make dev-apiserver
make dev-worker
make dev-collection
```

### 配置与环境变量

- 三进程配置与环境约定：[docs/00-总览/04-本地开发与配置约定.md](docs/00-总览/04-本地开发与配置约定.md)。
- 容器编排：[build/docker/docker-compose.dev.yml](build/docker/docker-compose.dev.yml)、[build/docker/docker-compose.prod.yml](build/docker/docker-compose.prod.yml)。

---

## 使用指南

- **文档入口**：[docs/README.md](docs/README.md)；写作约定：[docs/CONTRIBUTING-DOCS.md](docs/CONTRIBUTING-DOCS.md)。
- **契约**：REST — [api/rest/apiserver.yaml](api/rest/apiserver.yaml)、[api/rest/collection.yaml](api/rest/collection.yaml)；gRPC proto — [internal/apiserver/interface/grpc/proto/](internal/apiserver/interface/grpc/proto/)、事件 — [configs/events.yaml](configs/events.yaml)。
- **常用命令**：

```bash
make help               # 目标说明
make stop-all
make status-all
make test
make lint
make coverage
```

- **种子数据等工具**：[tools/seeddata-runner](tools/seeddata-runner)（配置 [tools/seeddata-runner/configs/seeddata.yaml](tools/seeddata-runner/configs/seeddata.yaml)）。

---

## 如何贡献

1. 通过 [Issues](https://github.com/FangcunMount/qs-server/issues) 讨论缺陷与需求；欢迎提交 Pull Request。
2. 提交前本地执行：`make lint`、`make test`（或 `make coverage`）。
3. **提交说明**建议采用简短约定，例如：

```text
<type>(<scope>): <subject>

feat(survey): 添加问卷版本管理
fix(evaluation): 修复计分边界
docs(readme): 更新快速开始
```

4.**文档变更**请遵循 [docs/CONTRIBUTING-DOCS.md](docs/CONTRIBUTING-DOCS.md)，并与源码、契约一致。

---

## 社区

- **问题与讨论**：[GitHub Issues](https://github.com/FangcunMount/qs-server/issues)

---

## 关于作者

本项目由 **[FangcunMount](https://github.com/FangcunMount)** 组织维护；具体贡献者见仓库提交历史。

---

## 许可证

本项目基于 **MIT License** 发布，全文见 [LICENSE](LICENSE)。
