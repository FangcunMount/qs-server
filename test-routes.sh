#!/bin/bash

echo "🧪 路由测试脚本"
echo "================"

API_BASE="http://localhost:8080"

# 检查服务器是否运行
echo ""
echo "📡 检查服务器状态..."
if curl -s "$API_BASE/ping" > /dev/null; then
    echo "✅ 服务器运行正常"
else
    echo "❌ 服务器未运行，请先启动服务"
    exit 1
fi

echo ""
echo "🔍 测试各个路由..."

# 测试公开路由
echo ""
echo "1. 测试公开路由:"
echo "   GET /ping"
curl -s "$API_BASE/ping" | jq -r '.message // "ERROR"' | sed 's/^/   /'

echo "   GET /health"
curl -s "$API_BASE/health" | jq -r '.status // "ERROR"' | sed 's/^/   /'

echo "   GET /api/v1/public/info"
curl -s "$API_BASE/api/v1/public/info" | jq -r '.service // "ERROR"' | sed 's/^/   /'

# 测试认证路由（无认证，应该返回401）
echo ""
echo "2. 测试受保护路由（无认证，期望401）:"
echo "   GET /api/v1/users/profile"
HTTP_STATUS=$(curl -s -w "%{http_code}" -o /dev/null "$API_BASE/api/v1/users/profile")
echo "   HTTP状态码: $HTTP_STATUS"

echo "   GET /api/v1/users/123"
HTTP_STATUS=$(curl -s -w "%{http_code}" -o /dev/null "$API_BASE/api/v1/users/123")
echo "   HTTP状态码: $HTTP_STATUS"

# 尝试登录并获取token
echo ""
echo "3. 测试登录..."
LOGIN_RESPONSE=$(curl -s -w "\nHTTP_STATUS:%{http_code}" \
  -H "Content-Type: application/json" \
  -X POST "$API_BASE/auth/login" \
  -d '{
    "username": "testuser",
    "password": "1234567890"
  }')

HTTP_STATUS=$(echo "$LOGIN_RESPONSE" | grep "HTTP_STATUS" | cut -d: -f2)
RESPONSE_BODY=$(echo "$LOGIN_RESPONSE" | sed '/HTTP_STATUS/d')

echo "   登录状态码: $HTTP_STATUS"
if [ "$HTTP_STATUS" = "200" ]; then
    TOKEN=$(echo "$RESPONSE_BODY" | jq -r '.token // "none"')
    echo "   Token: ${TOKEN:0:30}..."
    
    # 使用token测试受保护路由
    echo ""
    echo "4. 测试带认证的受保护路由:"
    echo "   GET /api/v1/users/profile (with token)"
    HTTP_STATUS=$(curl -s -w "%{http_code}" -o /dev/null \
      -H "Authorization: Bearer $TOKEN" \
      "$API_BASE/api/v1/users/profile")
    echo "   HTTP状态码: $HTTP_STATUS"
    
    echo "   GET /api/v1/users/573055107243979310 (with token)"  
    HTTP_STATUS=$(curl -s -w "%{http_code}" -o /dev/null \
      -H "Authorization: Bearer $TOKEN" \
      "$API_BASE/api/v1/users/573055107243979310")
    echo "   HTTP状态码: $HTTP_STATUS"
else
    echo "   登录失败，无法测试认证路由"
    echo "   错误: $RESPONSE_BODY"
fi

echo ""
echo "5. 列出所有注册的路由（如果可能）:"
echo "   这需要查看服务器日志或添加路由调试端点"

echo ""
echo "🔚 测试完成" 