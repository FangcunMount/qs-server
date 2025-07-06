package medicalscale

import (
	"fmt"
	"time"

	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// MedicalScale 医学量表聚合根
type MedicalScale struct {
	id                   MedicalScaleID
	code                 string
	title                string
	questionnaireCode    string
	questionnaireVersion string
	factors              []Factor
	createdAt            time.Time
	updatedAt            time.Time
}

// MedicalScaleID 医学量表ID值对象
type MedicalScaleID struct {
	value uint64
}

// NewMedicalScaleID 创建医学量表ID
func NewMedicalScaleID(value uint64) MedicalScaleID {
	return MedicalScaleID{value: value}
}

// Value 获取ID值
func (id MedicalScaleID) Value() uint64 {
	return id.value
}

// String 字符串表示
func (id MedicalScaleID) String() string {
	return fmt.Sprintf("%d", id.value)
}

// NewMedicalScale 创建新的医学量表
func NewMedicalScale(
	id MedicalScaleID,
	code, title, questionnaireCode, questionnaireVersion string,
	factors []Factor,
) *MedicalScale {
	now := time.Now()
	return &MedicalScale{
		id:                   id,
		code:                 code,
		title:                title,
		questionnaireCode:    questionnaireCode,
		questionnaireVersion: questionnaireVersion,
		factors:              factors,
		createdAt:            now,
		updatedAt:            now,
	}
}

// ID 获取医学量表ID
func (ms *MedicalScale) ID() MedicalScaleID {
	return ms.id
}

// Code 获取量表代码
func (ms *MedicalScale) Code() string {
	return ms.code
}

// Title 获取量表标题
func (ms *MedicalScale) Title() string {
	return ms.title
}

// QuestionnaireCode 获取关联的问卷代码
func (ms *MedicalScale) QuestionnaireCode() string {
	return ms.questionnaireCode
}

// QuestionnaireVersion 获取关联的问卷版本
func (ms *MedicalScale) QuestionnaireVersion() string {
	return ms.questionnaireVersion
}

// Factors 获取因子列表
func (ms *MedicalScale) Factors() []Factor {
	// 返回副本以保护内部状态
	result := make([]Factor, len(ms.factors))
	copy(result, ms.factors)
	return result
}

// CreatedAt 获取创建时间
func (ms *MedicalScale) CreatedAt() time.Time {
	return ms.createdAt
}

// UpdatedAt 获取更新时间
func (ms *MedicalScale) UpdatedAt() time.Time {
	return ms.updatedAt
}

// UpdateTitle 更新量表标题
func (ms *MedicalScale) UpdateTitle(title string) error {
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	ms.title = title
	ms.updatedAt = time.Now()
	log.Infof("Medical scale %s title updated to: %s", ms.code, title)
	return nil
}

// UpdateQuestionnaireBinding 更新问卷绑定
func (ms *MedicalScale) UpdateQuestionnaireBinding(questionnaireCode, questionnaireVersion string) error {
	if questionnaireCode == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}
	if questionnaireVersion == "" {
		return fmt.Errorf("questionnaire version cannot be empty")
	}

	ms.questionnaireCode = questionnaireCode
	ms.questionnaireVersion = questionnaireVersion
	ms.updatedAt = time.Now()

	log.Infof("Medical scale %s questionnaire binding updated to: %s@%s",
		ms.code, questionnaireCode, questionnaireVersion)
	return nil
}

// AddFactor 添加因子
func (ms *MedicalScale) AddFactor(factor Factor) error {
	// 检查因子代码是否重复
	for _, existingFactor := range ms.factors {
		if existingFactor.Code() == factor.Code() {
			return fmt.Errorf("factor with code %s already exists", factor.Code())
		}
	}

	ms.factors = append(ms.factors, factor)
	ms.updatedAt = time.Now()

	log.Infof("Factor %s added to medical scale %s", factor.Code(), ms.code)
	return nil
}

// UpdateFactor 更新因子
func (ms *MedicalScale) UpdateFactor(factorCode string, updatedFactor Factor) error {
	for i, factor := range ms.factors {
		if factor.Code() == factorCode {
			ms.factors[i] = updatedFactor
			ms.updatedAt = time.Now()
			log.Infof("Factor %s updated in medical scale %s", factorCode, ms.code)
			return nil
		}
	}
	return fmt.Errorf("factor with code %s not found", factorCode)
}

// RemoveFactor 移除因子
func (ms *MedicalScale) RemoveFactor(factorCode string) error {
	for i, factor := range ms.factors {
		if factor.Code() == factorCode {
			ms.factors = append(ms.factors[:i], ms.factors[i+1:]...)
			ms.updatedAt = time.Now()
			log.Infof("Factor %s removed from medical scale %s", factorCode, ms.code)
			return nil
		}
	}
	return fmt.Errorf("factor with code %s not found", factorCode)
}

// GetFactor 获取指定因子
func (ms *MedicalScale) GetFactor(factorCode string) (Factor, error) {
	for _, factor := range ms.factors {
		if factor.Code() == factorCode {
			return factor, nil
		}
	}
	return Factor{}, fmt.Errorf("factor with code %s not found", factorCode)
}

// GetTotalScoreFactors 获取总分因子列表
func (ms *MedicalScale) GetTotalScoreFactors() []Factor {
	var totalFactors []Factor
	for _, factor := range ms.factors {
		if factor.IsTotalScore() {
			totalFactors = append(totalFactors, factor)
		}
	}
	return totalFactors
}

// GetFactorsByType 根据类型获取因子列表
func (ms *MedicalScale) GetFactorsByType(factorType FactorType) []Factor {
	var factors []Factor
	for _, factor := range ms.factors {
		if factor.Type() == factorType {
			factors = append(factors, factor)
		}
	}
	return factors
}

// Validate 验证医学量表的完整性
func (ms *MedicalScale) Validate() error {
	if ms.code == "" {
		return fmt.Errorf("medical scale code cannot be empty")
	}
	if ms.title == "" {
		return fmt.Errorf("medical scale title cannot be empty")
	}
	if ms.questionnaireCode == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}
	if ms.questionnaireVersion == "" {
		return fmt.Errorf("questionnaire version cannot be empty")
	}
	if len(ms.factors) == 0 {
		return fmt.Errorf("medical scale must have at least one factor")
	}

	// 验证每个因子
	for i, factor := range ms.factors {
		if err := factor.Validate(); err != nil {
			return fmt.Errorf("factor %d validation failed: %w", i, err)
		}
	}

	// 检查是否有总分因子
	hasTotalScore := false
	for _, factor := range ms.factors {
		if factor.IsTotalScore() {
			hasTotalScore = true
			break
		}
	}
	if !hasTotalScore {
		log.Warnf("Medical scale %s has no total score factor", ms.code)
	}

	return nil
}
