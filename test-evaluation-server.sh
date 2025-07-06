#!/bin/bash

# 测试 evaluation-server 的启动脚本

echo "🚀 Testing Evaluation Server..."

# 构建服务
echo "📦 Building evaluation-server..."
go build -o evaluation-server ./cmd/evaluation-server

if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    exit 1
fi

echo "✅ Build successful"

# 测试健康检查（在后台启动服务器）
echo "🏃 Starting evaluation-server in background..."
./evaluation-server \
    --insecure.bind-port=8082 \
    --grpc-client.endpoint=localhost:8090 \
    --grpc-client.timeout=30 \
    --grpc-client.insecure=true \
    --message-queue.type=redis \
    --message-queue.endpoint=localhost:6379 \
    --message-queue.topic=answersheet_saved \
    --message-queue.group=evaluation_group \
    --log.level=info \
    --server.mode=debug &

SERVER_PID=$!
echo "📋 Server PID: $SERVER_PID"

# 等待服务器启动
sleep 3

# 测试健康检查
echo "🔍 Testing health check..."
curl -s http://localhost:8082/healthz

echo ""
echo "🔍 Testing readiness check..."
curl -s http://localhost:8082/readyz

echo ""
echo "🔍 Testing status check..."
curl -s http://localhost:8082/status

echo ""

# 停止服务器
echo "🛑 Stopping server..."
kill $SERVER_PID

echo "✅ Test completed" 