# Collection Server 目录结构重构计划

## 架构理解

### Collection Server 的核心职责
Collection Server 是一个**轻量级的收集服务**，主要负责：
1. **接收问卷提交**：通过 REST API 接收小程序等客户端的问卷提交
2. **验证答卷数据**：对提交的答卷进行验证
3. **转发到 apiserver**：通过 gRPC 将验证通过的答卷保存到 apiserver
4. **发布消息**：发布答卷保存事件到消息队列

### 数据流向
```
小程序/客户端 → Collection Server (REST API) → 验证 → gRPC → apiserver
                                    ↓
                              消息队列 → evaluation-server
```

## 当前问题分析

### 1. 验证模块过于集中
- `application/validation/` 目录包含 8 个文件
- 并发版本和串行版本混在一起
- 职责边界不清晰

### 2. 缺乏清晰的业务模块
- 没有明确的问卷和答卷业务模块
- 验证逻辑与业务逻辑混合

### 3. 接口层结构简单
- 缺少中间件、请求/响应模型
- 错误处理不够规范

## 重构目标

### 1. 明确业务边界
- 问卷管理：获取问卷信息、验证问卷代码
- 答卷处理：接收、验证、保存答卷
- 验证服务：独立的验证模块

### 2. 优化验证架构
- 分离并发和串行验证
- 配置驱动的验证策略选择
- 清晰的验证规则管理

### 3. 改进接口层
- 添加中间件
- 规范化请求/响应模型
- 统一的错误处理

## 新的目录结构

```
internal/collection-server/
├── app.go                    # 应用入口
├── run.go                    # 运行逻辑
├── server.go                 # 服务器配置
├── routers.go                # 路由配置
├── config/                   # 配置管理
│   └── config.go
├── options/                  # 命令行选项
│   └── options.go
├── container/                # 依赖注入容器
│   └── container.go
├── domain/                   # 领域层（轻量级）
│   ├── questionnaire/        # 问卷领域概念 ✅ 已完成
│   │   ├── entity.go         # 问卷实体 ✅
│   │   ├── validator.go      # 问卷验证器 ✅
│   │   └── adapter.go        # 适配器（实现接口） ✅
│   ├── answersheet/          # 答卷领域概念 ✅ 已完成
│   │   ├── entity.go         # 答卷实体 ✅
│   │   ├── validator.go      # 答卷验证器 ✅
│   │   └── README.md         # 使用文档 ✅
│   └── validation/           # 验证领域 ✅ 已完成重构
│       ├── validator.go      # 核心验证器 ✅
│       ├── builders.go       # 规则构建器 ✅
│       ├── validation_test.go # 测试文件 ✅
│       ├── README.md         # 架构文档 ✅
│       ├── rules/            # 验证规则 ✅
│       │   ├── rule.go       # 基础规则接口和类型 ✅
│       │   ├── required.go   # 必填验证规则 ✅
│       │   ├── length.go     # 长度验证规则 ✅
│       │   └── value.go      # 数值验证规则 ✅
│       └── strategies/       # 验证策略 ✅
│           ├── strategy.go   # 策略接口和工厂 ✅
│           ├── strategies.go # 具体策略实现 ✅
│           └── factory.go    # 策略工厂 ✅
├── application/              # 应用层
│   ├── questionnaire/        # 问卷应用服务
│   │   ├── service.go        # 问卷服务（gRPC调用）
│   │   └── dto.go           # 数据传输对象
│   ├── answersheet/          # 答卷应用服务
│   │   ├── service.go        # 答卷服务（gRPC调用）
│   │   └── dto.go           # 数据传输对象
│   └── validation/           # 验证应用服务
│       ├── service.go        # 验证服务接口
│       ├── concurrent/       # 并发验证实现
│       │   ├── validator.go
│       │   ├── service.go
│       │   └── adapter.go
│       ├── sequential/       # 串行验证实现
│       │   ├── validator.go
│       │   └── service.go
│       └── factory.go        # 验证服务工厂
├── infrastructure/           # 基础设施层
│   ├── grpc/                 # gRPC 客户端
│   │   ├── client_factory.go # 客户端工厂
│   │   ├── questionnaire_client.go
│   │   └── answersheet_client.go
│   └── pubsub/               # 消息发布
│       └── publisher.go
├── interface/                # 接口层
│   └── restful/              # REST API
│       ├── router.go         # 路由配置
│       ├── middleware/       # 中间件
│       │   ├── auth.go       # 认证中间件
│       │   ├── cors.go       # CORS中间件
│       │   ├── logging.go    # 日志中间件
│       │   └── validation.go # 请求验证中间件
│       ├── handler/          # 处理器
│       │   ├── questionnaire_handler.go
│       │   └── answersheet_handler.go
│       ├── request/          # 请求模型
│       │   ├── questionnaire.go
│       │   └── answersheet.go
│       └── response/         # 响应模型
│           ├── questionnaire.go
│           └── answersheet.go
└── docs/                     # 文档
    ├── README.md
    ├── architecture.md
    ├── api.md
    └── validation/
        ├── concurrent.md
        └── rules.md
```

## 业务模块设计

### 1. 问卷模块 (Questionnaire) ✅ 已完成
```go
// 职责：问卷信息获取和验证
type QuestionnaireService interface {
    GetQuestionnaire(ctx context.Context, code string) (*Questionnaire, error)
    ValidateQuestionnaireCode(ctx context.Context, code string) error
}
```

**实现特点：**
- 使用 protobuf 转换器从 gRPC 数据转换为领域实体
- 提供适配器实现接口，避免循环导入
- 支持问卷代码和实体验证

### 2. 答卷模块 (Answersheet) ✅ 已完成
```go
// 职责：答卷接收、验证、保存
type AnswersheetService interface {
    SubmitAnswersheet(ctx context.Context, req *SubmitRequest) (*SubmitResponse, error)
    ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error
}
```

**实现特点：**
- **基于问题验证规则的动态验证**：每个答案根据对应问题的验证规则进行校验
- 使用接口避免循环导入问题
- 支持多种问题类型的验证（文本、数值、单选、多选）
- 提供详细的验证错误信息

### 3. 验证模块 (Validation) ✅ 已存在
```go
// 职责：独立的验证服务
type ValidationService interface {
    ValidateAnswers(ctx context.Context, answers []Answer, questionnaire *Questionnaire) error
    GetValidationStrategy() ValidationStrategy
}
```

## 重构进度

### ✅ 已完成 - 领域层重构

#### 1. 问卷领域模块
- [x] 创建问卷实体 (`domain/questionnaire/entity.go`)
- [x] 实现 protobuf 转换器
- [x] 创建问卷验证器 (`domain/questionnaire/validator.go`)
- [x] 创建适配器 (`domain/questionnaire/adapter.go`)

#### 2. 答卷领域模块
- [x] 创建答卷实体 (`domain/answersheet/entity.go`)
- [x] 创建答卷验证器 (`domain/answersheet/validator.go`)
- [x] 实现基于问题验证规则的动态验证
- [x] 创建使用文档 (`domain/answersheet/README.md`)

#### 3. 验证架构设计 ✅ 已完成重构
- [x] 使用接口避免循环导入
- [x] 实现验证规则转换机制
- [x] 支持多种问题类型验证
- [x] 提供详细的错误处理
- [x] 重构为基于策略模式的验证架构
- [x] 实现验证规则和策略分离
- [x] 创建规则构建器模式
- [x] 添加完整的测试覆盖
- [x] 编写详细的架构文档

### 🔄 进行中 - 应用层重构

#### 1. 问卷应用服务
- [ ] 创建问卷服务接口
- [ ] 实现 gRPC 调用逻辑
- [ ] 创建数据传输对象

#### 2. 答卷应用服务
- [ ] 创建答卷服务接口
- [ ] 实现 gRPC 调用逻辑
- [ ] 集成领域验证器

#### 3. 验证应用服务
- [ ] 重构现有验证服务
- [ ] 分离并发和串行实现
- [ ] 创建验证服务工厂

### ⏳ 待开始 - 基础设施层重构

#### 1. gRPC 客户端
- [ ] 创建客户端工厂
- [ ] 实现问卷客户端
- [ ] 实现答卷客户端

#### 2. 消息发布
- [ ] 创建消息发布器
- [ ] 实现事件发布逻辑

### ⏳ 待开始 - 接口层重构

#### 1. REST API
- [ ] 创建路由配置
- [ ] 添加中间件
- [ ] 规范化请求/响应模型

#### 2. 处理器
- [ ] 重构问卷处理器
- [ ] 重构答卷处理器
- [ ] 集成新的应用服务

## 核心设计亮点

### 1. 基于问题验证规则的动态验证

这是本次重构的核心创新点：

```go
// 验证单个答案时，根据问题的验证规则进行验证
func (v *Validator) ValidateAnswer(ctx context.Context, answer *Answer, question QuestionInfo) error {
    // 1. 问题匹配验证
    // 2. 基础验证
    // 3. 规则验证 - 根据问题的验证规则进行验证
    if len(question.GetValidationRules()) > 0 {
        rules := v.convertValidationRules(question.GetValidationRules())
        errors := v.validationValidator.ValidateMultiple(answer.Value, rules)
        // ...
    }
    // 4. 类型验证
}
```

**优势：**
- 每个答案都根据其对应问题的具体验证规则进行校验
- 支持灵活的验证规则配置
- 验证逻辑与问题定义紧密耦合

### 2. 接口设计避免循环导入

通过定义接口和适配器模式，避免了循环导入问题：

```go
// 在 answersheet 包中定义接口
type QuestionInfo interface {
    GetCode() string
    GetType() string
    GetOptions() []QuestionOption
    GetValidationRules() []QuestionValidationRule
}

// 在 questionnaire 包中实现适配器
type QuestionAdapter struct {
    question *Question
}

func (a *QuestionAdapter) GetCode() string {
    return a.question.Code
}
```

### 3. 验证规则转换机制

自动将问卷中的验证规则转换为验证器的规则：

```go
func (v *Validator) convertValidationRule(protoRule QuestionValidationRule) *validation.ValidationRule {
    switch protoRule.GetRuleType() {
    case "required":
        return validation.NewValidationRule("required", nil, "此题为必答题")
    case "min_length":
        return validation.NewValidationRule("min_length", protoRule.GetTargetValue(), "答案长度不能少于指定字符数")
    // ...
    }
}
```

## 下一步计划

### 第二阶段：应用层重构
1. 创建问卷和答卷应用服务
2. 重构验证应用服务
3. 实现验证服务工厂

### 第三阶段：基础设施层重构
1. 重构 gRPC 客户端
2. 实现消息发布

### 第四阶段：接口层重构
1. 添加中间件
2. 规范化请求/响应模型
3. 重构处理器

## 优势

### 1. 清晰的业务边界
- 问卷管理：独立的问卷服务
- 答卷处理：独立的答卷服务
- 验证服务：可配置的验证策略

### 2. 更好的可维护性
- 模块化设计
- 职责分离
- 易于测试

### 3. 支持策略切换
- 并发/串行验证可配置
- 验证规则可扩展
- 中间件可插拔

### 4. 提高开发效率
- 直观的目录结构
- 清晰的命名约定
- 完善的文档 