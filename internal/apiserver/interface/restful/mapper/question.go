package mapper

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"
)

// QuestionMapper 问题映射器
type QuestionMapper struct{}

// NewQuestionMapper 创建问题映射器
func NewQuestionMapper() *QuestionMapper {
	return &QuestionMapper{}
}

// ToDTO 将 viewmodel 转换为 DTO
func (m *QuestionMapper) ToDTO(vm *viewmodel.QuestionDTO) *dto.QuestionDTO {
	if vm == nil {
		return nil
	}

	questionDTO := &dto.QuestionDTO{
		Code:  vm.Code,
		Type:  vm.Type,
		Title: vm.Title,
		Tips:  vm.Tips,
	}

	if vm.Options != nil {
		questionDTO.Options = make([]dto.OptionDTO, len(vm.Options))
		for i, opt := range vm.Options {
			questionDTO.Options[i] = dto.OptionDTO{
				Code:    opt.Code,
				Content: opt.Content,
				Score:   opt.Score,
			}
		}
	}

	if vm.ValidationRules != nil {
		questionDTO.ValidationRules = make([]dto.ValidationRuleDTO, len(vm.ValidationRules))
		for i, rule := range vm.ValidationRules {
			questionDTO.ValidationRules[i] = dto.ValidationRuleDTO{
				RuleType:    rule.RuleType,
				TargetValue: rule.TargetValue,
			}
		}
	}

	if vm.CalculationRule != nil {
		questionDTO.CalculationRule = &dto.CalculationRuleDTO{
			FormulaType: vm.CalculationRule.FormulaType,
		}
	}

	return questionDTO
}

// ToViewModel 将 DTO 转换为 viewmodel
func (m *QuestionMapper) ToViewModel(dto *dto.QuestionDTO) *viewmodel.QuestionDTO {
	if dto == nil {
		return nil
	}

	vm := &viewmodel.QuestionDTO{
		Code:  dto.Code,
		Type:  dto.Type,
		Title: dto.Title,
		Tips:  dto.Tips,
	}

	if dto.Options != nil {
		vm.Options = make([]viewmodel.OptionDTO, len(dto.Options))
		for i, opt := range dto.Options {
			vm.Options[i] = viewmodel.OptionDTO{
				Code:    opt.Code,
				Content: opt.Content,
				Score:   opt.Score,
			}
		}
	}

	if dto.ValidationRules != nil {
		vm.ValidationRules = make([]viewmodel.ValidationRuleDTO, len(dto.ValidationRules))
		for i, rule := range dto.ValidationRules {
			vm.ValidationRules[i] = viewmodel.ValidationRuleDTO{
				RuleType:    rule.RuleType,
				TargetValue: rule.TargetValue,
			}
		}
	}

	if dto.CalculationRule != nil {
		vm.CalculationRule = &viewmodel.CalculationRuleDTO{
			FormulaType: dto.CalculationRule.FormulaType,
		}
	}

	return vm
}

// ToDTOs 将多个 viewmodel 转换为 DTOs
func (m *QuestionMapper) ToDTOs(vms []viewmodel.QuestionDTO) []dto.QuestionDTO {
	if vms == nil {
		return nil
	}

	dtos := make([]dto.QuestionDTO, len(vms))
	for i, vm := range vms {
		dtos[i] = *m.ToDTO(&vm)
	}
	return dtos
}

// ToViewModels 将多个 DTO 转换为 viewmodels
func (m *QuestionMapper) ToViewModels(dtos []dto.QuestionDTO) []viewmodel.QuestionDTO {
	if dtos == nil {
		return nil
	}

	vms := make([]viewmodel.QuestionDTO, len(dtos))
	for i, dto := range dtos {
		vms[i] = *m.ToViewModel(&dto)
	}
	return vms
}
