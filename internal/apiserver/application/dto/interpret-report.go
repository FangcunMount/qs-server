package dto

import (
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// InterpretReportDTO 解读报告DTO
type InterpretReportDTO struct {
	ID               meta.ID            `json:"id"`
	AnswerSheetId    meta.ID            `json:"answer_sheet_id"`
	MedicalScaleCode string             `json:"medical_scale_code"`
	Title            string             `json:"title"`
	Description      string             `json:"description"`
	Testee           interface{}        `json:"testee,omitempty"` // TODO: 重构为 actor.TesteeRef
	InterpretItems   []InterpretItemDTO `json:"interpret_items"`
}

// InterpretItemDTO 解读项DTO
type InterpretItemDTO struct {
	FactorCode string  `json:"factor_code"`
	Title      string  `json:"title"`
	Score      float64 `json:"score"`
	Content    string  `json:"content"`
}

// InterpretReportFilterDTO 解读报告过滤条件DTO
type InterpretReportFilterDTO struct {
	AnswerSheetId    meta.ID `json:"answer_sheet_id,omitempty"`
	MedicalScaleCode string  `json:"medical_scale_code,omitempty"`
	Title            string  `json:"title,omitempty"`
	TesteeId         uint64  `json:"testee_id,omitempty"`
	CreatedAfter     string  `json:"created_after,omitempty"`  // 格式: "2006-01-02"
	CreatedBefore    string  `json:"created_before,omitempty"` // 格式: "2006-01-02"
}

// InterpretReportCreateDTO 创建解读报告DTO
type InterpretReportCreateDTO struct {
	AnswerSheetId    meta.ID            `json:"answer_sheet_id" binding:"required"`
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
	FactorCode string  `json:"factor_code" binding:"required"`
	Title      string  `json:"title" binding:"required"`
	Score      float64 `json:"score" binding:"required"`
	Content    string  `json:"content" binding:"required"`
}

// InterpretItemUpdateDTO 更新解读项DTO
type InterpretItemUpdateDTO struct {
	Title   string  `json:"title"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
}
