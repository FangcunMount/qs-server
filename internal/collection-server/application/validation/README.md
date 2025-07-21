# Application Layer Validation 应用验证层

## 概述

本模块实现了 collection-server 应用层的验证服务，提供了灵活、高效的答卷验证功能。支持串行和并发两种验证策略，通过工厂模式和适配器模式实现了良好的架构设计。

## 架构设计

### 目录结构

```
application/validation/
├── service.go              # 主验证服务接口和适配器
├── factory.go              # 验证服务工厂
├── concurrent/             # 并发验证实现
│   ├── service.go          # 并发验证服务
│   ├── validator.go        # 并发验证器
│   └── adapter.go          # 并发验证适配器
├── sequential/             # 串行验证实现
│   ├── service.go          # 串行验证服务
│   └── validator.go        # 串行验证器
└── README.md               # 本文档
```

### 核心组件

#### 1. 主服务接口
- **Service**: 统一验证服务接口
- **ServiceConcurrent**: 并发验证服务接口
- **ServiceAdapter**: 服务适配器

#### 2. 工厂模式
- **ServiceFactory**: 验证服务工厂
- 支持创建串行和并发验证服务
- 支持配置驱动的服务创建

#### 3. 验证策略
- **Sequential**: 串行验证策略
- **Concurrent**: 并发验证策略（支持可配置并发数）

#### 4. 请求转换
- **ValidationRequest**: 统一验证请求结构
- 支持自动转换为串行/并发请求格式

## 设计模式

### 1. 工厂模式
```go
factory := validation.NewServiceFactory(questionnaireService)

// 创建串行服务
sequentialService := factory.CreateSequentialService()

// 创建并发服务
concurrentService := factory.CreateConcurrentService(maxConcurrency)
```

### 2. 适配器模式
```go
// 将并发服务适配为通用服务接口
adapter := validation.NewServiceAdapter(concurrentService)

// 统一调用接口
err := adapter.ValidateAnswersheet(ctx, request)
```

### 3. 策略模式
```go
// 根据配置选择验证策略
if config.UseConcurrentValidation {
    service = factory.CreateConcurrentService(config.MaxConcurrency)
} else {
    service = factory.CreateSequentialService()
}
```

## 使用方式

### 1. 基本使用

```go
import (
    "github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation"
    "github.com/yshujie/questionnaire-scale/internal/collection-server/application/questionnaire"
)

// 创建问卷服务（依赖）
questionnaireService := questionnaire.NewService(grpcClient)

// 创建验证服务工厂
factory := validation.NewServiceFactory(questionnaireService)

// 创建验证服务（选择策略）
validationService := factory.CreateConcurrentService(10) // 并发验证，最大并发数10

// 构建验证请求
req := &validation.ValidationRequest{
    QuestionnaireCode: "sample-questionnaire",
    Title:             "样例问卷",
    TesteeInfo: &validation.TesteeInfo{
        Name:  "张三",
        Email: "zhangsan@example.com",
    },
    Answers: []*validation.AnswerValidationItem{
        {
            QuestionCode: "q1",
            QuestionType: "text",
            Value:        "这是文本答案",
        },
        {
            QuestionCode: "q2", 
            QuestionType: "number",
            Value:        85,
        },
    },
}

// 执行验证
err := validationService.ValidateAnswersheet(ctx, req)
if err != nil {
    log.Printf("验证失败: %v", err)
}
```

### 2. Container集成

```go
// 在 Container 中使用
func (c *Container) initializeApplication() error {
    // 创建问卷验证器
    questionnaireValidator := validation.NewQuestionnaireValidator(c.QuestionnaireClient)
    
    // 创建验证规则工厂
    ruleFactory := validation.NewDefaultValidationRuleFactory()
    
    // 创建答案验证器（并发版本）
    answerValidatorConcurrent := validation.NewAnswerValidatorConcurrent(
        ruleFactory, 
        c.concurrencyConfig.MaxConcurrency,
    )
    
    // 创建并发校验服务
    concurrentService := validation.NewServiceConcurrent(
        questionnaireValidator, 
        answerValidatorConcurrent,
    )
    
    // 使用适配器让并发服务实现原有Service接口
    c.ValidationService = validation.NewServiceAdapter(concurrentService)
    
    return nil
}
```

### 3. 配置驱动

```go
type ValidationConfig struct {
    Strategy       string `json:"strategy"`        // "sequential" or "concurrent"
    MaxConcurrency int    `json:"max_concurrency"` // 最大并发数
}

func CreateValidationService(config *ValidationConfig, questionnaireService questionnaire.Service) validation.Service {
    factory := validation.NewServiceFactory(questionnaireService)
    
    switch config.Strategy {
    case "concurrent":
        return factory.CreateConcurrentServiceWithAdapter(config.MaxConcurrency)
    case "sequential":
        return factory.CreateSequentialService()
    default:
        return factory.CreateConcurrentServiceWithAdapter(10) // 默认并发
    }
}
```

## 验证流程

### 1. 串行验证流程
1. 验证问卷代码
2. 获取问卷信息
3. 逐个验证答案
4. 返回验证结果

### 2. 并发验证流程  
1. 验证问卷代码
2. 获取问卷信息
3. 创建问题映射
4. 使用信号量控制并发数
5. 并发验证所有答案
6. 等待所有验证完成
7. 收集和返回验证结果

## 性能特点

### 串行验证
- **内存使用**: 低
- **CPU使用**: 低
- **适用场景**: 答案数量少、服务器资源有限

### 并发验证
- **内存使用**: 中等（可配置）
- **CPU使用**: 高（可配置）
- **适用场景**: 答案数量多、需要快速响应
- **并发控制**: 使用信号量限制最大并发数

## 错误处理

### 1. 验证错误分类
- **请求格式错误**: 请求参数不完整或格式错误
- **问卷代码错误**: 问卷不存在或无效
- **答案验证错误**: 答案不符合问题的验证规则
- **系统错误**: 网络连接、数据库访问等系统级错误

### 2. 错误信息结构
```go
type ValidationError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Field   string `json:"field,omitempty"`
    Details string `json:"details,omitempty"`
}
```

## 扩展点

### 1. 自定义验证策略
可以通过实现 `Service` 接口来创建自定义验证策略。

### 2. 自定义验证规则
可以通过扩展 domain/validation 层来添加新的验证规则。

### 3. 性能监控
可以添加性能监控中间件来跟踪验证服务的性能指标。

## 测试

### 单元测试
```bash
go test ./internal/collection-server/application/validation/...
```

### 基准测试
```bash
go test -bench=. ./internal/collection-server/application/validation/...
```

## 配置示例

### collection-server.yaml
```yaml
concurrency:
  max_concurrency: 10  # 最大并发验证数

validation:
  strategy: "concurrent"  # 验证策略: sequential | concurrent
  timeout: 30            # 验证超时时间（秒）
```

## 日志

验证服务会输出详细的日志信息，包括：
- 验证开始和结束时间
- 使用的验证策略和参数
- 验证结果统计
- 错误详细信息

## 监控指标

建议监控以下指标：
- 验证请求总数
- 验证成功率
- 平均验证时间
- 并发验证队列长度
- 验证错误分布

## 性能调优

### 并发数调优
- 根据服务器CPU核心数设置合适的并发数
- 监控内存使用情况，避免过度并发
- 考虑下游服务的承载能力

### 缓存优化
- 缓存问卷信息以减少网络调用
- 缓存验证规则以提高验证速度

## 注意事项

1. **并发安全**: 所有验证器都是线程安全的
2. **资源管理**: 并发验证会消耗更多CPU和内存资源
3. **错误处理**: 任何一个答案验证失败都会导致整个请求失败
4. **超时控制**: 需要设置合适的超时时间防止长时间阻塞 