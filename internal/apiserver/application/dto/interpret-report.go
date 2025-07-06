package dto

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
)

// InterpretReportDTO 解读报告DTO
type InterpretReportDTO struct {
	ID               uint64             `json:"id"`
	AnswerSheetId    uint64             `json:"answer_sheet_id"`
	MedicalScaleCode string             `json:"medical_scale_code"`
	Title            string             `json:"title"`
	Description      string             `json:"description"`
	Testee           *user.Testee       `json:"testee,omitempty"`
	InterpretItems   []InterpretItemDTO `json:"interpret_items"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

// InterpretItemDTO 解读项DTO
type InterpretItemDTO struct {
	FactorCode string    `json:"factor_code"`
	Title      string    `json:"title"`
	Score      int       `json:"score"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// InterpretReportFilterDTO 解读报告过滤条件DTO
type InterpretReportFilterDTO struct {
	AnswerSheetId    uint64 `json:"answer_sheet_id,omitempty"`
	MedicalScaleCode string `json:"medical_scale_code,omitempty"`
	Title            string `json:"title,omitempty"`
	TesteeId         uint64 `json:"testee_id,omitempty"`
	CreatedAfter     string `json:"created_after,omitempty"`  // 格式: "2006-01-02"
	CreatedBefore    string `json:"created_before,omitempty"` // 格式: "2006-01-02"
}

// InterpretReportCreateDTO 创建解读报告DTO
type InterpretReportCreateDTO struct {
	AnswerSheetId    uint64             `json:"answer_sheet_id" binding:"required"`
	MedicalScaleCode string             `json:"medical_scale_code" binding:"required"`
	Title            string             `json:"title" binding:"required"`
	Description      string             `json:"description"`
	InterpretItems   []InterpretItemDTO `json:"interpret_items"`
}

// InterpretReportUpdateDTO 更新解读报告DTO
type InterpretReportUpdateDTO struct {
	Title          string             `json:"title"`
	Description    string             `json:"description"`
	InterpretItems []InterpretItemDTO `json:"interpret_items"`
}

// InterpretItemCreateDTO 创建解读项DTO
type InterpretItemCreateDTO struct {
	FactorCode string `json:"factor_code" binding:"required"`
	Title      string `json:"title" binding:"required"`
	Score      int    `json:"score" binding:"required"`
	Content    string `json:"content" binding:"required"`
}

// InterpretItemUpdateDTO 更新解读项DTO
type InterpretItemUpdateDTO struct {
	Title   string `json:"title"`
	Score   int    `json:"score"`
	Content string `json:"content"`
}
