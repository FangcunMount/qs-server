#!/bin/bash

# 测试答卷提交和消息队列
# 这个脚本向 collection-server 提交一个测试答卷
# 然后观察 evaluation-server 是否接收到消息

set -e

echo "📝 测试答卷提交"
echo "==============="

# 检查服务是否运行
check_service() {
    local service_name=$1
    local port=$2
    
    if ! curl -s "http://localhost:$port/healthz" > /dev/null 2>&1; then
        echo "❌ $service_name 服务未运行 (端口: $port)"
        echo "请先运行: ./test-message-queue.sh"
        exit 1
    fi
    echo "✅ $service_name 服务正在运行"
}

# 检查所有服务
check_service "collection-server" 8081
check_service "evaluation-server" 8082

echo ""
echo "🚀 提交测试答卷..."

# 构建测试答卷数据
ANSWERSHEET_DATA='{
  "questionnaire_code": "PHQ9",
  "title": "PHQ-9 抑郁症筛查量表",
  "writer_id": 1001,
  "testee_id": 2001,
  "testee_info": {
    "name": "张三",
    "age": 25,
    "gender": "male",
    "phone": "13800138000",
    "email": "zhangsan@example.com"
  },
  "answers": [
    {
      "question_id": "Q1",
      "value": 2,
      "question_type": "radio"
    },
    {
      "question_id": "Q2", 
      "value": 1,
      "question_type": "radio"
    },
    {
      "question_id": "Q3",
      "value": 3,
      "question_type": "radio"
    },
    {
      "question_id": "Q4",
      "value": 1,
      "question_type": "radio"
    },
    {
      "question_id": "Q5",
      "value": 2,
      "question_type": "radio"
    }
  ]
}'

# 提交答卷
echo "📤 向 collection-server 提交答卷..."
RESPONSE=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -d "$ANSWERSHEET_DATA" \
  "http://localhost:8081/api/v1/answersheets" \
  -w "\n%{http_code}")

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
RESPONSE_BODY=$(echo "$RESPONSE" | head -n -1)

echo "HTTP 状态码: $HTTP_CODE"
echo "响应内容: $RESPONSE_BODY"

if [ "$HTTP_CODE" = "200" ]; then
    echo "✅ 答卷提交成功！"
    echo ""
    echo "🔍 查看消息处理日志:"
    echo "   tail -f logs/evaluation-server.log"
    echo ""
    echo "📨 预期行为:"
    echo "   1. collection-server 发布 'answersheet.saved' 消息到 Redis"
    echo "   2. evaluation-server 订阅并接收消息"
    echo "   3. evaluation-server 处理消息并输出日志"
else
    echo "❌ 答卷提交失败！"
    echo "错误详情: $RESPONSE_BODY"
fi

echo ""
echo "🎯 测试完成！" 