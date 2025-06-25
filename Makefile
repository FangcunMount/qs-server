.PHONY: help dev build clean test

# 默认目标
help: ## 显示帮助信息
	@echo "可用的命令:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev: ## 启动开发环境（热更新）
	@echo "🚀 启动开发环境..."
	@mkdir -p tmp
	@air

build: ## 构建应用
	@echo "🔨 构建应用..."
	@go build -o bin/qs-apiserver ./cmd/qs-apiserver

run: ## 运行应用
	@echo "▶️  运行应用..."
	@go run ./cmd/qs-apiserver/ --config=configs/qs-apiserver.yaml

test: ## 运行测试
	@echo "🧪 运行测试..."
	@go test ./...

clean: ## 清理构建文件
	@echo "🧹 清理构建文件..."
	@rm -rf tmp bin
	@go clean

install-air: ## 安装 Air 热更新工具
	@echo "📦 安装 Air..."
	@go install github.com/air-verse/air@latest

deps: ## 安装依赖
	@echo "📦 安装依赖..."
	@go mod download
	@go mod tidy 

# 数据库相关命令
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