package dto

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

// QuestionnaireDTO 问卷数据传输对象
type QuestionnaireDTO struct {
	ID          string               `json:"id"`
	Code        string               `json:"code"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Status      questionnaire.Status `json:"status"`
	Questions   []QuestionDTO        `json:"questions"`
	Settings    SettingsDTO          `json:"settings"`
	CreatedBy   string               `json:"created_by"`
	Version     int                  `json:"version"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// QuestionDTO 问题数据传输对象
type QuestionDTO struct {
	ID       string                     `json:"id"`
	Type     questionnaire.QuestionType `json:"type"`
	Title    string                     `json:"title"`
	Required bool                       `json:"required"`
	Options  []OptionDTO                `json:"options"`
	Settings map[string]interface{}     `json:"settings"`
}

// OptionDTO 选项数据传输对象
type OptionDTO struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Value string `json:"value"`
}

// SettingsDTO 设置数据传输对象
type SettingsDTO struct {
	AllowAnonymous bool           `json:"allow_anonymous"`
	ShowProgress   bool           `json:"show_progress"`
	RandomOrder    bool           `json:"random_order"`
	TimeLimit      *time.Duration `json:"time_limit,omitempty"`
}

// QuestionnaireListDTO 问卷列表数据传输对象
type QuestionnaireListDTO struct {
	Items      []QuestionnaireDTO             `json:"items"`
	Pagination *interfaces.PaginationResponse `json:"pagination"`
}

// QuestionnaireStatisticsDTO 问卷统计数据传输对象
type QuestionnaireStatisticsDTO struct {
	TotalCount       int64                          `json:"total_count"`
	StatusCounts     map[questionnaire.Status]int64 `json:"status_counts"`
	CreatedToday     int64                          `json:"created_today"`
	CreatedThisWeek  int64                          `json:"created_this_week"`
	CreatedThisMonth int64                          `json:"created_this_month"`
	PopularTags      []TagStatDTO                   `json:"popular_tags"`
}

// TagStatDTO 标签统计数据传输对象
type TagStatDTO struct {
	Tag   string `json:"tag"`
	Count int64  `json:"count"`
}

// FromDomain 从领域对象转换为DTO
func (dto *QuestionnaireDTO) FromDomain(q *questionnaire.Questionnaire) {
	dto.ID = q.ID().Value()
	dto.Code = q.Code()
	dto.Title = q.Title()
	dto.Description = q.Description()
	dto.Status = q.Status()
	dto.CreatedBy = q.CreatedBy()
	dto.Version = q.Version()
	dto.CreatedAt = q.CreatedAt()
	dto.UpdatedAt = q.UpdatedAt()

	// 转换问题
	dto.Questions = make([]QuestionDTO, len(q.Questions()))
	for i, domainQ := range q.Questions() {
		questionDTO := QuestionDTO{
			ID:       domainQ.ID(),
			Type:     domainQ.Type(),
			Title:    domainQ.Title(),
			Required: domainQ.Required(),
			Settings: domainQ.Settings(),
		}

		// 转换选项
		questionDTO.Options = make([]OptionDTO, len(domainQ.Options()))
		for j, opt := range domainQ.Options() {
			questionDTO.Options[j] = OptionDTO{
				ID:    opt.ID(),
				Text:  opt.Text(),
				Value: opt.Value(),
			}
		}

		dto.Questions[i] = questionDTO
	}

	// 转换设置
	dto.Settings = SettingsDTO{
		AllowAnonymous: q.Settings().AllowAnonymous(),
		ShowProgress:   q.Settings().ShowProgress(),
		RandomOrder:    q.Settings().RandomOrder(),
		TimeLimit:      q.Settings().TimeLimit(),
	}
}

// FromDomainList 从领域对象列表转换为DTO列表
func FromDomainList(questionnaires []*questionnaire.Questionnaire) []QuestionnaireDTO {
	dtos := make([]QuestionnaireDTO, len(questionnaires))
	for i, q := range questionnaires {
		dtos[i].FromDomain(q)
	}
	return dtos
}

// QuestionnaireFilterDTO 问卷过滤器DTO
type QuestionnaireFilterDTO struct {
	interfaces.FilterRequest
	interfaces.SortingRequest

	CreatorID *string               `form:"creator_id" json:"creator_id"`
	Status    *questionnaire.Status `form:"status" json:"status"`
	Code      *string               `form:"code" json:"code"`
	DateFrom  *time.Time            `form:"date_from" json:"date_from"`
	DateTo    *time.Time            `form:"date_to" json:"date_to"`
	Tags      []string              `form:"tags" json:"tags"`
}

// SetDefaults 设置默认值
func (f *QuestionnaireFilterDTO) SetDefaults() {
	f.SortingRequest.SetDefaults("updated_at")
}

// HasCreatorFilter 是否有创建者过滤
func (f *QuestionnaireFilterDTO) HasCreatorFilter() bool {
	return f.CreatorID != nil && *f.CreatorID != ""
}

// HasStatusFilter 是否有状态过滤
func (f *QuestionnaireFilterDTO) HasStatusFilter() bool {
	return f.Status != nil
}

// HasCodeFilter 是否有代码过滤
func (f *QuestionnaireFilterDTO) HasCodeFilter() bool {
	return f.Code != nil && *f.Code != ""
}

// HasDateFilter 是否有日期过滤
func (f *QuestionnaireFilterDTO) HasDateFilter() bool {
	return f.DateFrom != nil || f.DateTo != nil
}

// HasTagsFilter 是否有标签过滤
func (f *QuestionnaireFilterDTO) HasTagsFilter() bool {
	return len(f.Tags) > 0
}

// GetCreatorID 获取创建者ID
func (f *QuestionnaireFilterDTO) GetCreatorID() string {
	if f.HasCreatorFilter() {
		return *f.CreatorID
	}
	return ""
}

// GetStatus 获取状态
func (f *QuestionnaireFilterDTO) GetStatus() questionnaire.Status {
	if f.HasStatusFilter() {
		return *f.Status
	}
	return questionnaire.StatusDraft // 返回默认状态
}

// GetCode 获取代码
func (f *QuestionnaireFilterDTO) GetCode() string {
	if f.HasCodeFilter() {
		return *f.Code
	}
	return ""
}
