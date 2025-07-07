.PHONY: help dev build clean test
.PHONY: build-all run-all stop-all status-all logs-all
.PHONY: build-apiserver run-apiserver stop-apiserver
.PHONY: build-collection run-collection stop-collection
.PHONY: build-evaluation run-evaluation stop-evaluation

# 服务配置
APISERVER_BIN = qs-apiserver
COLLECTION_BIN = collection-server
EVALUATION_BIN = evaluation-server

APISERVER_CONFIG = configs/apiserver.yaml
COLLECTION_CONFIG = configs/collection-server.yaml
EVALUATION_CONFIG = configs/evaluation-server.yaml

APISERVER_PORT = 9080
COLLECTION_PORT = 9081
EVALUATION_PORT = 9082

# PID 文件目录
PID_DIR = tmp/pids
LOG_DIR = logs

# 默认目标
help: ## 显示帮助信息
	@echo "问卷量表系统 - 服务管理工具"
	@echo "================================="
	@echo ""
	@echo "🏗️  构建命令:"
	@grep -E '^build.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "🚀 服务管理:"
	@grep -E '^(run|start|stop|restart|status|logs).*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "🗄️  数据库管理:"
	@grep -E '^db-.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "🧪 开发工具:"
	@grep -E '^(dev|test|clean|deps).*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# =============================================================================
# 构建命令
# =============================================================================

build: build-all ## 构建所有服务

build-all: ## 构建所有服务
	@echo "🔨 构建所有服务..."
	@$(MAKE) build-apiserver
	@$(MAKE) build-collection
	@$(MAKE) build-evaluation
	@echo "✅ 所有服务构建完成"

build-apiserver: ## 构建 API 服务器
	@echo "🔨 构建 apiserver..."
	@go build -o $(APISERVER_BIN) ./cmd/qs-apiserver/

build-collection: ## 构建收集服务器
	@echo "🔨 构建 collection-server..."
	@go build -o $(COLLECTION_BIN) ./cmd/collection-server/

build-evaluation: ## 构建评估服务器
	@echo "🔨 构建 evaluation-server..."
	@go build -o $(EVALUATION_BIN) ./cmd/evaluation-server/

# =============================================================================
# 服务运行管理
# =============================================================================

run-all: ## 启动所有服务
	@echo "🚀 启动所有服务..."
	@$(MAKE) create-dirs
	@$(MAKE) run-apiserver
	@sleep 2
	@$(MAKE) run-collection
	@sleep 2
	@$(MAKE) run-evaluation
	@echo "✅ 所有服务已启动"
	@$(MAKE) status-all

run-apiserver: ## 启动 API 服务器
	@echo "🚀 启动 apiserver..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		echo "⚠️  apiserver 可能已在运行 (PID: $$(cat $(PID_DIR)/apiserver.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/apiserver.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		else \
			echo "❌ apiserver 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(APISERVER_BIN) --config=$(APISERVER_CONFIG) > $(LOG_DIR)/apiserver.log 2>&1 & echo $$! > $(PID_DIR)/apiserver.pid
	@echo "✅ apiserver 已启动 (PID: $$(cat $(PID_DIR)/apiserver.pid))"

run-collection: ## 启动收集服务器
	@echo "🚀 启动 collection-server..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/collection.pid ]; then \
		echo "⚠️  collection-server 可能已在运行 (PID: $$(cat $(PID_DIR)/collection.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/collection.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/collection.pid; \
		else \
			echo "❌ collection-server 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(COLLECTION_BIN) --config=$(COLLECTION_CONFIG) > $(LOG_DIR)/collection-server.log 2>&1 & echo $$! > $(PID_DIR)/collection.pid
	@echo "✅ collection-server 已启动 (PID: $$(cat $(PID_DIR)/collection.pid))"

run-evaluation: ## 启动评估服务器
	@echo "🚀 启动 evaluation-server..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/evaluation.pid ]; then \
		echo "⚠️  evaluation-server 可能已在运行 (PID: $$(cat $(PID_DIR)/evaluation.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/evaluation.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/evaluation.pid; \
		else \
			echo "❌ evaluation-server 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(EVALUATION_BIN) --config=$(EVALUATION_CONFIG) > $(LOG_DIR)/evaluation-server.log 2>&1 & echo $$! > $(PID_DIR)/evaluation.pid
	@echo "✅ evaluation-server 已启动 (PID: $$(cat $(PID_DIR)/evaluation.pid))"

# =============================================================================
# 服务停止管理
# =============================================================================

stop-all: ## 停止所有服务
	@echo "⏹️  停止所有服务..."
	@$(MAKE) stop-evaluation
	@$(MAKE) stop-collection
	@$(MAKE) stop-apiserver
	@echo "✅ 所有服务已停止"

stop-apiserver: ## 停止 API 服务器
	@echo "⏹️  停止 apiserver..."
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		PID=$$(cat $(PID_DIR)/apiserver.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ apiserver 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		else \
			echo "⚠️  apiserver 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		fi; \
	else \
		echo "ℹ️  apiserver 未运行"; \
	fi

stop-collection: ## 停止收集服务器
	@echo "⏹️  停止 collection-server..."
	@if [ -f $(PID_DIR)/collection.pid ]; then \
		PID=$$(cat $(PID_DIR)/collection.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ collection-server 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/collection.pid; \
		else \
			echo "⚠️  collection-server 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/collection.pid; \
		fi; \
	else \
		echo "ℹ️  collection-server 未运行"; \
	fi

stop-evaluation: ## 停止评估服务器
	@echo "⏹️  停止 evaluation-server..."
	@if [ -f $(PID_DIR)/evaluation.pid ]; then \
		PID=$$(cat $(PID_DIR)/evaluation.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ evaluation-server 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/evaluation.pid; \
		else \
			echo "⚠️  evaluation-server 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/evaluation.pid; \
		fi; \
	else \
		echo "ℹ️  evaluation-server 未运行"; \
	fi

# =============================================================================
# 服务重启管理
# =============================================================================

restart-all: ## 重启所有服务
	@echo "🔄 重启所有服务..."
	@$(MAKE) stop-all
	@sleep 2
	@$(MAKE) run-all

restart-apiserver: ## 重启 API 服务器
	@echo "🔄 重启 apiserver..."
	@$(MAKE) stop-apiserver
	@sleep 1
	@$(MAKE) run-apiserver

restart-collection: ## 重启收集服务器
	@echo "🔄 重启 collection-server..."
	@$(MAKE) stop-collection
	@sleep 1
	@$(MAKE) run-collection

restart-evaluation: ## 重启评估服务器
	@echo "🔄 重启 evaluation-server..."
	@$(MAKE) stop-evaluation
	@sleep 1
	@$(MAKE) run-evaluation

# =============================================================================
# 服务状态和日志
# =============================================================================

status-all: ## 查看所有服务状态
	@echo "📊 服务状态:"
	@echo "============"
	@$(MAKE) status-apiserver
	@$(MAKE) status-collection
	@$(MAKE) status-evaluation

status-apiserver: ## 查看 API 服务器状态
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		PID=$$(cat $(PID_DIR)/apiserver.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ apiserver      - 运行中 (PID: $$PID, Port: $(APISERVER_PORT))"; \
		else \
			echo "❌ apiserver      - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ apiserver      - 未运行"; \
	fi

status-collection: ## 查看收集服务器状态
	@if [ -f $(PID_DIR)/collection.pid ]; then \
		PID=$$(cat $(PID_DIR)/collection.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ collection-server - 运行中 (PID: $$PID, Port: $(COLLECTION_PORT))"; \
		else \
			echo "❌ collection-server - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ collection-server - 未运行"; \
	fi

status-evaluation: ## 查看评估服务器状态
	@if [ -f $(PID_DIR)/evaluation.pid ]; then \
		PID=$$(cat $(PID_DIR)/evaluation.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ evaluation-server - 运行中 (PID: $$PID, Port: $(EVALUATION_PORT))"; \
		else \
			echo "❌ evaluation-server - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ evaluation-server - 未运行"; \
	fi

logs-all: ## 查看所有服务日志
	@echo "📋 查看所有服务日志..."
	@echo "使用 Ctrl+C 退出"
	@tail -f $(LOG_DIR)/apiserver.log $(LOG_DIR)/collection-server.log $(LOG_DIR)/evaluation-server.log

logs-apiserver: ## 查看 API 服务器日志
	@echo "📋 查看 apiserver 日志..."
	@tail -f $(LOG_DIR)/apiserver.log

logs-collection: ## 查看收集服务器日志
	@echo "📋 查看 collection-server 日志..."
	@tail -f $(LOG_DIR)/collection-server.log

logs-evaluation: ## 查看评估服务器日志
	@echo "📋 查看 evaluation-server 日志..."
	@tail -f $(LOG_DIR)/evaluation-server.log

# =============================================================================
# 健康检查
# =============================================================================

health-check: ## 检查所有服务健康状态
	@echo "🔍 健康检查:"
	@echo "============"
	@echo -n "apiserver:        "; curl -s http://localhost:$(APISERVER_PORT)/healthz || echo "❌ 无响应"
	@echo -n "collection-server: "; curl -s http://localhost:$(COLLECTION_PORT)/healthz || echo "❌ 无响应"
	@echo -n "evaluation-server: "; curl -s http://localhost:$(EVALUATION_PORT)/healthz || echo "❌ 无响应"

# =============================================================================
# 测试工具
# =============================================================================

test-message-queue: ## 测试消息队列系统
	@echo "📨 测试消息队列系统..."
	@if [ ! -x "./test-message-queue.sh" ]; then \
		echo "❌ 测试脚本不存在或不可执行"; \
		exit 1; \
	fi
	@./test-message-queue.sh

test-submit: ## 测试答卷提交
	@echo "📝 测试答卷提交..."
	@if [ ! -x "./test-answersheet-submit.sh" ]; then \
		echo "❌ 测试脚本不存在或不可执行"; \
		exit 1; \
	fi
	@./test-answersheet-submit.sh

# =============================================================================
# 开发工具
# =============================================================================

dev: ## 启动开发环境（热更新）
	@echo "🚀 启动开发环境..."
	@mkdir -p tmp
	@echo "启动 apiserver..."
	@air -c .air-apiserver.toml & echo $$! > tmp/pids/air-apiserver.pid
	@sleep 2
	@echo "启动 collection-server..."
	@air -c .air-collection.toml & echo $$! > tmp/pids/air-collection.pid
	@sleep 2
	@echo "启动 evaluation-server..."
	@air -c .air-evaluation.toml & echo $$! > tmp/pids/air-evaluation.pid
	@echo "✅ 所有服务已启动（热更新模式）"
	@echo "提示：使用 Ctrl+C 停止所有服务"
	@echo "      或使用 make dev-stop 停止服务"

dev-stop: ## 停止开发环境
	@echo "⏹️  停止开发环境..."
	@if [ -f tmp/pids/air-evaluation.pid ]; then \
		kill $$(cat tmp/pids/air-evaluation.pid) 2>/dev/null || true; \
		rm -f tmp/pids/air-evaluation.pid; \
	fi
	@if [ -f tmp/pids/air-collection.pid ]; then \
		kill $$(cat tmp/pids/air-collection.pid) 2>/dev/null || true; \
		rm -f tmp/pids/air-collection.pid; \
	fi
	@if [ -f tmp/pids/air-apiserver.pid ]; then \
		kill $$(cat tmp/pids/air-apiserver.pid) 2>/dev/null || true; \
		rm -f tmp/pids/air-apiserver.pid; \
	fi
	@echo "✅ 开发环境已停止"

dev-status: ## 查看开发环境状态
	@echo "📊 开发环境状态:"
	@echo "=============="
	@if [ -f tmp/pids/air-apiserver.pid ] && kill -0 $$(cat tmp/pids/air-apiserver.pid) 2>/dev/null; then \
		echo "✅ apiserver      - 运行中 (PID: $$(cat tmp/pids/air-apiserver.pid))"; \
	else \
		echo "⚪ apiserver      - 未运行"; \
	fi
	@if [ -f tmp/pids/air-collection.pid ] && kill -0 $$(cat tmp/pids/air-collection.pid) 2>/dev/null; then \
		echo "✅ collection     - 运行中 (PID: $$(cat tmp/pids/air-collection.pid))"; \
	else \
		echo "⚪ collection     - 未运行"; \
	fi
	@if [ -f tmp/pids/air-evaluation.pid ] && kill -0 $$(cat tmp/pids/air-evaluation.pid) 2>/dev/null; then \
		echo "✅ evaluation     - 运行中 (PID: $$(cat tmp/pids/air-evaluation.pid))"; \
	else \
		echo "⚪ evaluation     - 未运行"; \
	fi

dev-logs: ## 查看开发环境日志
	@echo "📋 开发环境日志:"
	@echo "=============="
	@tail -f tmp/build-errors-*.log

test: ## 运行测试
	@echo "🧪 运行测试..."
	@go test ./...

clean: ## 清理构建文件和进程
	@echo "🧹 清理构建文件和进程..."
	@$(MAKE) stop-all
	@rm -rf tmp bin $(LOG_DIR)/*.log
	@rm -f $(APISERVER_BIN) $(COLLECTION_BIN) $(EVALUATION_BIN)
	@go clean
	@echo "✅ 清理完成"

create-dirs: ## 创建必要的目录
	@mkdir -p $(PID_DIR) $(LOG_DIR)

install-air: ## 安装 Air 热更新工具
	@echo "📦 安装 Air..."
	@go install github.com/air-verse/air@latest

deps: ## 安装依赖
	@echo "📦 安装依赖..."
	@go mod download
	@go mod tidy

# =============================================================================
# 数据库管理（保持原有功能）
# =============================================================================

db-deploy: ## 部署所有数据库服务
	@echo "🗄️  部署数据库基础设施..."
	@if [ ! -f configs/env/config.env ]; then \
		echo "⚠️  配置文件不存在，从模板创建..."; \
		cp configs/env/config.prod.env configs/env/config.env; \
		echo "✅ 已创建配置文件 configs/env/config.env"; \
		echo "🔧 请根据需要修改配置文件中的参数"; \
	fi
	@cd build/docker/infra && ./deploy.sh deploy

db-start: ## 启动所有数据库服务
	@echo "▶️  启动数据库服务..."
	@cd build/docker/infra && ./deploy.sh start

db-stop: ## 停止所有数据库服务
	@echo "⏹️  停止数据库服务..."
	@cd build/docker/infra && ./deploy.sh stop

db-restart: ## 重启所有数据库服务
	@echo "🔄 重启数据库服务..."
	@cd build/docker/infra && ./deploy.sh restart

db-status: ## 查看数据库服务状态
	@echo "📊 数据库服务状态:"
	@cd build/docker/infra && ./deploy.sh status

db-logs: ## 查看数据库服务日志
	@echo "📋 数据库服务日志:"
	@cd build/docker/infra && ./deploy.sh logs

db-backup: ## 备份所有数据库
	@echo "💾 备份数据库..."
	@cd build/docker/infra && ./deploy.sh backup

db-clean: ## 清理所有数据库数据（危险操作）
	@echo "🧹 清理数据库数据..."
	@cd build/docker/infra && ./deploy.sh clean

db-info: ## 显示数据库连接信息
	@echo "ℹ️  数据库连接信息:"
	@cd build/docker/infra && ./deploy.sh info

db-config: ## 配置数据库环境变量
	@echo "🔧 数据库配置管理:"
	@if [ ! -f configs/env/config.env ]; then \
		echo "📄 从模板创建配置文件..."; \
		cp configs/env/config.prod.env configs/env/config.env; \
		echo "✅ 已创建 configs/env/config.env"; \
	else \
		echo "📄 配置文件已存在: configs/env/config.env"; \
	fi
	@echo "🔧 请编辑配置文件: nano configs/env/config.env"
	@echo "📖 查看配置说明: cat configs/env/README.md" 