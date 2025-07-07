# Makefile 使用指南

本文档介绍如何使用项目中的 Makefile 来管理问卷量表系统的所有服务。

## 概述

问卷量表系统包含三个主要服务：
- **apiserver** - 核心API服务器 (端口: 8080)
- **collection-server** - 问卷收集服务器 (端口: 8081)  
- **evaluation-server** - 评估解析服务器 (端口: 8082)

## 快速开始

### 1. 查看所有可用命令
```bash
make help
```

### 2. 构建所有服务
```bash
make build-all
```

### 3. 启动所有服务
```bash
make run-all
```

### 4. 查看服务状态
```bash
make status-all
```

### 5. 停止所有服务
```bash
make stop-all
```

## 详细命令说明

### 🏗️ 构建命令

| 命令 | 描述 |
|------|------|
| `make build-all` | 构建所有服务 |
| `make build-apiserver` | 构建 API 服务器 |
| `make build-collection` | 构建收集服务器 |
| `make build-evaluation` | 构建评估服务器 |

### 🚀 服务管理

#### 启动服务
| 命令 | 描述 |
|------|------|
| `make run-all` | 启动所有服务 |
| `make run-apiserver` | 启动 API 服务器 |
| `make run-collection` | 启动收集服务器 |
| `make run-evaluation` | 启动评估服务器 |

#### 停止服务
| 命令 | 描述 |
|------|------|
| `make stop-all` | 停止所有服务 |
| `make stop-apiserver` | 停止 API 服务器 |
| `make stop-collection` | 停止收集服务器 |
| `make stop-evaluation` | 停止评估服务器 |

#### 重启服务
| 命令 | 描述 |
|------|------|
| `make restart-all` | 重启所有服务 |
| `make restart-apiserver` | 重启 API 服务器 |
| `make restart-collection` | 重启收集服务器 |
| `make restart-evaluation` | 重启评估服务器 |

#### 服务状态
| 命令 | 描述 |
|------|------|
| `make status-all` | 查看所有服务状态 |
| `make status-apiserver` | 查看 API 服务器状态 |
| `make status-collection` | 查看收集服务器状态 |
| `make status-evaluation` | 查看评估服务器状态 |

#### 查看日志
| 命令 | 描述 |
|------|------|
| `make logs-all` | 查看所有服务日志 |
| `make logs-apiserver` | 查看 API 服务器日志 |
| `make logs-collection` | 查看收集服务器日志 |
| `make logs-evaluation` | 查看评估服务器日志 |

### 🔍 健康检查

| 命令 | 描述 |
|------|------|
| `make health-check` | 检查所有服务健康状态 |

### 📨 测试工具

| 命令 | 描述 |
|------|------|
| `make test-message-queue` | 测试消息队列系统 |
| `make test-submit` | 测试答卷提交 |

### 🗄️ 数据库管理

| 命令 | 描述 |
|------|------|
| `make db-deploy` | 部署所有数据库服务 |
| `make db-start` | 启动所有数据库服务 |
| `make db-stop` | 停止所有数据库服务 |
| `make db-restart` | 重启所有数据库服务 |
| `make db-status` | 查看数据库服务状态 |
| `make db-logs` | 查看数据库服务日志 |
| `make db-backup` | 备份所有数据库 |
| `make db-clean` | 清理所有数据库数据（危险操作） |
| `make db-info` | 显示数据库连接信息 |
| `make db-config` | 配置数据库环境变量 |

### 🧪 开发工具

| 命令 | 描述 |
|------|------|
| `make dev` | 启动开发环境（热更新） |
| `make test` | 运行测试 |
| `make clean` | 清理构建文件和进程 |
| `make deps` | 安装依赖 |
| `make install-air` | 安装 Air 热更新工具 |

## 开发环境

### 热更新模式

开发环境支持热更新模式，当代码发生变化时会自动重新编译和运行服务。

#### 开发环境命令

| 命令 | 描述 |
|------|------|
| `make dev` | 启动所有服务（热更新模式） |
| `make dev-stop` | 停止开发环境 |
| `make dev-status` | 查看开发环境状态 |
| `make dev-logs` | 查看开发环境日志 |

#### 开发环境特点

1. **多服务支持**：同时启动 apiserver、collection-server 和 evaluation-server
2. **独立配置**：每个服务使用独立的 air 配置文件
   - apiserver: `.air-apiserver.toml`
   - collection-server: `.air-collection.toml`
   - evaluation-server: `.air-evaluation.toml`
3. **日志管理**：每个服务的构建错误日志单独保存
   - apiserver: `tmp/build-errors-apiserver.log`
   - collection-server: `tmp/build-errors-collection.log`
   - evaluation-server: `tmp/build-errors-evaluation.log`
4. **进程管理**：使用 PID 文件管理 air 进程
   - apiserver: `tmp/pids/air-apiserver.pid`
   - collection-server: `tmp/pids/air-collection.pid`
   - evaluation-server: `tmp/pids/air-evaluation.pid`

#### 使用方法

1. 启动开发环境：
   ```bash
   make dev
   ```
   这将按顺序启动所有服务，每个服务启动后等待2秒确保初始化完成。

2. 查看服务状态：
   ```bash
   make dev-status
   ```

3. 查看构建日志：
   ```bash
   make dev-logs
   ```

4. 停止开发环境：
   ```bash
   make dev-stop
   ```
   或使用 Ctrl+C 停止所有服务。

#### 注意事项

1. **端口占用**：确保以下端口未被占用
   - 8080: apiserver
   - 8081: collection-server
   - 8082: evaluation-server

2. **日志查看**：
   - 使用 `make dev-logs` 查看所有服务的构建错误日志
   - 使用 `make logs-all` 查看服务运行日志

3. **热更新触发**：
   - 修改 Go 源代码文件会触发重新编译
   - 修改配置文件（yaml、json等）也会触发重新加载
   - 修改 `testdata`、`tmp`、`vendor` 等目录下的文件不会触发重新编译

4. **调试建议**：
   - 建议在开发环境中同时打开多个终端窗口
   - 一个窗口运行 `make dev`
   - 另一个窗口运行 `make dev-logs` 查看构建日志
   - 第三个窗口运行 `make logs-all` 查看运行日志

## 服务架构

### 端口分配
- **apiserver**: 8080
- **collection-server**: 8081
- **evaluation-server**: 8082

### 配置文件
- **apiserver**: `configs/apiserver.yaml`
- **collection-server**: `configs/collection-server.yaml`
- **evaluation-server**: `configs/evaluation-server.yaml`

### 日志文件
- **apiserver**: `logs/apiserver.log`
- **collection-server**: `logs/collection-server.log`
- **evaluation-server**: `logs/evaluation-server.log`

### PID 文件
- **apiserver**: `tmp/pids/apiserver.pid`
- **collection-server**: `tmp/pids/collection.pid`
- **evaluation-server**: `tmp/pids/evaluation.pid`

## 典型工作流程

### 开发环境启动
```bash
# 1. 启动数据库服务
make db-start

# 2. 构建所有服务
make build-all

# 3. 启动所有服务
make run-all

# 4. 查看服务状态
make status-all

# 5. 进行健康检查
make health-check
```

### 测试消息队列
```bash
# 启动所有服务后，测试消息队列系统
make test-message-queue

# 测试答卷提交
make test-submit
```

### 查看日志
```bash
# 查看所有服务的实时日志
make logs-all

# 或者查看单个服务的日志
make logs-apiserver
make logs-collection
make logs-evaluation
```

### 停止服务
```bash
# 停止所有服务
make stop-all

# 停止数据库服务
make db-stop
```

## 注意事项

1. **启动顺序**: 使用 `make run-all` 时，系统会按照正确的顺序启动服务（apiserver -> collection-server -> evaluation-server），每个服务启动后会等待2秒。

2. **PID 管理**: 系统使用 PID 文件来跟踪服务进程，确保不会重复启动同一服务。

3. **日志管理**: 所有服务的日志都会保存在 `logs/` 目录下，可以使用 `make logs-*` 命令查看。

4. **健康检查**: 使用 `make health-check` 可以快速检查所有服务是否正常响应。

5. **清理**: 使用 `make clean` 会停止所有服务并清理构建文件和日志。

## 故障排除

### 服务启动失败
1. 检查配置文件是否正确
2. 确认端口是否被占用
3. 查看日志文件了解详细错误信息

### 无法停止服务
1. 使用 `make status-all` 检查服务状态
2. 如果 PID 文件存在但进程不存在，系统会自动清理
3. 必要时可以手动删除 `tmp/pids/` 下的 PID 文件

### 健康检查失败
1. 确认服务是否正在运行
2. 检查防火墙设置
3. 查看服务日志了解详细错误

## 扩展

如果需要添加新的服务，可以按照以下模式在 Makefile 中添加相应的命令：

```makefile
# 新服务的构建命令
build-newservice: ## 构建新服务
	@echo "🔨 构建 newservice..."
	@go build -o $(NEWSERVICE_BIN) ./cmd/newservice/

# 新服务的运行命令
run-newservice: ## 启动新服务
	@echo "🚀 启动 newservice..."
	@$(MAKE) create-dirs
	# ... PID 检查逻辑 ...
	@nohup ./$(NEWSERVICE_BIN) --config=$(NEWSERVICE_CONFIG) > $(LOG_DIR)/newservice.log 2>&1 & echo $$! > $(PID_DIR)/newservice.pid
	@echo "✅ newservice 已启动 (PID: $$(cat $(PID_DIR)/newservice.pid))"
```

然后在相应的 `*-all` 命令中添加对新服务的调用。 