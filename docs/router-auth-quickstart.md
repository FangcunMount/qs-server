# 🚀 路由器认证快速开始

## ⚡ 快速上手

### 1. 启动服务器

```bash
# 确保数据库正在运行
# 启动你的API服务器
go run cmd/qs-apiserver/apiserver.go --config configs/qs-apiserver.yaml
```

### 2. 测试公开端点

```bash
# 健康检查
curl http://localhost:8080/health

# 服务信息
curl http://localhost:8080/api/v1/public/info
```

### 3. 用户注册（如果需要）

```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "password123",
    "email": "test@example.com",
    "nickname": "Test User"
  }'
```

### 4. 用户登录获取令牌

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "password123"
  }'
```

**响应示例：**
```json
{
  "code": 200,
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expire": "2024-01-16T10:30:15Z",
  "user": {
    "id": 1,
    "username": "testuser",
    "nickname": "Test User"
  },
  "message": "Login successful"
}
```

### 5. 使用令牌访问受保护资源

```bash
# 保存令牌到变量
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# 获取用户资料
curl -X GET http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Bearer $TOKEN"

# 修改密码
curl -X POST http://localhost:8080/api/v1/users/change-password \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "old_password": "password123",
    "new_password": "newpassword456"
  }'
```

## 🛠️ 常用操作

### Basic认证方式
```bash
# 使用用户名密码直接认证（无需先登录）
curl -X GET http://localhost:8080/api/v1/users/profile \
  -u testuser:password123
```

### 查看所有可用端点
```bash
# 方法1：查看健康检查信息
curl http://localhost:8080/health

# 方法2：查看路由注册日志
# 在服务器启动时会显示：
# 🔗 Registered routes for: public, protected(user, questionnaire)
```

### JWT令牌刷新
```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Authorization: Bearer $TOKEN"
```

### 登出
```bash
curl -X POST http://localhost:8080/auth/logout \
  -H "Authorization: Bearer $TOKEN"
```

## 🔧 配置JWT密钥

在 `configs/qs-apiserver.yaml` 中配置JWT相关参数：

```yaml
# JWT配置（如果不存在请添加）
jwt:
  realm: "qs jwt"
  key: "your-secret-key-here"  # 请使用强密钥
  timeout: "24h"               # 令牌有效期
  max-refresh: "168h"          # 最大刷新时间（7天）
```

## ⚠️ 故障排除

### 问题1：认证失败
```bash
# 错误：{"code": 401, "message": "用户未认证"}
# 解决：检查令牌是否正确，是否已过期

# 获取新令牌
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"your-username","password":"your-password"}'
```

### 问题2：用户不存在
```bash
# 错误：用户不存在
# 解决：先注册用户或使用现有用户
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"newuser","password":"pass123","email":"user@test.com","nickname":"New User"}'
```

### 问题3：密码错误
```bash
# 错误：密码不正确
# 解决：确认密码是否正确，或重置密码
# （重置密码功能需要额外实现）
```

### 问题4：服务不可用
```bash
# 错误：认证服务不可用
# 解决：检查数据库连接和服务器配置

# 检查健康状态
curl http://localhost:8080/health
```

## 📝 测试脚本

创建一个测试脚本 `test-auth.sh`：

```bash
#!/bin/bash

BASE_URL="http://localhost:8080"
USERNAME="testuser"
PASSWORD="password123"

echo "🧪 开始认证测试..."

# 1. 健康检查
echo "1. 检查服务健康状态..."
curl -s "$BASE_URL/health" | jq .

# 2. 用户注册（如果用户不存在）
echo -e "\n2. 注册用户..."
curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\",\"email\":\"test@example.com\",\"nickname\":\"Test User\"}"

# 3. 用户登录
echo -e "\n3. 用户登录..."
RESPONSE=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}")

TOKEN=$(echo $RESPONSE | jq -r '.token')
echo "获取到令牌: $TOKEN"

# 4. 获取用户资料
echo -e "\n4. 获取用户资料..."
curl -s -X GET "$BASE_URL/api/v1/users/profile" \
  -H "Authorization: Bearer $TOKEN" | jq .

# 5. 测试Basic认证
echo -e "\n5. 测试Basic认证..."
curl -s -X GET "$BASE_URL/api/v1/users/profile" \
  -u "$USERNAME:$PASSWORD" | jq .

echo -e "\n✅ 认证测试完成！"
```

运行测试：
```bash
chmod +x test-auth.sh
./test-auth.sh
```

## 📊 API响应格式

### 成功响应
```json
{
  "code": 0,
  "data": { ... },
  "message": "操作成功"
}
```

### 错误响应
```json
{
  "code": 401,
  "message": "用户未认证"
}
```

### 登录响应
```json
{
  "code": 200,
  "token": "jwt-token-here",
  "expire": "2024-01-16T10:30:15Z",
  "user": {
    "id": 1,
    "username": "testuser",
    "nickname": "Test User"
  },
  "message": "Login successful"
}
```

通过这个快速开始指南，您可以立即开始使用路由器的认证功能！ 