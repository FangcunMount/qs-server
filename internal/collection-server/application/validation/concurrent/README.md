# Concurrent Validation 并发验证包

## 概述

本包实现了collection-server应用层的并发验证功能，通过信号量控制并发数，实现高效的答卷验证处理。

## 架构设计

### 核心组件

#### 1. Service (服务层)
- **Service**: 并发验证服务接口
- **service**: 并发验证服务实现
- 负责整体验证流程和业务逻辑

#### 2. Validator (验证器层)
- **Validator**: 纯并发验证器
- 专注于并发验证算法实现
- 使用信号量控制并发数

#### 3. Adapter (适配器层)
- **ValidatorAdapter**: 基础验证器适配器
- **ExtendedServiceAdapter**: 扩展服务适配器
- **ConcurrentValidationManager**: 并发验证管理器

## 并发验证算法

### 核心流程

```go
func (v *Validator) ValidateAnswersConcurrently(ctx context.Context, answers []*answersheet.Answer, questionnaireInfo answersheet.QuestionnaireInfo) error {
    // 1. 创建问题映射表
    questionMap := make(map[string]answersheet.QuestionInfo)
    
    // 2. 创建错误收集通道
    errorChan := make(chan error, len(answers))
    
    // 3. 创建信号量控制并发数
    semaphore := make(chan struct{}, v.maxConcurrency)
    
    // 4. 并发验证每个答案
    var wg sync.WaitGroup
    for _, answer := range answers {
        wg.Add(1)
        go func(answer *answersheet.Answer) {
            defer wg.Done()
            
            // 获取信号量
            semaphore <- struct{}{}
            defer func() { <-semaphore }()
            
            // 执行验证逻辑
        }(answer)
    }
    
    // 5. 等待所有验证完成
    wg.Wait()
    close(errorChan)
    
    // 6. 收集并返回错误
    return handleErrors(errorChan)
}
```

### 性能特点

- **并发控制**: 使用信号量限制最大并发数
- **内存效率**: 有界的错误通道避免内存泄漏
- **快速失败**: 任一验证失败立即收集错误
- **线程安全**: 所有并发操作都是线程安全的

## 使用方式

### 1. 基本使用

```go
import "github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation/concurrent"

// 创建并发验证服务
service := concurrent.NewService(questionnaireService, 10) // 最大并发数10

// 构建验证请求
req := &concurrent.ValidationRequest{
    QuestionnaireCode: "sample-questionnaire",
    Title: "样例问卷",
    TesteeInfo: &concurrent.TesteeInfo{
        Name: "张三",
    },
    Answers: []*concurrent.AnswerValidationItem{
        {
            QuestionCode: "q1",
            QuestionType: "text", 
            Value: "答案内容",
        },
    },
}

// 执行并发验证
err := service.ValidateAnswersheet(ctx, req)
```

### 2. 使用独立验证器

```go
// 创建纯并发验证器
validator := concurrent.NewValidator(10)

// 直接验证答案数组
err := validator.ValidateAnswersConcurrently(ctx, answers, questionnaireInfo)

// 验证完整提交请求
err := validator.ValidateAnswersWithValidation(ctx, submitRequest, questionnaireInfo)
```

### 3. 使用管理器模式

```go
// 创建并发验证管理器
manager := concurrent.NewConcurrentValidationManager(questionnaireService, 10)

// 获取验证器和服务
validator := manager.GetValidator()
service := manager.GetService()

// 使用双重验证
err := manager.ValidateWithBoth(ctx, req)

// 获取管理器信息
info := manager.GetManagerInfo()
```

### 4. 使用扩展服务适配器

```go
// 创建扩展服务适配器
adapter := concurrent.NewExtendedServiceAdapter(questionnaireService, 10)

// 执行验证
err := adapter.ValidateAnswersheet(ctx, req)

// 动态调整并发数
adapter.SetMaxConcurrency(20)
maxConcurrency := adapter.GetMaxConcurrency()

// 获取服务信息
info := adapter.GetServiceInfo()
```

## 配置参数

### 并发数设置

```go
type ConcurrencyConfig struct {
    MaxConcurrency int `json:"max_concurrency"` // 建议设置为 CPU核心数 * 2
}

// 推荐配置
// 单核: 2-4
// 双核: 4-8  
// 四核: 8-16
// 八核: 16-32
```

### 性能监控

```go
// 获取并发统计信息
stats := validator.GetConcurrencyStats()
// 输出: {
//   "max_concurrency": 10,
//   "validator_type": "concurrent", 
//   "domain_validator": "answersheet"
// }

// 获取服务信息
info := service.GetServiceInfo()
// 输出: {
//   "service_type": "concurrent",
//   "max_concurrency": 10,
//   "domain_layer": "answersheet"
// }
```

## 错误处理

### 错误收集策略

```go
// 1. 快速失败 - 任一验证失败立即返回第一个错误
// 2. 错误聚合 - 收集所有错误并返回最重要的错误
// 3. 上下文感知 - 包含详细的错误上下文信息
```

### 常见错误类型

- **问题映射错误**: 答案对应的问题不存在
- **验证规则错误**: 答案不符合问题的验证规则
- **并发控制错误**: 超过最大并发数限制
- **资源竞争错误**: 多个goroutine访问共享资源

## 性能优化

### 1. 并发数调优

```go
// 根据实际负载调整
func OptimalConcurrency(cpuCores int, answerCount int) int {
    // 基于CPU核心数
    baseConcurrency := cpuCores * 2
    
    // 基于答案数量
    if answerCount < 10 {
        return min(baseConcurrency, answerCount)
    }
    
    // 最大不超过50个并发
    return min(baseConcurrency, 50)
}
```

### 2. 内存优化

```go
// 使用有界通道避免内存爆炸
errorChan := make(chan error, len(answers)) // 容量等于答案数

// 及时释放资源
defer close(errorChan)
```

### 3. CPU优化  

```go
// 合理分配goroutine
runtime.GOMAXPROCS(runtime.NumCPU())

// 避免goroutine泄露
defer wg.Done()
```

## 测试

### 单元测试

```bash
go test ./internal/collection-server/application/validation/concurrent/...
```

### 压力测试

```go
func BenchmarkConcurrentValidation(b *testing.B) {
    validator := NewValidator(10)
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            validator.ValidateAnswersConcurrently(ctx, answers, questionnaire)
        }
    })
}
```

### 并发安全测试

```go
func TestConcurrentSafety(t *testing.T) {
    validator := NewValidator(100)
    
    var wg sync.WaitGroup
    for i := 0; i < 1000; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            validator.ValidateAnswersConcurrently(ctx, answers, questionnaire)
        }()
    }
    wg.Wait()
}
```

## 监控指标

### 关键指标

- **验证吞吐量**: 每秒处理的答案数
- **平均延迟**: 单次验证的平均时间
- **并发利用率**: 实际并发数/最大并发数
- **错误率**: 验证失败的比例

### 日志输出

```
INFO  Starting concurrent validation of 50 answers (max concurrency: 10)
INFO  Concurrent validation completed successfully
ERROR Concurrent validation failed with 3 errors, returning first error: ...
```

## 故障排除

### 常见问题

1. **并发数过高导致资源耗尽**
   - 解决: 降低maxConcurrency参数
   
2. **验证性能不如预期**
   - 解决: 检查CPU使用率和网络延迟
   
3. **内存使用过高**
   - 解决: 检查错误通道是否正确关闭

4. **goroutine泄露**
   - 解决: 确保所有defer语句执行

## 最佳实践

1. **合理设置并发数**: 通常为CPU核心数的2-4倍
2. **监控资源使用**: 关注CPU和内存使用情况
3. **错误处理**: 提供详细的错误上下文
4. **日志记录**: 记录关键操作和性能指标
5. **测试覆盖**: 包括并发安全性测试 