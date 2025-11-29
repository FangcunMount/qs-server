# ============================================================================
# Makefile for QS-Server
# ============================================================================
# 项目：qs-server - 问卷量表系统
# 架构：六边形架构 + DDD + CQRS
# ============================================================================

.DEFAULT_GOAL := help

# ============================================================================
# 变量定义
# ============================================================================

# 项目信息
PROJECT_NAME := qs-server
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Go 相关
GO := go
GO_BUILD := $(GO) build
GO_TEST := $(GO) test
GO_LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# 目录结构
BIN_DIR := bin
TMP_DIR := tmp
PID_DIR := $(TMP_DIR)/pids
LOG_DIR := logs
COVERAGE_DIR := coverage

# 服务配置
APISERVER_BIN := $(BIN_DIR)/qs-apiserver
COLLECTION_BIN := $(BIN_DIR)/collection-server
WORKER_BIN := $(BIN_DIR)/qs-worker

# 根据 ENV 选择配置与端口（默认 dev）
ifeq ($(ENV),prod)
  APISERVER_CONFIG := configs/apiserver.prod.yaml
  COLLECTION_CONFIG := configs/collection-server.prod.yaml
  WORKER_CONFIG := configs/worker.prod.yaml
  # 宿主机端口为避免与已部署的 IAM 冲突，统一后移一位
  APISERVER_PORT := 8081
  COLLECTION_PORT := 8082
else
  APISERVER_CONFIG := configs/apiserver.dev.yaml
  COLLECTION_CONFIG := configs/collection-server.dev.yaml
  WORKER_CONFIG := configs/worker.dev.yaml
  APISERVER_PORT := 18082
  COLLECTION_PORT := 18083
endif

# 环境配置
ENV ?= dev

# 颜色输出
COLOR_RESET := \033[0m
COLOR_BOLD := \033[1m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_BLUE := \033[34m
COLOR_CYAN := \033[36m
COLOR_RED := \033[31m

# ============================================================================
# .PHONY 声明
# ============================================================================

.PHONY: help version debug
.PHONY: build build-all build-apiserver build-collection build-worker clean
.PHONY: run run-all run-apiserver run-collection run-worker
.PHONY: stop stop-all stop-apiserver stop-collection stop-worker
.PHONY: restart restart-all restart-apiserver restart-collection restart-worker
.PHONY: status status-all status-apiserver status-collection status-worker
.PHONY: logs logs-all logs-apiserver logs-collection logs-worker
.PHONY: health health-check
.PHONY: check-infra check-mysql check-redis check-mongodb check-nsq
.PHONY: dev dev-apiserver dev-collection dev-worker dev-stop dev-status dev-logs
.PHONY: test test-unit test-coverage test-race test-bench test-all
.PHONY: test-submit test-message-queue
.PHONY: lint fmt fmt-check
.PHONY: deps deps-download deps-tidy deps-verify deps-check
.PHONY: install-tools install-air create-dirs
.PHONY: up down re st log
.PHONY: quick-start

# ============================================================================
# 帮助信息
# ============================================================================

help: ## 显示帮助信息
	@echo "$(COLOR_BOLD)$(COLOR_CYAN)======================================"
	@echo "  问卷量表系统 - 构建和管理工具"
	@echo "======================================$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)项目信息:$(COLOR_RESET)"
	@echo "  版本:     $(COLOR_GREEN)$(VERSION)$(COLOR_RESET)"
	@echo "  分支:     $(COLOR_GREEN)$(GIT_BRANCH)$(COLOR_RESET)"
	@echo "  提交:     $(COLOR_GREEN)$(GIT_COMMIT)$(COLOR_RESET)"
	@echo "  环境:     $(COLOR_GREEN)$(ENV)$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)📦 构建命令:$(COLOR_RESET)"
	@grep -E '^build.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)🚀 服务管理:$(COLOR_RESET)"
	@grep -E '^(run|stop|restart|status|logs|health).*:.*?## .*$$' $(MAKEFILE_LIST) | grep -v "dev" | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)🔍 环境检查:$(COLOR_RESET)"
	@grep -E '^check.*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)🛠️  开发工具:$(COLOR_RESET)"
	@grep -E '^(dev|test|lint|fmt).*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(COLOR_BOLD)📚 其他命令:$(COLOR_RESET)"
	@grep -E '^(deps|install|clean|version|debug|up|down|quick).*:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(COLOR_CYAN)%-25s$(COLOR_RESET) %s\n", $$1, $$2}'
	@echo ""

version: ## 显示版本信息
	@echo "$(COLOR_BOLD)版本信息:$(COLOR_RESET)"
	@echo "  版本:     $(COLOR_GREEN)$(VERSION)$(COLOR_RESET)"
	@echo "  构建时间: $(BUILD_TIME)"
	@echo "  Git 提交: $(GIT_COMMIT)"
	@echo "  Git 分支: $(GIT_BRANCH)"
	@echo "  Go 版本:  $(shell $(GO) version)"

# ============================================================================
# 快速启动
# ============================================================================

quick-start: check-infra build-all run-all ## 快速启动 (检查环境 + 构建 + 运行所有服务)
	@echo "$(COLOR_GREEN)✅ 开发环境已就绪!$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)ℹ️  已启动服务:$(COLOR_RESET)"
	@echo "  $(COLOR_GREEN)•$(COLOR_RESET) API Server        ($(APISERVER_PORT))"
	@echo "  $(COLOR_GREEN)•$(COLOR_RESET) Collection Server ($(COLLECTION_PORT))"
	@echo "  $(COLOR_GREEN)•$(COLOR_RESET) Worker            (后台处理)"
	@echo ""
	@$(MAKE) status-all

# ============================================================================
# 构建命令
# ============================================================================

build: build-all ## 构建所有服务

build-all: ## 构建所有服务
	@echo "$(COLOR_BOLD)$(COLOR_BLUE)🔨 构建所有服务 [环境: $(ENV)]...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(MAKE) build-apiserver
	@$(MAKE) build-collection
	@$(MAKE) build-worker
	@echo "$(COLOR_GREEN)✅ 所有服务构建完成$(COLOR_RESET)"

build-apiserver: ## 构建 API 服务器
	@echo "$(COLOR_BLUE)🔨 构建 API 服务器...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(GO_BUILD) $(GO_LDFLAGS) -o $(APISERVER_BIN) ./cmd/qs-apiserver/
	@echo "$(COLOR_GREEN)✅ API 服务器构建完成: $(APISERVER_BIN)$(COLOR_RESET)"

build-collection: ## 构建 Collection 服务器
	@echo "$(COLOR_BLUE)🔨 构建 Collection 服务器...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(GO_BUILD) $(GO_LDFLAGS) -o $(COLLECTION_BIN) ./cmd/collection-server/
	@echo "$(COLOR_GREEN)✅ Collection 服务器构建完成: $(COLLECTION_BIN)$(COLOR_RESET)"

build-worker: ## 构建 Worker 服务
	@echo "$(COLOR_BLUE)🔨 构建 Worker 服务...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(GO_BUILD) $(GO_LDFLAGS) -o $(WORKER_BIN) ./cmd/qs-worker/
	@echo "$(COLOR_GREEN)✅ Worker 服务构建完成: $(WORKER_BIN)$(COLOR_RESET)"

# ============================================================================
# 服务运行管理
# ============================================================================

run: run-all ## 启动所有服务

run-all: check-infra ## 启动所有服务（先检查基础设施）
	@echo "$(COLOR_BOLD)$(COLOR_BLUE)🚀 启动所有服务 [环境: $(ENV)]...$(COLOR_RESET)"
	@$(MAKE) create-dirs
	@$(MAKE) run-apiserver
	@sleep 2
	@$(MAKE) run-collection
	@sleep 2
	@$(MAKE) run-worker
	@echo "$(COLOR_GREEN)✅ 所有服务已启动$(COLOR_RESET)"
	@echo ""
	@$(MAKE) status-all

run-apiserver: ## 启动 API 服务器
	@echo "🚀 启动 qs-apiserver..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		echo "⚠️  qs-apiserver 可能已在运行 (PID: $$(cat $(PID_DIR)/apiserver.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/apiserver.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		else \
			echo "❌ qs-apiserver 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(APISERVER_BIN) --config=$(APISERVER_CONFIG) > $(LOG_DIR)/apiserver.log 2>&1 & echo $$! > $(PID_DIR)/apiserver.pid
	@echo "✅ qs-apiserver 已启动 (PID: $$(cat $(PID_DIR)/apiserver.pid))"

run-collection: ## 启动 Collection 服务器
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

run-worker: ## 启动 Worker 服务
	@echo "🚀 启动 qs-worker..."
	@$(MAKE) create-dirs
	@if [ -f $(PID_DIR)/worker.pid ]; then \
		echo "⚠️  qs-worker 可能已在运行 (PID: $$(cat $(PID_DIR)/worker.pid))"; \
		if ! kill -0 $$(cat $(PID_DIR)/worker.pid) 2>/dev/null; then \
			echo "🧹 清理无效的 PID 文件"; \
			rm -f $(PID_DIR)/worker.pid; \
		else \
			echo "❌ qs-worker 已在运行，请先停止"; \
			exit 1; \
		fi; \
	fi
	@nohup ./$(WORKER_BIN) --config=$(WORKER_CONFIG) > $(LOG_DIR)/worker.log 2>&1 & echo $$! > $(PID_DIR)/worker.pid
	@echo "✅ qs-worker 已启动 (PID: $$(cat $(PID_DIR)/worker.pid))"

# ============================================================================
# 服务停止管理
# ============================================================================

stop: stop-all ## 停止所有服务

stop-all: ## 停止所有服务
	@echo "⏹️  停止所有服务..."
	@$(MAKE) stop-worker
	@$(MAKE) stop-collection
	@$(MAKE) stop-apiserver
	@echo "✅ 所有服务已停止"

stop-apiserver: ## 停止 API 服务器
	@echo "⏹️  停止 qs-apiserver..."
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		PID=$$(cat $(PID_DIR)/apiserver.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ qs-apiserver 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		else \
			echo "⚠️  qs-apiserver 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/apiserver.pid; \
		fi; \
	else \
		echo "ℹ️  qs-apiserver 未运行"; \
	fi

stop-collection: ## 停止 Collection 服务器
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

stop-worker: ## 停止 Worker 服务
	@echo "⏹️  停止 qs-worker..."
	@if [ -f $(PID_DIR)/worker.pid ]; then \
		PID=$$(cat $(PID_DIR)/worker.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID && echo "✅ qs-worker 已停止 (PID: $$PID)"; \
			rm -f $(PID_DIR)/worker.pid; \
		else \
			echo "⚠️  qs-worker 进程不存在，清理 PID 文件"; \
			rm -f $(PID_DIR)/worker.pid; \
		fi; \
	else \
		echo "ℹ️  qs-worker 未运行"; \
	fi

# ============================================================================
# 服务重启管理
# ============================================================================

restart: restart-all ## 重启所有服务

restart-all: ## 重启所有服务
	@echo "🔄 重启所有服务..."
	@$(MAKE) stop-all
	@sleep 2
	@$(MAKE) run-all

restart-apiserver: ## 重启 API 服务器
	@echo "🔄 重启 qs-apiserver..."
	@$(MAKE) stop-apiserver
	@sleep 1
	@$(MAKE) run-apiserver

restart-collection: ## 重启 Collection 服务器
	@echo "🔄 重启 collection-server..."
	@$(MAKE) stop-collection
	@sleep 1
	@$(MAKE) run-collection

restart-worker: ## 重启 Worker 服务
	@echo "🔄 重启 qs-worker..."
	@$(MAKE) stop-worker
	@sleep 1
	@$(MAKE) run-worker

# ============================================================================
# 服务状态和日志
# ============================================================================

status: status-all ## 查看所有服务状态

status-all: ## 查看所有服务状态
	@echo "📊 服务状态 [环境: $(ENV)]:"
	@echo "============"
	@$(MAKE) status-apiserver
	@$(MAKE) status-collection
	@$(MAKE) status-worker

status-apiserver: ## 查看 API 服务器状态
	@if [ -f $(PID_DIR)/apiserver.pid ]; then \
		PID=$$(cat $(PID_DIR)/apiserver.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ qs-apiserver      - 运行中 (PID: $$PID, Port: $(APISERVER_PORT))"; \
		else \
			echo "❌ qs-apiserver      - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ qs-apiserver      - 未运行"; \
	fi

status-collection: ## 查看 Collection 服务器状态
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

status-worker: ## 查看 Worker 服务状态
	@if [ -f $(PID_DIR)/worker.pid ]; then \
		PID=$$(cat $(PID_DIR)/worker.pid); \
		if kill -0 $$PID 2>/dev/null; then \
			echo "✅ qs-worker         - 运行中 (PID: $$PID)"; \
		else \
			echo "❌ qs-worker         - 已停止 (PID 文件存在但进程不存在)"; \
		fi; \
	else \
		echo "⚪ qs-worker         - 未运行"; \
	fi

logs: logs-all ## 查看所有日志

logs-all: ## 查看所有服务日志
	@echo "📋 查看所有服务日志..."
	@echo "使用 Ctrl+C 退出"
	@tail -f $(LOG_DIR)/apiserver.log $(LOG_DIR)/collection-server.log $(LOG_DIR)/worker.log

logs-apiserver: ## 查看 API 服务器日志
	@echo "📋 查看 qs-apiserver 日志..."
	@tail -f $(LOG_DIR)/apiserver.log

logs-collection: ## 查看 Collection 服务器日志
	@echo "📋 查看 collection-server 日志..."
	@tail -f $(LOG_DIR)/collection-server.log

logs-worker: ## 查看 Worker 服务日志
	@echo "📋 查看 qs-worker 日志..."
	@tail -f $(LOG_DIR)/worker.log

# ============================================================================
# 健康检查
# ============================================================================

health: health-check ## 健康检查

health-check: ## 检查所有服务健康状态
	@echo "🔍 健康检查:"
	@echo "============"
	@echo -n "qs-apiserver:      "; curl -s http://localhost:$(APISERVER_PORT)/healthz || echo "❌ 无响应"
	@echo -n "collection-server: "; curl -s http://localhost:$(COLLECTION_PORT)/healthz || echo "❌ 无响应"
	@echo -n "qs-worker:         "; \
		if [ -f $(PID_DIR)/worker.pid ] && kill -0 $$(cat $(PID_DIR)/worker.pid) 2>/dev/null; then \
			echo "✅ 运行中 (后台消费者)"; \
		else \
			echo "❌ 未运行"; \
		fi

# ============================================================================
# 基础设施检查
# ============================================================================

check-infra: ## 检查所有基础设施组件是否就绪
	@bash scripts/check-infra.sh all

check-mysql: ## 检查 MySQL 是否就绪
	@bash scripts/check-infra.sh mysql

check-redis: ## 检查 Redis 是否就绪
	@bash scripts/check-infra.sh redis

check-mongodb: ## 检查 MongoDB 是否就绪
	@bash scripts/check-infra.sh mongodb

check-nsq: ## 检查 NSQ 是否就绪
	@bash scripts/check-infra.sh nsq

# ============================================================================
# 开发工具
# ============================================================================

dev: ## 启动开发环境（热更新）
	@echo "🚀 启动开发环境..."
	@mkdir -p $(PID_DIR)
	@echo "启动 qs-apiserver..."
	@air -c .air-apiserver.toml & echo $$! > $(PID_DIR)/air-apiserver.pid
	@sleep 2
	@echo "启动 collection-server..."
	@air -c .air-collection.toml & echo $$! > $(PID_DIR)/air-collection.pid
	@sleep 2
	@echo "启动 qs-worker..."
	@air -c .air-worker.toml & echo $$! > $(PID_DIR)/air-worker.pid
	@sleep 2
	@echo "✅ 所有服务已启动（热更新模式）"
	@echo "提示：使用 Ctrl+C 停止所有服务"
	@echo "      或使用 make dev-stop 停止服务"

dev-apiserver: ## 独立启动 API 服务器（热更新）
	@echo "🚀 启动 qs-apiserver 开发环境..."
	@mkdir -p $(PID_DIR)
	@air -c .air-apiserver.toml

dev-collection: ## 独立启动 Collection 服务器（热更新）
	@echo "🚀 启动 collection-server 开发环境..."
	@mkdir -p $(PID_DIR)
	@air -c .air-collection.toml

dev-worker: ## 独立启动 Worker 服务（热更新）
	@echo "🚀 启动 qs-worker 开发环境..."
	@mkdir -p $(PID_DIR)
	@air -c .air-worker.toml

dev-stop: ## 停止开发环境
	@echo "⏹️  停止开发环境..."
	@if [ -f $(PID_DIR)/air-worker.pid ]; then \
		kill $$(cat $(PID_DIR)/air-worker.pid) 2>/dev/null || true; \
		rm -f $(PID_DIR)/air-worker.pid; \
	fi
	@if [ -f $(PID_DIR)/air-collection.pid ]; then \
		kill $$(cat $(PID_DIR)/air-collection.pid) 2>/dev/null || true; \
		rm -f $(PID_DIR)/air-collection.pid; \
	fi
	@if [ -f $(PID_DIR)/air-apiserver.pid ]; then \
		kill $$(cat $(PID_DIR)/air-apiserver.pid) 2>/dev/null || true; \
		rm -f $(PID_DIR)/air-apiserver.pid; \
	fi
	@echo "✅ 开发环境已停止"

dev-status: ## 查看开发环境状态
	@echo "📊 开发环境状态:"
	@echo "=============="
	@if [ -f $(PID_DIR)/air-apiserver.pid ] && kill -0 $$(cat $(PID_DIR)/air-apiserver.pid) 2>/dev/null; then \
		echo "✅ qs-apiserver      - 运行中 (PID: $$(cat $(PID_DIR)/air-apiserver.pid))"; \
	else \
		echo "⚪ qs-apiserver      - 未运行"; \
	fi
	@if [ -f $(PID_DIR)/air-collection.pid ] && kill -0 $$(cat $(PID_DIR)/air-collection.pid) 2>/dev/null; then \
		echo "✅ collection-server - 运行中 (PID: $$(cat $(PID_DIR)/air-collection.pid))"; \
	else \
		echo "⚪ collection-server - 未运行"; \
	fi
	@if [ -f $(PID_DIR)/air-worker.pid ] && kill -0 $$(cat $(PID_DIR)/air-worker.pid) 2>/dev/null; then \
		echo "✅ qs-worker         - 运行中 (PID: $$(cat $(PID_DIR)/air-worker.pid))"; \
	else \
		echo "⚪ qs-worker         - 未运行"; \
	fi

dev-logs: ## 查看开发环境日志
	@echo "📋 开发环境日志:"
	@echo "=============="
	@tail -f $(TMP_DIR)/build-errors-*.log

# ============================================================================
# 测试
# ============================================================================

test: ## 运行测试
	@echo "$(COLOR_CYAN)🧪 运行测试...$(COLOR_RESET)"
	@$(GO_TEST) ./...

test-unit: ## 运行单元测试
	@echo "$(COLOR_CYAN)🧪 运行单元测试...$(COLOR_RESET)"
	@$(GO_TEST) -v -short ./...

test-coverage: create-dirs ## 生成测试覆盖率报告
	@echo "$(COLOR_CYAN)🧪 生成测试覆盖率报告...$(COLOR_RESET)"
	@mkdir -p $(COVERAGE_DIR)
	@$(GO_TEST) -v -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	@$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "$(COLOR_GREEN)✅ 覆盖率报告: $(COVERAGE_DIR)/coverage.html$(COLOR_RESET)"
	@$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out | tail -n 1

test-race: ## 运行竞态检测
	@echo "$(COLOR_CYAN)🧪 运行竞态检测...$(COLOR_RESET)"
	@$(GO_TEST) -v -race ./...

test-bench: ## 运行基准测试
	@echo "$(COLOR_CYAN)🧪 运行基准测试...$(COLOR_RESET)"
	@$(GO_TEST) -v -bench=. -benchmem ./...

test-all: test test-race ## 运行所有测试

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

# ============================================================================
# 代码质量
# ============================================================================

lint: ## 运行代码检查
	@echo "$(COLOR_CYAN)🔍 运行代码检查...$(COLOR_RESET)"
	@if command -v golangci-lint > /dev/null 2>&1; then \
		golangci-lint run --timeout=5m ./...; \
	else \
		echo "$(COLOR_YELLOW)⚠️  golangci-lint 未安装，使用 go vet$(COLOR_RESET)"; \
		$(GO) vet ./...; \
	fi

fmt: ## 格式化代码
	@echo "$(COLOR_CYAN)✨ 格式化代码...$(COLOR_RESET)"
	@$(GO) fmt ./...
	@gofmt -s -w .
	@echo "$(COLOR_GREEN)✅ 代码格式化完成$(COLOR_RESET)"

fmt-check: ## 检查代码格式
	@echo "$(COLOR_CYAN)🔍 检查代码格式...$(COLOR_RESET)"
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "$(COLOR_RED)❌ 以下文件需要格式化:$(COLOR_RESET)"; \
		gofmt -l .; \
		exit 1; \
	else \
		echo "$(COLOR_GREEN)✅ 代码格式正确$(COLOR_RESET)"; \
	fi

# ============================================================================
# 依赖管理
# ============================================================================

deps: deps-download ## 下载依赖

deps-download: ## 下载所有依赖
	@echo "$(COLOR_CYAN)📦 下载依赖...$(COLOR_RESET)"
	@$(GO) mod download
	@echo "$(COLOR_GREEN)✅ 依赖下载完成$(COLOR_RESET)"

deps-tidy: ## 整理依赖
	@echo "$(COLOR_CYAN)🧹 整理依赖...$(COLOR_RESET)"
	@$(GO) mod tidy
	@echo "$(COLOR_GREEN)✅ 依赖整理完成$(COLOR_RESET)"

deps-verify: ## 验证依赖
	@echo "$(COLOR_CYAN)🔍 验证依赖...$(COLOR_RESET)"
	@$(GO) mod verify
	@echo "$(COLOR_GREEN)✅ 依赖验证通过$(COLOR_RESET)"

deps-check: ## 检查可更新的依赖
	@echo "$(COLOR_CYAN)🔍 检查依赖状态...$(COLOR_RESET)"
	@$(GO) list -u -m all | grep -v indirect || true
	@echo ""
	@echo "$(COLOR_YELLOW)说明: 后面有方括号 [...] 的表示有更新可用$(COLOR_RESET)"

# ============================================================================
# 工具安装
# ============================================================================

install-tools: ## 安装开发工具
	@echo "$(COLOR_CYAN)📦 安装开发工具...$(COLOR_RESET)"
	@echo "安装 Air (热更新)..."
	@$(GO) install github.com/air-verse/air@latest
	@echo "$(COLOR_GREEN)✅ 工具安装完成$(COLOR_RESET)"

install-air: ## 安装 Air 热更新工具
	@echo "📦 安装 Air..."
	@$(GO) install github.com/air-verse/air@latest

# ============================================================================
# 清理和维护
# ============================================================================

clean: ## 清理构建文件和进程
	@echo "🧹 清理构建文件和进程..."
	@$(MAKE) stop-all 2>/dev/null || true
	@$(MAKE) dev-stop 2>/dev/null || true
	@rm -rf $(TMP_DIR) $(BIN_DIR) $(LOG_DIR)/*.log
	@$(GO) clean
	@echo "✅ 清理完成"

create-dirs: ## 创建必要的目录
	@mkdir -p $(PID_DIR) $(LOG_DIR) $(BIN_DIR)

# ============================================================================
# 调试和诊断
# ============================================================================

debug: ## 显示调试信息
	@echo "$(COLOR_BOLD)$(COLOR_CYAN)🔍 调试信息:$(COLOR_RESET)"
	@echo "$(COLOR_BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(COLOR_RESET)"
	@echo "项目名称:     $(PROJECT_NAME)"
	@echo "版本:         $(VERSION)"
	@echo "Git 提交:     $(GIT_COMMIT)"
	@echo "Git 分支:     $(GIT_BRANCH)"
	@echo "构建时间:     $(BUILD_TIME)"
	@echo "Go 版本:      $(shell $(GO) version)"
	@echo "GOPATH:       $(shell go env GOPATH)"
	@echo "GOOS:         $(shell go env GOOS)"
	@echo "GOARCH:       $(shell go env GOARCH)"
	@echo "$(COLOR_BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(COLOR_RESET)"

# ============================================================================
# 快捷命令
# ============================================================================

up: run ## 启动服务（别名）
down: stop ## 停止服务（别名）
re: restart ## 重启服务（别名）
st: status ## 查看状态（别名）
log: logs ## 查看日志（别名）

# ============================================================================
# 结束
# ============================================================================
