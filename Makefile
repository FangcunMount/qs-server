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