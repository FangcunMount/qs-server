package scale

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// MedicalScale 医学量表聚合根
type MedicalScale struct {
	// 标识
	id        meta.ID
	scaleCode meta.Code

	// 基本信息
	title       string
	description string

	// 关联的问卷（编码 + 版本）
	questionnaireCode    meta.Code
	questionnaireVersion string

	// 状态
	status Status

	// 因子列表（包含解读规则）
	factors []*Factor
}

// ===================== MedicalScale 构造相关 =================

// MedicalScaleOption 医学量表构造选项
type MedicalScaleOption func(*MedicalScale)

// NewMedicalScale 创建医学量表
func NewMedicalScale(scaleCode meta.Code, title string, opts ...MedicalScaleOption) (*MedicalScale, error) {
	if scaleCode.IsEmpty() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "scale code cannot be empty")
	}
	if title == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "scale title cannot be empty")
	}

	m := &MedicalScale{
		scaleCode: scaleCode,
		title:     title,
		status:    StatusDraft,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m, nil
}

// With*** 构造选项

// WithID 设置ID
func WithID(id meta.ID) MedicalScaleOption {
	return func(m *MedicalScale) {
		m.id = id
	}
}

// WithDescription 设置描述
func WithDescription(desc string) MedicalScaleOption {
	return func(m *MedicalScale) {
		m.description = desc
	}
}

// WithQuestionnaire 设置关联的问卷（编码 + 版本）
func WithQuestionnaire(qCode meta.Code, qVersion string) MedicalScaleOption {
	return func(m *MedicalScale) {
		m.questionnaireCode = qCode
		m.questionnaireVersion = qVersion
	}
}

// WithStatus 设置状态
func WithStatus(s Status) MedicalScaleOption {
	return func(m *MedicalScale) {
		m.status = s
	}
}

// WithFactors 设置因子列表
func WithFactors(factors []*Factor) MedicalScaleOption {
	return func(m *MedicalScale) {
		m.factors = factors
	}
}

// ===================== Getter 方法 =================

// GetID 获取ID
func (m *MedicalScale) GetID() meta.ID {
	return m.id
}

// GetCode 获取编码
func (m *MedicalScale) GetCode() meta.Code {
	return m.scaleCode
}

// GetTitle 获取标题
func (m *MedicalScale) GetTitle() string {
	return m.title
}

// GetDescription 获取描述
func (m *MedicalScale) GetDescription() string {
	return m.description
}

// GetQuestionnaireCode 获取关联的问卷编码
func (m *MedicalScale) GetQuestionnaireCode() meta.Code {
	return m.questionnaireCode
}

// GetQuestionnaireVersion 获取关联的问卷版本
func (m *MedicalScale) GetQuestionnaireVersion() string {
	return m.questionnaireVersion
}

// GetStatus 获取状态
func (m *MedicalScale) GetStatus() Status {
	return m.status
}

// GetFactors 获取因子列表
func (m *MedicalScale) GetFactors() []*Factor {
	return m.factors
}

// ===================== 状态判断方法 =================

// IsDraft 是否草稿状态
func (m *MedicalScale) IsDraft() bool {
	return m.status.IsDraft()
}

// IsPublished 是否已发布状态
func (m *MedicalScale) IsPublished() bool {
	return m.status.IsPublished()
}

// IsArchived 是否已归档状态
func (m *MedicalScale) IsArchived() bool {
	return m.status.IsArchived()
}

// ===================== 业务查询方法 =================

// FactorCount 获取因子数量
func (m *MedicalScale) FactorCount() int {
	return len(m.factors)
}

// FindFactorByCode 根据因子编码查找因子
func (m *MedicalScale) FindFactorByCode(factorCode FactorCode) (*Factor, bool) {
	for _, f := range m.factors {
		if f.GetCode().Equals(factorCode) {
			return f, true
		}
	}
	return nil, false
}

// GetTotalScoreFactor 获取总分因子
func (m *MedicalScale) GetTotalScoreFactor() (*Factor, bool) {
	for _, f := range m.factors {
		if f.IsTotalScore() {
			return f, true
		}
	}
	return nil, false
}

// GetNonTotalScoreFactors 获取非总分因子列表
func (m *MedicalScale) GetNonTotalScoreFactors() []*Factor {
	var result []*Factor
	for _, f := range m.factors {
		if !f.IsTotalScore() {
			result = append(result, f)
		}
	}
	return result
}

// ===================== 包内私有方法（供领域服务调用）=================

// setID 设置ID（仅供仓储层使用）
func (m *MedicalScale) setID(id meta.ID) {
	m.id = id
}

// updateBasicInfo 更新基本信息
func (m *MedicalScale) updateBasicInfo(title, description string) error {
	if title == "" {
		return errors.WithCode(code.ErrInvalidArgument, "title cannot be empty")
	}
	m.title = title
	m.description = description
	return nil
}

// updateStatus 更新状态
func (m *MedicalScale) updateStatus(newStatus Status) error {
	if m.status.IsArchived() && !newStatus.IsArchived() {
		return errors.WithCode(code.ErrInvalidArgument, "archived scale cannot change status")
	}
	m.status = newStatus
	return nil
}

// updateQuestionnaire 更新关联的问卷
func (m *MedicalScale) updateQuestionnaire(qCode meta.Code, qVersion string) error {
	if qCode.IsEmpty() {
		return errors.WithCode(code.ErrInvalidArgument, "questionnaire code cannot be empty")
	}
	if qVersion == "" {
		return errors.WithCode(code.ErrInvalidArgument, "questionnaire version cannot be empty")
	}
	m.questionnaireCode = qCode
	m.questionnaireVersion = qVersion
	return nil
}

// addFactor 添加因子
func (m *MedicalScale) addFactor(f *Factor) error {
	// 幂等性检查
	for _, existingFactor := range m.factors {
		if existingFactor.GetCode().Equals(f.GetCode()) {
			return errors.WithCode(code.ErrInvalidArgument, "factor code already exists")
		}
	}
	m.factors = append(m.factors, f)
	return nil
}

// removeFactor 移除因子
func (m *MedicalScale) removeFactor(factorCode FactorCode) error {
	for i, f := range m.factors {
		if f.GetCode().Equals(factorCode) {
			m.factors = append(m.factors[:i], m.factors[i+1:]...)
			return nil
		}
	}
	return errors.WithCode(code.ErrInvalidArgument, "factor not found")
}

// updateFactors 更新因子列表
func (m *MedicalScale) updateFactors(factors []*Factor) {
	m.factors = factors
}
