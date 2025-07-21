# Collection Server Interface Layer 重构文档

## 📋 重构概述

本次重构完成了 Collection Server 接口层的规范化和模块化设计，采用了 RESTful API 标准和清晰的分层架构。

## 🏗️ 架构设计

### 目录结构

```
interface/
└── restful/
    ├── router.go           # 路由配置和中间件设置
    ├── handler/            # 处理器（现有）
    │   ├── questionnaire_handler.go
    │   └── answersheet_handler.go
    ├── request/            # 请求模型
    │   ├── questionnaire.go
    │   └── answersheet.go
    └── response/           # 响应模型
        ├── questionnaire.go
        └── answersheet.go
```

## 🔧 核心组件

### 1. 路由配置 (`router.go`)

#### 特性
- **配置驱动**：支持灵活的中间件配置
- **模块化路由**：按业务域分组路由
- **健康检查**：完整的监控端点
- **标准化响应**：统一的 API 响应格式

#### 路由分组
```go
// API 路由
/api/v1/questionnaire/*    # 问卷相关路由
/api/v1/answersheet/*      # 答卷相关路由
/api/v1/public/*           # 公共信息路由

// 监控路由
/health, /healthz, /ping   # 健康检查
/ready, /live              # 就绪和存活检查
```

#### 中间件集成
```go
// 使用 internal/pkg/middleware
r.engine.Use(middleware.RequestID())   # 请求ID
r.engine.Use(middleware.Logger())      # 日志记录
r.engine.Use(middleware.Cors())        # CORS处理
r.engine.Use(middleware.Secure)        # 安全头部
r.engine.Use(middleware.NoCache)       # 缓存控制
r.engine.Use(middleware.Options)       # OPTIONS处理
```

### 2. 请求模型 (`request/`)

#### 设计原则
- **验证完备**：完整的 binding 验证规则
- **类型安全**：强类型定义，避免运行时错误
- **分层清晰**：按业务域组织模型
- **扩展友好**：支持未来功能扩展

#### 问卷请求模型
```go
// 获取问卷
type QuestionnaireGetRequest struct {
    Code string `uri:"code" binding:"required"`
}

// 提交问卷
type AnswersheetSubmitRequest struct {
    QuestionnaireCode string        `json:"questionnaire_code" binding:"required,min=3,max=50"`
    TesteeInfo        TesteeInfo    `json:"testee_info" binding:"required"`
    Answers           []AnswerValue `json:"answers" binding:"required,min=1"`
    // ...
}
```

#### 验证特性
- **必填验证**：`binding:"required"`
- **长度限制**：`binding:"min=3,max=50"`
- **格式验证**：`binding:"email"`, `binding:"numeric"`
- **枚举验证**：`binding:"oneof=male female other"`

### 3. 响应模型 (`response/`)

#### 设计特点
- **一致性**：统一的响应结构
- **完整性**：包含所有必要信息
- **可扩展性**：支持添加新字段
- **类型安全**：明确的数据类型

#### 核心响应类型
```go
// 问卷详细响应
type QuestionnaireResponse struct {
    Code         string      `json:"code"`
    Title        string      `json:"title"`
    Questions    []Question  `json:"questions"`
    Settings     Settings    `json:"settings"`
    CreatedAt    time.Time   `json:"created_at"`
    // ...
}

// 答卷提交响应
type AnswersheetSubmitResponse struct {
    ID               string            `json:"id"`
    Status           string            `json:"status"`
    ValidationStatus string            `json:"validation_status"`
    ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
    NextSteps        []NextStep        `json:"next_steps,omitempty"`
    // ...
}
```

## 🚀 使用方式

### 1. 创建路由器

```go
import (
    "github.com/yshujie/questionnaire-scale/internal/collection-server/interface/restful"
    "github.com/yshujie/questionnaire-scale/internal/collection-server/interface/restful/handler"
)

// 创建处理器
questionnaireHandler := handler.NewQuestionnaireHandler(...)
answersheetHandler := handler.NewAnswersheetHandler(...)

// 创建路由器
router := restful.NewRouter(
    nil, // 使用默认配置
    questionnaireHandler,
    answersheetHandler,
)

// 设置路由和中间件
engine := router.Setup()
```

### 2. 配置中间件

```go
config := &restful.RouterConfig{
    EnableCORS:       true,
    EnableAuth:       false, // collection-server 通常不需要认证
    EnableLogging:    true,
    EnableValidation: true,
    APIVersion:       "v1",
    APIPrefix:        "/api",
}

router := restful.NewRouter(config, questionnaireHandler, answersheetHandler)
```

### 3. 处理请求

```go
// 在 handler 中使用请求模型
func (h *Handler) SubmitAnswersheet(c *gin.Context) {
    var req request.AnswersheetSubmitRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // 处理验证错误
        return
    }
    
    // 业务处理
    result, err := h.service.Submit(ctx, req)
    
    // 返回响应
    resp := response.AnswersheetSubmitResponse{
        ID:     result.ID,
        Status: result.Status,
        // ...
    }
    c.JSON(http.StatusOK, resp)
}
```

## 📊 API 规范

### 请求格式
```json
{
  "questionnaire_code": "depression-scale-v1",
  "testee_info": {
    "name": "张三",
    "gender": "male",
    "age": 25
  },
  "answers": [
    {
      "question_code": "q1",
      "value": "never"
    }
  ]
}
```

### 响应格式
```json
{
  "id": "as_1234567890",
  "questionnaire_code": "depression-scale-v1",
  "status": "completed",
  "validation_status": "valid",
  "submission_time": "2024-07-21T10:30:00Z",
  "next_steps": [
    {
      "type": "evaluation",
      "description": "等待系统计算结果"
    }
  ],
  "message": "答卷提交成功"
}
```

### 错误响应
```json
{
  "error": "validation_failed",
  "message": "请求参数验证失败",
  "details": {
    "questionnaire_code": "此字段为必填项",
    "answers": "至少需要一个答案"
  }
}
```

## 🔍 监控端点

| 端点 | 描述 | 响应 |
|------|------|------|
| `/health` | 综合健康检查 | 服务状态和组件检查 |
| `/ping` | 连通性测试 | `{"message": "pong"}` |
| `/ready` | 就绪检查 | 服务是否准备接受请求 |
| `/live` | 存活检查 | 服务是否运行正常 |
| `/api/v1/public/info` | 服务信息 | 版本和端点信息 |

## ✅ 重构优势

### 1. 规范化
- **标准化路由**：符合 RESTful 设计原则
- **统一验证**：使用 binding 标签进行输入验证
- **一致响应**：标准化的 API 响应格式

### 2. 可维护性
- **分层清晰**：请求、处理、响应分层
- **类型安全**：强类型模型减少运行时错误
- **文档完整**：完整的结构体文档

### 3. 可扩展性
- **配置驱动**：灵活的中间件和路由配置
- **模块化**：按业务域组织，便于扩展
- **兼容性**：向后兼容的 API 设计

### 4. 开发效率
- **自动验证**：输入参数自动验证
- **类型提示**：IDE 完整的类型提示和补全
- **错误处理**：统一的错误处理和响应

## 🔧 中间件系统

### 已集成中间件
- **RequestID**：为每个请求生成唯一ID
- **Logger**：记录请求日志和性能指标
- **CORS**：处理跨域请求
- **Secure**：添加安全头部
- **NoCache**：控制缓存策略
- **Options**：处理 OPTIONS 预检请求

### 可选中间件
根据需要可以添加：
- **认证中间件**：使用 `internal/pkg/middleware/auth`
- **限流中间件**：防止 API 滥用
- **压缩中间件**：响应内容压缩

## 📝 后续优化

1. **Handler 重构**：使用新的请求/响应模型更新现有 handler
2. **验证增强**：添加更多自定义验证规则
3. **文档生成**：基于结构体标签自动生成 API 文档
4. **测试覆盖**：为所有请求/响应模型添加单元测试
5. **性能优化**：路由性能优化和缓存策略

---

**重构完成时间**: 2024-07-21
**架构模式**: RESTful API + 分层架构
**技术栈**: Gin + 标准化中间件 + 强类型模型 