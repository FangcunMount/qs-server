# 密码验证问题排查指南

## 🔍 问题现象
请求参数中密码是10位，但是系统报错。

## 📋 验证规则说明

系统中有三层密码验证：

### 1. HTTP请求验证层
```go
// UserCreateRequest 
Password string `json:"password" valid:"required,stringlength(6|50)"`
```
- **验证库**: `govalidator`
- **规则**: 密码长度6-50个字符
- **字符计算**: 按Unicode字符计算（不是字节）

### 2. 领域模型验证层
```go
// User.ChangePassword()
if len(newPassword) < 6 {
    return errors.WithCode(code.ErrUserBasicInfoInvalid, "password must be at least 6 characters")
}
```
- **规则**: 密码长度至少6个字符
- **字符计算**: 按Unicode字符计算

### 3. Router验证层（密码修改）
```go
// ChangePasswordRequest
NewPassword string `json:"new_password" binding:"required,min=6,max=50"`
```
- **验证库**: Gin binding
- **规则**: 密码长度6-50个字符

## 🚨 可能的问题原因

### 1. **其他字段验证失败**
密码验证通过，但其他必填字段可能有问题：

```json
{
  "username": "",           // ❌ 用户名为空
  "password": "1234567890", // ✅ 10位密码正常
  "nickname": "",           // ❌ 昵称为空
  "email": "invalid",       // ❌ 邮箱格式错误
  "phone": ""               // ❌ 手机号为空
}
```

### 2. **请求格式问题**
- Content-Type 不是 `application/json`
- JSON格式错误
- 字段名称拼写错误

### 3. **API端点错误**
确保使用正确的端点：
- 用户注册: `POST /auth/register`
- 密码修改: `POST /api/v1/users/change-password`

### 4. **字符编码问题**
包含特殊字符的密码可能有编码问题：
```
"密码123456"   // 包含中文
"pass@#$%"     // 包含特殊符号
"test🔐word"   // 包含emoji
```

## 🛠️ 排查步骤

### 步骤1: 检查完整请求
确保所有必填字段都提供：
```json
{
  "username": "testuser",
  "password": "1234567890",
  "nickname": "测试用户",
  "email": "test@example.com",
  "phone": "13800138000",
  "introduction": "可选字段"
}
```

### 步骤2: 验证字段格式
- **用户名**: 非空字符串
- **密码**: 6-50个字符
- **昵称**: 非空字符串
- **邮箱**: 有效的邮箱格式
- **手机**: 非空字符串

### 步骤3: 检查HTTP头
```bash
curl -H "Content-Type: application/json" \
     -X POST http://localhost:8080/auth/register \
     -d '{"username":"testuser","password":"1234567890",...}'
```

### 步骤4: 查看完整错误信息
不只看HTTP状态码，还要查看响应体中的详细错误信息。

## 🧪 测试用例

### 有效的10位密码请求
```bash
curl -H "Content-Type: application/json" \
     -X POST http://localhost:8080/auth/register \
     -d '{
       "username": "testuser10",
       "password": "1234567890",
       "nickname": "测试用户",
       "email": "test@example.com",
       "phone": "13800138000"
     }'
```

### 预期响应（成功）
```json
{
  "code": 0,
  "data": {
    "id": 1,
    "username": "testuser10",
    "nickname": "测试用户",
    "email": "test@example.com",
    "phone": "13800138000",
    "status": "init",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  },
  "message": "用户创建成功"
}
```

### 预期响应（验证失败）
```json
{
  "code": 400,
  "error": "请求格式错误",
  "details": "email: test@invalid does not validate as email"
}
```

## 🔧 修复建议

### 1. 如果是邮箱格式问题
确保邮箱包含 `@` 和有效域名：
```json
{
  "email": "user@example.com"  // ✅ 正确
  "email": "userexample.com"   // ❌ 缺少@
  "email": "user@"             // ❌ 缺少域名
}
```

### 2. 如果是字段缺失问题
检查所有必填字段：
```json
{
  "username": "必填",
  "password": "必填",
  "nickname": "必填", 
  "email": "必填",
  "phone": "必填"
}
```

### 3. 如果是密码特殊字符问题
尝试使用纯英文数字密码测试：
```json
{
  "password": "testpass123"  // 纯英文数字
}
```

## 📱 快速测试

使用提供的测试脚本：
```bash
./test_api.sh
```

该脚本会自动测试：
- 服务器连通性
- 10位密码用户创建
- 不同密码长度
- 字段验证
- 邮箱格式验证

## 🆘 如果问题仍然存在

请提供以下信息：
1. **完整的请求数据**（JSON格式）
2. **完整的错误响应**（包括状态码和响应体）
3. **使用的API端点**
4. **请求方法**（POST/PUT等）
5. **Content-Type头信息**

这样可以更精确地定位问题所在。 