package interpretationreport

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	v1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
)

// InterpretReport 解读报告
type InterpretReport struct {
	id               v1.ID
	answerSheetId    uint64
	medicalScaleCode string
	title            string
	description      string
	testee           user.Testee
	interpretItems   []InterpretItem
	createdAt        time.Time
	updatedAt        time.Time
}

// InterpretReportOption 解读报告选项
type InterpretReportOption func(*InterpretReport)

// NewInterpretReport 创建解读报告
func NewInterpretReport(answerSheetId uint64, medicalScaleCode string, title string, opts ...InterpretReportOption) *InterpretReport {
	report := &InterpretReport{
		answerSheetId:    answerSheetId,
		medicalScaleCode: medicalScaleCode,
		title:            title,
		createdAt:        time.Now(),
		updatedAt:        time.Now(),
	}

	for _, opt := range opts {
		opt(report)
	}

	return report
}

// WithID 设置ID
func WithID(id v1.ID) InterpretReportOption {
	return func(r *InterpretReport) {
		r.id = id
	}
}

// WithDescription 设置描述
func WithDescription(description string) InterpretReportOption {
	return func(r *InterpretReport) {
		r.description = description
	}
}

// WithTestee 设置被试者
func WithTestee(testee user.Testee) InterpretReportOption {
	return func(r *InterpretReport) {
		r.testee = testee
	}
}

// WithInterpretItems 设置解读项列表
func WithInterpretItems(items []InterpretItem) InterpretReportOption {
	return func(r *InterpretReport) {
		r.interpretItems = items
	}
}

// WithCreatedAt 设置创建时间
func WithCreatedAt(createdAt time.Time) InterpretReportOption {
	return func(r *InterpretReport) {
		r.createdAt = createdAt
	}
}

// WithUpdatedAt 设置更新时间
func WithUpdatedAt(updatedAt time.Time) InterpretReportOption {
	return func(r *InterpretReport) {
		r.updatedAt = updatedAt
	}
}

// Getter 方法

// GetID 获取ID
func (r *InterpretReport) GetID() v1.ID {
	return r.id
}

// GetAnswerSheetId 获取答卷ID
func (r *InterpretReport) GetAnswerSheetId() uint64 {
	return r.answerSheetId
}

// GetMedicalScaleCode 获取医学量表代码
func (r *InterpretReport) GetMedicalScaleCode() string {
	return r.medicalScaleCode
}

// GetTitle 获取标题
func (r *InterpretReport) GetTitle() string {
	return r.title
}

// GetDescription 获取描述
func (r *InterpretReport) GetDescription() string {
	return r.description
}

// GetTestee 获取被试者
func (r *InterpretReport) GetTestee() user.Testee {
	return r.testee
}

// GetInterpretItems 获取解读项列表
func (r *InterpretReport) GetInterpretItems() []InterpretItem {
	return r.interpretItems
}

// GetCreatedAt 获取创建时间
func (r *InterpretReport) GetCreatedAt() time.Time {
	return r.createdAt
}

// GetUpdatedAt 获取更新时间
func (r *InterpretReport) GetUpdatedAt() time.Time {
	return r.updatedAt
}

// 业务方法

// SetID 设置ID
func (r *InterpretReport) SetID(id v1.ID) {
	r.id = id
}

// UpdateTitle 更新标题
func (r *InterpretReport) UpdateTitle(title string) {
	r.title = title
	r.updatedAt = time.Now()
}

// UpdateDescription 更新描述
func (r *InterpretReport) UpdateDescription(description string) {
	r.description = description
	r.updatedAt = time.Now()
}

// AddInterpretItem 添加解读项
func (r *InterpretReport) AddInterpretItem(item InterpretItem) {
	r.interpretItems = append(r.interpretItems, item)
	r.updatedAt = time.Now()
}

// RemoveInterpretItem 移除解读项
func (r *InterpretReport) RemoveInterpretItem(factorCode string) {
	for i, item := range r.interpretItems {
		if item.GetFactorCode() == factorCode {
			r.interpretItems = append(r.interpretItems[:i], r.interpretItems[i+1:]...)
			r.updatedAt = time.Now()
			break
		}
	}
}

// UpdateInterpretItem 更新解读项
func (r *InterpretReport) UpdateInterpretItem(factorCode string, updatedItem InterpretItem) {
	for i, item := range r.interpretItems {
		if item.GetFactorCode() == factorCode {
			r.interpretItems[i] = updatedItem
			r.updatedAt = time.Now()
			break
		}
	}
}

// GetInterpretItemByFactorCode 根据因子代码获取解读项
func (r *InterpretReport) GetInterpretItemByFactorCode(factorCode string) *InterpretItem {
	for _, item := range r.interpretItems {
		if item.GetFactorCode() == factorCode {
			return &item
		}
	}
	return nil
}

// GetInterpretItemsCount 获取解读项数量
func (r *InterpretReport) GetInterpretItemsCount() int {
	return len(r.interpretItems)
}

// IsEmpty 判断是否为空报告
func (r *InterpretReport) IsEmpty() bool {
	return len(r.interpretItems) == 0
}

// GetTotalScore 获取总分（所有解读项分数之和）
func (r *InterpretReport) GetTotalScore() int {
	total := 0
	for _, item := range r.interpretItems {
		total += item.GetScore()
	}
	return total
}

// HasFactorCode 判断是否包含指定因子代码的解读项
func (r *InterpretReport) HasFactorCode(factorCode string) bool {
	return r.GetInterpretItemByFactorCode(factorCode) != nil
}

// ClearInterpretItems 清空所有解读项
func (r *InterpretReport) ClearInterpretItems() {
	r.interpretItems = []InterpretItem{}
	r.updatedAt = time.Now()
}

// SetInterpretItems 设置解读项列表
func (r *InterpretReport) SetInterpretItems(items []InterpretItem) {
	r.interpretItems = items
	r.updatedAt = time.Now()
}
