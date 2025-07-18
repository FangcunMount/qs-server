# 内部消息包 (internal/pkg/pubsub)

这个包定义了系统内各个服务之间通信的消息类型和常量。

## 目录结构

```
internal/pkg/pubsub/
├── messages.go  # 消息类型定义
└── README.md    # 说明文档
```

## 设计原则

1. **跨服务共享**：定义在 `internal/pkg` 下，可以被项目内所有服务导入
2. **类型安全**：使用强类型定义，避免运行时错误
3. **扩展性**：易于添加新的消息类型
4. **版本兼容**：消息格式设计考虑向前兼容

## 消息类型

### 1. 答卷已保存消息 (AnswersheetSavedMessage)

当用户提交答卷后，collection-server 会发布此消息。

```go
type AnswersheetSavedData struct {
    ResponseID      string `json:"response_id"`
    QuestionnaireID string `json:"questionnaire_id"`
    UserID          string `json:"user_id"`
    SubmittedAt     int64  `json:"submitted_at"`
}
```

**发布者**: collection-server  
**订阅者**: evaluation-server  
**主题**: "answersheet.saved"

### 2. 评估完成消息 (EvaluationCompletedMessage)

当评估服务完成量表计算后，evaluation-server 会发布此消息。

```go
type EvaluationCompletedData struct {
    ResponseID   string             `json:"response_id"`
    UserID       string             `json:"user_id"`
    ScaleID      string             `json:"scale_id"`
    TotalScore   float64            `json:"total_score"`
    FactorScores map[string]float64 `json:"factor_scores"`
    CompletedAt  int64              `json:"completed_at"`
}
```

**发布者**: evaluation-server  
**订阅者**: api-server  
**主题**: "evaluation.completed"

### 3. 报告生成消息 (ReportGeneratedMessage)

当解读报告生成完成后，evaluation-server 会发布此消息。

```go
type ReportGeneratedData struct {
    ResponseID string `json:"response_id"`
    UserID     string `json:"user_id"`
    ReportID   string `json:"report_id"`
    ReportURL  string `json:"report_url"`
    CreatedAt  int64  `json:"created_at"`
}
```

**发布者**: evaluation-server  
**订阅者**: api-server, collection-server  
**主题**: "report.generated"

## 消息常量

### 消息类型常量

```go
const (
    MessageTypeAnswersheetSaved     = "answersheet.saved"
    MessageTypeAnswersheetSubmitted = "answersheet.submitted"
    MessageTypeEvaluationCompleted  = "evaluation.completed"
    MessageTypeReportGenerated      = "report.generated"
)
```

### 消息来源常量

```go
const (
    SourceCollectionServer = "collection-server"
    SourceAPIServer        = "api-server"
    SourceEvaluationServer = "evaluation-server"
)
```

## 使用方式

### 发布消息

```go
import internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"

// 创建消息数据
data := &internalpubsub.AnswersheetSavedData{
    ResponseID:      "12345",
    QuestionnaireID: "questionnaire-001",
    UserID:          "user-001",
    SubmittedAt:     time.Now().Unix(),
}

// 创建消息
message := internalpubsub.NewAnswersheetSavedMessage(
    internalpubsub.SourceCollectionServer,
    data,
)

// 发布消息
err := publisher.Publish(ctx, "answersheet.saved", message)
```

### 订阅消息

```go
import internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"

// 创建消息工厂
factory := internalpubsub.NewMessageFactory()

// 消息处理器
handler := func(topic string, data []byte) error {
    // 解析消息
    msg, err := factory.ParseMessage(data)
    if err != nil {
        return err
    }
    
    // 根据消息类型处理
    switch msg.GetType() {
    case internalpubsub.MessageTypeAnswersheetSaved:
        // 提取答卷数据
        answersheetData, err := internalpubsub.GetAnswersheetSavedData(msg)
        if err != nil {
            return err
        }
        
        // 处理答卷已保存事件
        return processAnswersheetSaved(answersheetData)
    }
    
    return nil
}

// 订阅消息
err := subscriber.Subscribe(ctx, "answersheet.saved", handler)
```

## 消息工厂

`MessageFactory` 提供了统一的消息解析接口：

```go
factory := internalpubsub.NewMessageFactory()

// 解析消息
msg, err := factory.ParseMessage(data)

// 根据类型创建具体消息
specificMsg, err := factory.CreateMessage(msgType, data)
```

## 最佳实践

1. **使用常量**：始终使用定义的常量，避免硬编码字符串
2. **类型检查**：在处理消息前检查消息类型
3. **错误处理**：妥善处理消息解析和处理错误
4. **向前兼容**：添加新字段时使用可选字段，保持向前兼容
5. **文档更新**：添加新消息类型时及时更新文档

## 扩展指南

### 添加新的消息类型

1. 在 `messages.go` 中添加新的消息类型常量
2. 定义消息数据结构
3. 定义消息结构体（嵌入 `pubsub.BaseMessage`）
4. 实现构造函数和序列化方法
5. 在 `MessageFactory` 中添加对应的解析逻辑
6. 更新文档

### 示例：添加用户注册消息

```go
// 1. 添加常量
const MessageTypeUserRegistered = "user.registered"

// 2. 定义数据结构
type UserRegisteredData struct {
    UserID      string `json:"user_id"`
    Email       string `json:"email"`
    RegisteredAt int64  `json:"registered_at"`
}

// 3. 定义消息结构
type UserRegisteredMessage struct {
    *pubsub.BaseMessage
    UserData *UserRegisteredData `json:"user_data"`
}

// 4. 实现构造函数
func NewUserRegisteredMessage(source string, data *UserRegisteredData) *UserRegisteredMessage {
    return &UserRegisteredMessage{
        BaseMessage: pubsub.NewBaseMessage(MessageTypeUserRegistered, source, data),
        UserData:    data,
    }
}

// 5. 在 MessageFactory 中添加解析逻辑
func (f *MessageFactory) CreateMessage(msgType string, data []byte) (pubsub.Message, error) {
    switch msgType {
    case MessageTypeUserRegistered:
        return UnmarshalUserRegisteredMessage(data)
    // ... 其他类型
    }
}
``` 