package storage

import (
	"context"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

// QuestionnaireDocumentRepository 问卷文档仓储端口
// 专门负责问卷的文档结构存储（问题列表、设置等复杂数据）
type QuestionnaireDocumentRepository interface {
	// 基本文档操作
	SaveDocument(ctx context.Context, q *questionnaire.Questionnaire) error
	GetDocument(ctx context.Context, id questionnaire.QuestionnaireID) (*QuestionnaireDocumentResult, error)
	UpdateDocument(ctx context.Context, q *questionnaire.Questionnaire) error
	RemoveDocument(ctx context.Context, id questionnaire.QuestionnaireID) error

	// 批量操作
	FindDocumentsByQuestionnaireIDs(ctx context.Context, ids []questionnaire.QuestionnaireID) (map[string]*QuestionnaireDocumentResult, error)

	// 搜索功能
	SearchDocuments(ctx context.Context, query DocumentSearchQuery) ([]*QuestionnaireDocumentResult, error)
}

// QuestionnaireDocumentResult 问卷文档查询结果
type QuestionnaireDocumentResult struct {
	ID        string
	Questions []QuestionResult
	Settings  SettingsResult
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// QuestionResult 问题查询结果
type QuestionResult struct {
	ID       string
	Type     string
	Title    string
	Required bool
	Options  []OptionResult
	Settings map[string]interface{}
	Order    int
}

// OptionResult 选项查询结果
type OptionResult struct {
	ID    string
	Text  string
	Value string
	Order int
}

// SettingsResult 设置查询结果
type SettingsResult struct {
	AllowAnonymous bool
	ShowProgress   bool
	RandomOrder    bool
	TimeLimit      *time.Duration
}

// DocumentSearchQuery 文档搜索查询
type DocumentSearchQuery struct {
	Keyword string
	Limit   int
	Skip    int
}
