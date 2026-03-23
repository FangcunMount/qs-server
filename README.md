# qs-server

> 问卷量表测评系统 - 基于 DDD 和六边形架构的企业级解决方案

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## 🎯 系统概述

**qs-server** 是一个专业的问卷量表测评系统，支持问卷收集、医学/心理量表测评、智能计分和解读报告生成。

### 核心服务

| 服务 | 职责 | 端口 (dev) |
| ----- | ------ | ----------- |
| **qs-apiserver** | 核心 API 服务（问卷、量表、评估、用户管理） | 18082 |
| **qs-worker** | 后台事件处理（异步评估、报告生成） | - |
| **collection-server** | 轻量级数据收集（小程序端） | 18083 |

### 技术栈

- **语言**: Go 1.21+
- **架构**: DDD + 六边形架构 + 事件驱动
- **存储**: MySQL (业务) + MongoDB (内容) + Redis (缓存/队列)
- **API**: RESTful + gRPC

## 🚀 快速开始

### 环境要求

- Go 1.21+
- MySQL 8.0+
- MongoDB 6.0+
- Redis 7.0+ (双实例: Cache 6379, Store 6380)

### 启动服务

```bash
# 克隆项目
git clone https://github.com/FangcunMountain/qs-server.git
cd qs-server

# 检查基础设施
make check-infra

# 编译并启动
make build
make run-apiserver      # API Server
make run-worker         # Worker
make run-collection     # Collection Server

# 或启动全部
make run-all
```

### 开发模式（热重载）

```bash
make dev-apiserver
make dev-worker
make dev-collection
```

### 验证服务

```bash
curl http://localhost:18082/healthz   # API Server
curl http://localhost:18083/healthz   # Collection Server
```

## 📁 项目结构

```text
qs-server/
├── cmd/                          # 服务入口
│   ├── qs-apiserver/
│   ├── qs-worker/
│   └── collection-server/
├── internal/                     # 内部实现
│   ├── apiserver/
│   │   ├── domain/              # 领域层 (DDD 核心)
│   │   ├── application/         # 应用服务层
│   │   ├── infra/               # 基础设施层
│   │   └── interface/           # 接口层 (REST/gRPC)
│   ├── worker/
│   └── collection-server/
├── pkg/                          # 公共包
├── configs/                      # 配置文件
├── docs/                         # 设计文档
└── build/docker/                 # Docker 配置
```

## 📚 文档导航

详细设计文档位于 [`docs/`](./docs/) 目录：

| 目录 | 内容 |
| ----- | ------ |
| [00-概览](./docs/00-概览/) | 系统架构、代码结构 |
| [01-survey域](./docs/01-survey域/) | 问卷子域设计 |
| [02-scale域](./docs/02-scale域/) | 量表子域设计 |
| [03-evaluation域](./docs/03-evaluation域/) | 评估子域设计 |
| [04-actor域](./docs/04-actor域/) | 用户模型设计 |
| [05-plan域](./docs/05-plan域/) | 测评计划设计 |
| [06-screening域](./docs/06-screening域/) | 入校筛查设计 |
| [07-基础设施](./docs/07-基础设施/) | 高并发架构 |
| [08-运维部署](./docs/08-运维部署/) | 端口配置 |

## 🔧 常用命令

```bash
# 构建
make build              # 编译所有服务
make build-apiserver    # 编译 API Server

# 运行
make run-all            # 启动所有服务
make stop-all           # 停止所有服务
make status-all         # 查看服务状态

# 开发
make dev-apiserver      # 热重载开发
make test               # 运行测试
make lint               # 代码检查

# 基础设施
make check-infra        # 检查依赖服务
make check-mysql
make check-redis
make check-mongodb
```

## 🏗️ 架构概览

```text
┌─────────────────────────────────────────────────────────────┐
│                      Interface Layer                        │
│              REST API / gRPC / WebSocket                    │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│                 DTO / Service / Mapper                      │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                      Domain Layer                           │
│  ┌─────────┐ ┌─────────┐ ┌────────────┐ ┌───────────────┐  │
│  │ Survey  │ │  Scale  │ │ Evaluation │ │ Actor/Plan/   │  │
│  │  域     │ │   域    │ │    域      │ │ Screening     │  │
│  └─────────┘ └─────────┘ └────────────┘ └───────────────┘  │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                   Infrastructure Layer                      │
│              MySQL / MongoDB / Redis / MQ                   │
└─────────────────────────────────────────────────────────────┘
```

## 📝 开发规范

### 提交格式

```text
<type>(<scope>): <subject>

feat(survey): 添加问卷版本管理
fix(evaluation): 修复计分算法
docs(readme): 更新快速开始指南
```

### 代码检查

```bash
make lint       # golangci-lint
make test       # 单元测试
make coverage   # 测试覆盖率
```

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.

---

**[📖 查看完整文档](./docs/README.md)** | **[🐛 问题反馈](https://github.com/FangcunMountain/qs-server/issues)**
