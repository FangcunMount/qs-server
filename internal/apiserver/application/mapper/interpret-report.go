package mapper

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	interpretreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpret-report"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// InterpretReportMapper 解读报告映射器
type InterpretReportMapper struct{}

// NewInterpretReportMapper 创建解读报告映射器
func NewInterpretReportMapper() *InterpretReportMapper {
	return &InterpretReportMapper{}
}

// ToDTO 将领域对象转换为DTO
func (m *InterpretReportMapper) ToDTO(report *interpretreport.InterpretReport) *dto.InterpretReportDTO {
	if report == nil {
		return nil
	}

	testee := report.GetTestee()
	reportDTO := &dto.InterpretReportDTO{
		ID:               report.GetID(),
		AnswerSheetId:    report.GetAnswerSheetId(),
		MedicalScaleCode: report.GetMedicalScaleCode(),
		Title:            report.GetTitle(),
		Description:      report.GetDescription(),
		Testee:           &testee,
	}

	// 转换解读项
	items := report.GetInterpretItems()
	reportDTO.InterpretItems = make([]dto.InterpretItemDTO, len(items))
	for i, item := range items {
		reportDTO.InterpretItems[i] = m.InterpretItemToDTO(item)
	}

	return reportDTO
}

// ToDTOList 将领域对象列表转换为DTO列表
func (m *InterpretReportMapper) ToDTOList(reports []*interpretreport.InterpretReport) []dto.InterpretReportDTO {
	if reports == nil {
		return nil
	}

	dtoList := make([]dto.InterpretReportDTO, len(reports))
	for i, report := range reports {
		if reportDTO := m.ToDTO(report); reportDTO != nil {
			dtoList[i] = *reportDTO
		}
	}

	return dtoList
}

// ToDomain 将DTO转换为领域对象
func (m *InterpretReportMapper) ToDomain(reportDTO *dto.InterpretReportDTO) *interpretreport.InterpretReport {
	if reportDTO == nil {
		return nil
	}

	// 转换解读项
	items := make([]interpretreport.InterpretItem, len(reportDTO.InterpretItems))
	for i, itemDTO := range reportDTO.InterpretItems {
		items[i] = m.InterpretItemToDomain(itemDTO)
	}

	// 创建解读报告
	report := interpretreport.NewInterpretReport(
		reportDTO.AnswerSheetId,
		reportDTO.MedicalScaleCode,
		reportDTO.Title,
		interpretreport.WithID(meta.ID(reportDTO.ID)),
		interpretreport.WithDescription(reportDTO.Description),
		interpretreport.WithInterpretItems(items),
	)

	if reportDTO.Testee != nil {
		report = interpretreport.NewInterpretReport(
			reportDTO.AnswerSheetId,
			reportDTO.MedicalScaleCode,
			reportDTO.Title,
			interpretreport.WithID(meta.ID(reportDTO.ID)),
			interpretreport.WithDescription(reportDTO.Description),
			interpretreport.WithTestee(*reportDTO.Testee),
			interpretreport.WithInterpretItems(items),
		)
	}

	return report
}

// InterpretItemToDTO 将解读项领域对象转换为DTO
func (m *InterpretReportMapper) InterpretItemToDTO(item interpretreport.InterpretItem) dto.InterpretItemDTO {
	return dto.InterpretItemDTO{
		FactorCode: item.GetFactorCode(),
		Title:      item.GetTitle(),
		Score:      item.GetScore(),
		Content:    item.GetContent(),
	}
}

// InterpretItemToDomain 将解读项DTO转换为领域对象
func (m *InterpretReportMapper) InterpretItemToDomain(dto dto.InterpretItemDTO) interpretreport.InterpretItem {
	return interpretreport.NewInterpretItem(
		dto.FactorCode,
		dto.Title,
		dto.Score,
		dto.Content,
	)
}

// CreateDTOToDomain 将创建DTO转换为领域对象
func (m *InterpretReportMapper) CreateDTOToDomain(createDTO *dto.InterpretReportCreateDTO) *interpretreport.InterpretReport {
	if createDTO == nil {
		return nil
	}

	// 转换解读项
	items := make([]interpretreport.InterpretItem, len(createDTO.InterpretItems))
	for i, itemDTO := range createDTO.InterpretItems {
		items[i] = m.InterpretItemToDomain(itemDTO)
	}

	return interpretreport.NewInterpretReport(
		createDTO.AnswerSheetId,
		createDTO.MedicalScaleCode,
		createDTO.Title,
		interpretreport.WithDescription(createDTO.Description),
		interpretreport.WithInterpretItems(items),
	)
}

// UpdateDTOToDomain 将更新DTO应用到领域对象
func (m *InterpretReportMapper) UpdateDTOToDomain(report *interpretreport.InterpretReport, updateDTO *dto.InterpretReportUpdateDTO) {
	if report == nil || updateDTO == nil {
		return
	}

	// 更新基本信息
	if updateDTO.Title != "" {
		report.UpdateTitle(updateDTO.Title)
	}
	if updateDTO.Description != "" {
		report.UpdateDescription(updateDTO.Description)
	}

	// 更新解读项
	if updateDTO.InterpretItems != nil {
		items := make([]interpretreport.InterpretItem, len(updateDTO.InterpretItems))
		for i, itemDTO := range updateDTO.InterpretItems {
			items[i] = m.InterpretItemToDomain(itemDTO)
		}
		report.SetInterpretItems(items)
	}
}
