package mapper

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/viewmodel"
)

// AnswerSheetMapper ViewModel 和 DTO 转换器
type AnswerSheetMapper struct{}

// NewAnswerSheetMapper 创建答卷映射器
func NewAnswerSheetMapper() *AnswerSheetMapper {
	return &AnswerSheetMapper{}
}

// ToAnswerDTO 将答案视图模型转换为 DTO
func (m *AnswerSheetMapper) ToAnswerDTO(vm viewmodel.AnswerDTO) dto.AnswerDTO {
	return dto.AnswerDTO{
		QuestionCode: vm.QuestionCode,
		QuestionType: vm.QuestionType,
		Value:        vm.Value,
	}
}

// ToAnswerDTOs 将答案视图模型列表转换为 DTO 列表
func (m *AnswerSheetMapper) ToAnswerDTOs(vms []viewmodel.AnswerDTO) []dto.AnswerDTO {
	dtos := make([]dto.AnswerDTO, len(vms))
	for i, vm := range vms {
		dtos[i] = m.ToAnswerDTO(vm)
	}
	return dtos
}

// ToAnswerViewModel 将答案 DTO 转换为视图模型
func (m *AnswerSheetMapper) ToAnswerViewModel(dto dto.AnswerDTO) viewmodel.AnswerDTO {
	return viewmodel.AnswerDTO{
		QuestionCode: dto.QuestionCode,
		QuestionType: dto.QuestionType,
		Score:        dto.Score,
		Value:        dto.Value,
	}
}

// ToAnswerViewModels 将答案 DTO 列表转换为视图模型列表
func (m *AnswerSheetMapper) ToAnswerViewModels(dtos []dto.AnswerDTO) []viewmodel.AnswerDTO {
	vms := make([]viewmodel.AnswerDTO, len(dtos))
	for i, dto := range dtos {
		vms[i] = m.ToAnswerViewModel(dto)
	}
	return vms
}

// ToAnswerSheetDTO 将保存请求转换为 DTO
func (m *AnswerSheetMapper) ToAnswerSheetDTO(req viewmodel.SaveAnswerSheetRequest) dto.AnswerSheetDTO {
	return dto.AnswerSheetDTO{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		WriterID:             req.WriterID,
		TesteeID:             req.TesteeID,
		Answers:              m.ToAnswerDTOs(req.Answers),
	}
}

// ToAnswerSheetFilterDTO 将查询请求转换为过滤 DTO
func (m *AnswerSheetMapper) ToAnswerSheetFilterDTO(req viewmodel.ListAnswerSheetsRequest) dto.AnswerSheetDTO {
	return dto.AnswerSheetDTO{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		WriterID:             req.WriterID,
		TesteeID:             req.TesteeID,
	}
}

// ToAnswerSheetViewModel 将答卷 DTO 转换为视图模型
func (m *AnswerSheetMapper) ToAnswerSheetViewModel(dto dto.AnswerSheetDTO) viewmodel.AnswerSheetViewModel {
	return viewmodel.AnswerSheetViewModel{
		ID:                   dto.ID,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
		Title:                dto.Title,
		Score:                dto.Score,
		WriterID:             dto.WriterID,
		TesteeID:             dto.TesteeID,
		Answers:              m.ToAnswerViewModels(dto.Answers),
	}
}

// ToAnswerSheetDetailViewModel 将答卷详情 DTO 转换为视图模型
func (m *AnswerSheetMapper) ToAnswerSheetDetailViewModel(dto dto.AnswerSheetDetailDTO) viewmodel.AnswerSheetDetailViewModel {
	return viewmodel.AnswerSheetDetailViewModel{
		AnswerSheet:   m.ToAnswerSheetViewModel(dto.AnswerSheet),
		WriterName:    dto.WriterName,
		TesteeName:    dto.TesteeName,
		Questionnaire: dto.Questionnaire,
		CreatedAt:     dto.CreatedAt,
		UpdatedAt:     dto.UpdatedAt,
	}
}
