#!/bin/bash

# 开发环境启动脚本
# 使用 Air 进行热更新

echo "🚀 启动开发环境..."

# 检查 Air 是否安装
if ! command -v air &> /dev/null; then
    echo "❌ Air 未安装，正在安装..."
    go install github.com/air-verse/air@latest
fi

# 检查必要的配置文件
if [ ! -f "configs/apiserver.yaml" ]; then
    echo "❌ 配置文件 configs/apiserver.yaml 不存在"
    exit 1
fi

# 创建临时目录
mkdir -p tmp

# 启动 Air
echo "✅ 启动热更新服务..."
air 