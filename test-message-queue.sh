#!/bin/bash

# 测试消息队列系统
# 这个脚本演示了完整的消息队列流程：
# 1. 启动 Redis 服务器
# 2. 启动 evaluation-server (订阅者)
# 3. 启动 collection-server (发布者)
# 4. 模拟提交答卷，触发消息发布

set -e

echo "🚀 测试消息队列系统"
echo "===================="

# 检查 Redis 是否运行
check_redis() {
    if ! pgrep redis-server > /dev/null; then
        echo "❌ Redis 服务器未运行"
        echo "请先启动 Redis 服务器: redis-server"
        exit 1
    fi
    echo "✅ Redis 服务器正在运行"
}

# 检查服务是否启动
check_service() {
    local service_name=$1
    local port=$2
    local max_attempts=10
    local attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        if curl -s "http://localhost:$port/healthz" > /dev/null 2>&1; then
            echo "✅ $service_name 服务已启动"
            return 0
        fi
        echo "⏳ 等待 $service_name 服务启动... ($((attempt + 1))/$max_attempts)"
        sleep 2
        ((attempt++))
    done
    
    echo "❌ $service_name 服务启动失败"
    return 1
}

# 清理进程
cleanup() {
    echo "🧹 清理进程..."
    pkill -f "evaluation-server" || true
    pkill -f "collection-server" || true
    pkill -f "qs-apiserver" || true
    sleep 2
}

# 注册清理函数
trap cleanup EXIT

# 1. 检查 Redis
check_redis

# 2. 启动 apiserver
echo "🔧 启动 apiserver..."
./qs-apiserver --config=configs/apiserver.dev.yaml > logs/apiserver.log 2>&1 &
APISERVER_PID=$!
echo "apiserver PID: $APISERVER_PID"

# 等待 apiserver 启动
sleep 3
check_service "apiserver" 8080

# 3. 启动 evaluation-server (订阅者)
echo "📨 启动 evaluation-server (订阅者)..."
./evaluation-server --config=configs/evaluation-server.yaml > logs/evaluation-server.log 2>&1 &
EVALUATION_PID=$!
echo "evaluation-server PID: $EVALUATION_PID"

# 等待 evaluation-server 启动
sleep 3
check_service "evaluation-server" 8082

# 4. 启动 collection-server (发布者)
echo "📡 启动 collection-server (发布者)..."
./collection-server --config=configs/collection-server.dev.yaml > logs/collection-server.log 2>&1 &
COLLECTION_PID=$!
echo "collection-server PID: $COLLECTION_PID"

# 等待 collection-server 启动
sleep 3
check_service "collection-server" 8081

echo ""
echo "🎉 所有服务已启动成功！"
echo "===================="
echo "📋 服务状态:"
echo "   - apiserver:        http://localhost:8080"
echo "   - collection-server: http://localhost:8081"
echo "   - evaluation-server: http://localhost:8082"
echo ""
echo "📨 测试消息队列:"
echo "   现在可以通过 collection-server 提交答卷"
echo "   evaluation-server 将自动接收并处理消息"
echo ""
echo "🔍 查看日志:"
echo "   - apiserver:         tail -f logs/apiserver.log"
echo "   - collection-server: tail -f logs/collection-server.log"
echo "   - evaluation-server: tail -f logs/evaluation-server.log"
echo ""
echo "⏹️  按 Ctrl+C 停止所有服务"

# 保持脚本运行
wait 