package port

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
)

// QuestionnaireCreator 问卷创建接口
type QuestionnaireCreator interface {
	// CreateQuestionnaire 创建问卷
	CreateQuestionnaire(ctx context.Context, questionnaireDTO *dto.QuestionnaireDTO) (*dto.QuestionnaireDTO, error)
}

// QuestionnaireQueryer 问卷查询接口
type QuestionnaireQueryer interface {
	// GetQuestionnaireByCode 根据问卷代码获取问卷
	GetQuestionnaireByCode(ctx context.Context, code string) (*dto.QuestionnaireDTO, error)
	// ListQuestionnaires 列出问卷列表
	ListQuestionnaires(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*dto.QuestionnaireDTO, int64, error)
}

// QuestionnaireEditor 问卷编辑接口
type QuestionnaireEditor interface {
	// EditBasicInfo 编辑问卷基本信息
	EditBasicInfo(ctx context.Context, questionnaireDTO *dto.QuestionnaireDTO) (*dto.QuestionnaireDTO, error)
	// UpdateQuestions 更新问卷问题
	UpdateQuestions(ctx context.Context, code string, questions []dto.QuestionDTO) (*dto.QuestionnaireDTO, error)
}

// QuestionnairePublisher 问卷发布接口
type QuestionnairePublisher interface {
	// Publish 发布问卷
	Publish(ctx context.Context, code string) (*dto.QuestionnaireDTO, error)
	// Unpublish 取消发布问卷
	Unpublish(ctx context.Context, code string) (*dto.QuestionnaireDTO, error)
}
