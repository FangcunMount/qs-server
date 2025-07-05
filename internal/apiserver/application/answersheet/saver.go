package answersheet

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
)

// Saver 答卷保存器
type Saver struct {
	aRepoMongo port.AnswerSheetRepositoryMongo
}

// NewSaver 创建答卷保存器
func NewSaver(aRepoMongo port.AnswerSheetRepositoryMongo) *Saver {
	return &Saver{aRepoMongo: aRepoMongo}
}

// SaveOriginalAnswerSheet 保存原始答卷
func (s *Saver) SaveOriginalAnswerSheet(ctx context.Context, aDomain *answersheet.AnswerSheet) (*answersheet.AnswerSheet, error) {
	// 1. 保存到 MongoDB
	if err := s.aRepoMongo.Create(ctx, aDomain); err != nil {
		return nil, err
	}

	// 2. 返回答卷领域对象
	return aDomain, nil
}

// SaveAnswerSheetScores 保存答卷得分
func (s *Saver) SaveAnswerSheetScores(ctx context.Context, aDomain *answersheet.AnswerSheet) (*answersheet.AnswerSheet, error) {
	// 1. 更新答卷到 MongoDB（包含计算后的得分）
	if err := s.aRepoMongo.Update(ctx, aDomain); err != nil {
		return nil, err
	}

	// 2. 返回答卷领域对象
	return aDomain, nil
}
