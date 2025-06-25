package questionnaire

import (
	questionnaireDomain "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

// Converter 问卷转换器
type Converter struct{}

// NewConverter 创建问卷转换器
func NewConverter() *Converter {
	return &Converter{}
}

// DomainToModel 将领域对象转换为数据模型
func (c *Converter) DomainToModel(q *questionnaireDomain.Questionnaire) *Model {
	if q == nil {
		return nil
	}

	return &Model{
		ID:          q.ID().Value(),
		Code:        q.Code(),
		Title:       q.Title(),
		Description: q.Description(),
		Status:      int(q.Status()),
		CreatedBy:   q.CreatedBy(),
		CreatedAt:   q.CreatedAt(),
		UpdatedAt:   q.UpdatedAt(),
		Version:     q.Version(),
	}
}

// ModelToDomain 将数据模型转换为领域对象
func (c *Converter) ModelToDomain(model *Model) *questionnaireDomain.Questionnaire {
	if model == nil || model.ID == "" {
		return nil
	}

	// TODO: 这里需要完善，应该通过工厂方法或构造函数创建完整的Questionnaire对象
	// 目前使用简单的工厂方法
	return questionnaireDomain.NewQuestionnaire(model.Code, model.Title, model.Description, model.CreatedBy)
}
