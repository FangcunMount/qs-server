package medicalscale

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/factor"
	v1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
)

// MedicalScale 医学量表聚合根
type MedicalScale struct {
	id                v1.ID
	code              string
	questionnaireCode string
	title             string
	description       string
	factors           []factor.Factor
}

// NewMedicalScale 创建医学量表
func NewMedicalScale(code string, title string, opts ...MedicalScaleOption) *MedicalScale {
	m := &MedicalScale{
		code:  code,
		title: title,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// MedicalScaleOption 医学量表选项
type MedicalScaleOption func(*MedicalScale)

// WithID 设置ID
func WithID(id v1.ID) MedicalScaleOption {
	return func(s *MedicalScale) {
		s.id = id
	}
}

// WithCode 设置代码
func WithCode(code string) MedicalScaleOption {
	return func(s *MedicalScale) {
		s.code = code
	}
}

// WithQuestionnaireCode 设置问卷代码
func WithQuestionnaireCode(questionnaireCode string) MedicalScaleOption {
	return func(s *MedicalScale) {
		s.questionnaireCode = questionnaireCode
	}
}

// WithTitle 设置标题
func WithTitle(title string) MedicalScaleOption {
	return func(s *MedicalScale) {
		s.title = title
	}
}

// WithDescription 设置描述
func WithDescription(description string) MedicalScaleOption {
	return func(s *MedicalScale) {
		s.description = description
	}
}

// WithFactors 设置因子
func WithFactors(factors []factor.Factor) MedicalScaleOption {
	return func(s *MedicalScale) {
		s.factors = factors
	}
}

// SetID 设置ID
func (s *MedicalScale) SetID(id v1.ID) {
	s.id = id
}

// GetID 获取ID
func (s *MedicalScale) GetID() v1.ID {
	return s.id
}

// GetCode 获取代码
func (s *MedicalScale) GetCode() string {
	return s.code
}

// GetQuestionnaireCode 获取问卷代码
func (s *MedicalScale) GetQuestionnaireCode() string {
	return s.questionnaireCode
}

// GetTitle 获取标题
func (s *MedicalScale) GetTitle() string {
	return s.title
}

// GetDescription 获取描述
func (s *MedicalScale) GetDescription() string {
	return s.description
}

// Factors 获取因子列表
func (s *MedicalScale) GetFactors() []factor.Factor {
	return s.factors
}

// SetFactors 设置因子列表
func (s *MedicalScale) SetFactors(factors []factor.Factor) {
	s.factors = factors
}
