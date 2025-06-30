# 🔐 登录API使用指南

## 📋 接口概览

系统提供了JWT登录认证机制，支持多种参数组织方式。

### 🔗 端点信息
- **URL**: `POST /auth/login`
- **认证**: 无需认证（公开端点）
- **Content-Type**: `application/json`

## 📝 参数组织方式

### 方式1: JSON请求体 (推荐)

**请求格式：**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "your_username",
    "password": "your_password"
  }'
```

**请求体结构：**
```json
{
  "username": "string (必填)",
  "password": "string (必填, 6-50字符)"
}
```

### 方式2: Basic Authentication Header

**请求格式：**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Authorization: Basic $(echo -n 'username:password' | base64)"
```

**Base64编码示例：**
```bash
# 用户名: testuser, 密码: 1234567890
echo -n 'testuser:1234567890' | base64
# 输出: dGVzdHVzZXI6MTIzNDU2Nzg5MA==

curl -X POST http://localhost:8080/auth/login \
  -H "Authorization: Basic dGVzdHVzZXI6MTIzNDU2Nzg5MA=="
```

## 📤 响应格式

### 登录成功 (200)
```json
{
  "code": 200,
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expire": "2024-01-02T12:34:56Z",
  "user": {
    "id": 1,
    "username": "testuser",
    "nickname": "测试用户",
    "email": "test@example.com",
    "phone": "13800138000",
    "status": "active"
  },
  "message": "Login successful"
}
```

### 登录失败 (401)
```json
{
  "code": 401,
  "message": "Authentication failed"
}
```

### 请求格式错误 (400)
```json
{
  "code": 400,
  "message": "Invalid request format",
  "details": "username: non zero value required"
}
```

## 🧪 测试用例

### 1. 基本登录测试
```bash
# 创建测试用户（如果还没有）
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "1234567890",
    "nickname": "测试用户",
    "email": "test@example.com",
    "phone": "13800138000"
  }'

# 登录测试
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "1234567890"
  }'
```

### 2. 错误情况测试

**用户名为空：**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "",
    "password": "1234567890"
  }'
```

**密码错误：**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "wrongpassword"
  }'
```

**密码过短：**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "123"
  }'
```

## 🔄 使用JWT Token

登录成功后，您可以使用返回的token访问受保护的API：

```bash
# 使用token访问受保护的端点
TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

curl -X GET http://localhost:8080/api/v1/users/profile \
  -H "Authorization: Bearer $TOKEN"
```

## 📱 相关端点

### Token刷新
```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Authorization: Bearer $TOKEN"
```

### 退出登录
```bash
curl -X POST http://localhost:8080/auth/logout \
  -H "Authorization: Bearer $TOKEN"
```

## 🛠️ 开发调试

### 检查JWT Token内容
```bash
# 安装jwt-cli (可选)
cargo install jwt-cli

# 解码token查看内容
jwt decode $TOKEN
```

### 查看Token过期时间
```bash
# 使用jq解析响应
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"1234567890"}' \
  | jq '.expire'
```

## ⚠️ 注意事项

1. **密码长度**: 6-50个字符
2. **Token有效期**: 默认24小时（可配置）
3. **安全建议**: 
   - 生产环境使用HTTPS
   - 妥善保管JWT token
   - 定期刷新token
   - 登出时清除本地token

## 🔧 常见问题

### Q: 登录后如何使用token？
A: 在请求头中添加 `Authorization: Bearer <token>`

### Q: Token过期怎么办？
A: 使用 `/auth/refresh` 端点刷新token，或重新登录

### Q: 支持记住登录吗？
A: 系统会设置cookie，支持一定程度的记住登录

### Q: 如何检查token是否有效？
A: 访问任何受保护的端点，如 `/api/v1/users/profile`

## 📊 完整示例脚本

```bash
#!/bin/bash

API_BASE="http://localhost:8080"

echo "=== 登录API测试 ==="

# 1. 登录获取token
echo "1. 登录中..."
RESPONSE=$(curl -s -X POST "$API_BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "password": "1234567890"
  }')

echo "登录响应:"
echo "$RESPONSE" | jq .

# 2. 提取token
TOKEN=$(echo "$RESPONSE" | jq -r '.token')
echo "Token: $TOKEN"

# 3. 使用token访问受保护资源
echo ""
echo "2. 访问用户资料..."
curl -s -X GET "$API_BASE/api/v1/users/profile" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .

# 4. 刷新token
echo ""
echo "3. 刷新token..."
curl -s -X POST "$API_BASE/auth/refresh" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .

# 5. 退出登录
echo ""
echo "4. 退出登录..."
curl -s -X POST "$API_BASE/auth/logout" \
  -H "Authorization: Bearer $TOKEN" \
  | jq .
``` 