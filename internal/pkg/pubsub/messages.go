package pubsub

import (
	"encoding/json"

	"github.com/yshujie/questionnaire-scale/pkg/pubsub"
)

// 消息类型常量
const (
	MessageTypeAnswersheetSaved     = "answersheet.saved"
	MessageTypeAnswersheetSubmitted = "answersheet.submitted"
	MessageTypeEvaluationCompleted  = "evaluation.completed"
	MessageTypeReportGenerated      = "report.generated"
)

// 消息来源常量
const (
	SourceCollectionServer = "collection-server"
	SourceAPIServer        = "api-server"
	SourceEvaluationServer = "evaluation-server"
)

// AnswersheetSavedData 答卷已保存数据
type AnswersheetSavedData struct {
	ResponseID    string `json:"response_id"`
	AnswerSheetID uint64 `json:"answer_sheet_id"`
	WriterID      uint64 `json:"writer_id"`
	SubmittedAt   int64  `json:"submitted_at"`
}

// AnswersheetSavedMessage 答卷已保存消息
type AnswersheetSavedMessage struct {
	*pubsub.BaseMessage
	AnswersheetData *AnswersheetSavedData `json:"answersheet_data"`
}

// NewAnswersheetSavedMessage 创建答卷已保存消息
func NewAnswersheetSavedMessage(source string, data *AnswersheetSavedData) *AnswersheetSavedMessage {
	return &AnswersheetSavedMessage{
		BaseMessage:     pubsub.NewBaseMessage(MessageTypeAnswersheetSaved, source, data),
		AnswersheetData: data,
	}
}

// Marshal 序列化消息
func (m *AnswersheetSavedMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalAnswersheetSavedMessage 反序列化答卷已保存消息
func UnmarshalAnswersheetSavedMessage(data []byte) (*AnswersheetSavedMessage, error) {
	var msg AnswersheetSavedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EvaluationCompletedData 评估完成数据
type EvaluationCompletedData struct {
	ResponseID   string             `json:"response_id"`
	UserID       string             `json:"user_id"`
	ScaleID      string             `json:"scale_id"`
	TotalScore   float64            `json:"total_score"`
	FactorScores map[string]float64 `json:"factor_scores"`
	CompletedAt  int64              `json:"completed_at"`
}

// EvaluationCompletedMessage 评估完成消息
type EvaluationCompletedMessage struct {
	*pubsub.BaseMessage
	EvaluationData *EvaluationCompletedData `json:"evaluation_data"`
}

// NewEvaluationCompletedMessage 创建评估完成消息
func NewEvaluationCompletedMessage(source string, data *EvaluationCompletedData) *EvaluationCompletedMessage {
	return &EvaluationCompletedMessage{
		BaseMessage:    pubsub.NewBaseMessage(MessageTypeEvaluationCompleted, source, data),
		EvaluationData: data,
	}
}

// Marshal 序列化消息
func (m *EvaluationCompletedMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalEvaluationCompletedMessage 反序列化评估完成消息
func UnmarshalEvaluationCompletedMessage(data []byte) (*EvaluationCompletedMessage, error) {
	var msg EvaluationCompletedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ReportGeneratedData 报告生成数据
type ReportGeneratedData struct {
	ResponseID string `json:"response_id"`
	UserID     string `json:"user_id"`
	ReportID   string `json:"report_id"`
	ReportURL  string `json:"report_url"`
	CreatedAt  int64  `json:"created_at"`
}

// ReportGeneratedMessage 报告生成消息
type ReportGeneratedMessage struct {
	*pubsub.BaseMessage
	ReportData *ReportGeneratedData `json:"report_data"`
}

// NewReportGeneratedMessage 创建报告生成消息
func NewReportGeneratedMessage(source string, data *ReportGeneratedData) *ReportGeneratedMessage {
	return &ReportGeneratedMessage{
		BaseMessage: pubsub.NewBaseMessage(MessageTypeReportGenerated, source, data),
		ReportData:  data,
	}
}

// Marshal 序列化消息
func (m *ReportGeneratedMessage) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// UnmarshalReportGeneratedMessage 反序列化报告生成消息
func UnmarshalReportGeneratedMessage(data []byte) (*ReportGeneratedMessage, error) {
	var msg ReportGeneratedMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// MessageFactory 消息工厂
type MessageFactory struct{}

// NewMessageFactory 创建消息工厂
func NewMessageFactory() *MessageFactory {
	return &MessageFactory{}
}

// CreateMessage 根据类型创建消息
func (f *MessageFactory) CreateMessage(msgType string, data []byte) (pubsub.Message, error) {
	switch msgType {
	case MessageTypeAnswersheetSaved:
		return UnmarshalAnswersheetSavedMessage(data)
	case MessageTypeEvaluationCompleted:
		return UnmarshalEvaluationCompletedMessage(data)
	case MessageTypeReportGenerated:
		return UnmarshalReportGeneratedMessage(data)
	default:
		// 对于未知类型，返回基础消息
		return pubsub.UnmarshalMessage(data)
	}
}

// ParseMessage 解析原始消息数据
func (f *MessageFactory) ParseMessage(data []byte) (pubsub.Message, error) {
	// 先解析基础消息以获取类型
	baseMsg, err := pubsub.UnmarshalMessage(data)
	if err != nil {
		return nil, err
	}

	// 根据类型创建具体消息
	return f.CreateMessage(baseMsg.GetType(), data)
}

// GetAnswersheetSavedData 从基础消息中提取答卷已保存数据
func GetAnswersheetSavedData(msg pubsub.Message) (*AnswersheetSavedData, error) {
	data, ok := msg.GetData().(map[string]interface{})
	if !ok {
		return nil, json.Unmarshal([]byte("{}"), &AnswersheetSavedData{})
	}

	responseID, _ := data["response_id"].(string)
	answerSheetID, _ := data["answer_sheet_id"].(uint64)
	writerID, _ := data["writer_id"].(uint64)
	submittedAt, _ := data["submitted_at"].(float64)

	return &AnswersheetSavedData{
		ResponseID:    responseID,
		AnswerSheetID: answerSheetID,
		WriterID:      writerID,
		SubmittedAt:   int64(submittedAt),
	}, nil
}
