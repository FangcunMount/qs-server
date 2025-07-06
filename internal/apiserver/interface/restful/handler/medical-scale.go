package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	medicalScale "github.com/yshujie/questionnaire-scale/internal/apiserver/application/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/request"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/response"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"
	errorCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// MedicalScaleHandler 医学量表处理器
type MedicalScaleHandler struct {
	BaseHandler
	creator medicalScale.Creator
	queryer medicalScale.Queryer
	editor  medicalScale.Editor
}

// NewMedicalScaleHandler 创建医学量表处理器
func NewMedicalScaleHandler(
	creator medicalScale.Creator,
	queryer medicalScale.Queryer,
	editor medicalScale.Editor,
) *MedicalScaleHandler {
	return &MedicalScaleHandler{
		creator: creator,
		queryer: queryer,
		editor:  editor,
	}
}

// Create 创建医学量表
// @Summary 创建医学量表
// @Description 创建一个新的医学量表，包含基础信息
// @Tags MedicalScale
// @Accept json
// @Produce json
// @Param request body request.CreateMedicalScaleRequest true "创建医学量表请求"
// @Success 200 {object} response.MedicalScaleResponse
// @Router /api/v1/medical-scales [post]
func (h *MedicalScaleHandler) Create(c *gin.Context) {
	var req request.CreateMedicalScaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.ErrorResponse(c, errors.WithCode(errorCode.ErrBind, "参数验证失败"))
		return
	}

	// 创建医学量表DTO
	medicalScaleDTO := &dto.MedicalScaleDTO{
		Code:              req.Code,
		Title:             req.Title,
		QuestionnaireCode: req.QuestionnaireCode,
	}

	// 创建医学量表
	scale, err := h.creator.Create(c.Request.Context(), medicalScaleDTO)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, &response.MedicalScaleResponse{
		Data: h.convertDTOToVM(scale),
	})
}

// UpdateBaseInfo 更新医学量表基础信息
// @Summary 更新医学量表基础信息
// @Description 更新医学量表的标题和问卷绑定信息
// @Tags MedicalScale
// @Accept json
// @Produce json
// @Param code path string true "医学量表代码"
// @Param request body request.UpdateMedicalScaleRequest true "更新医学量表请求"
// @Success 200 {object} response.MedicalScaleResponse
// @Router /api/v1/medical-scales/{code} [put]
func (h *MedicalScaleHandler) UpdateBaseInfo(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		h.ErrorResponse(c, errors.WithCode(errorCode.ErrValidation, "医学量表代码不能为空"))
		return
	}

	var req request.UpdateMedicalScaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.ErrorResponse(c, errors.WithCode(errorCode.ErrBind, "参数验证失败"))
		return
	}

	// 创建医学量表DTO
	medicalScaleDTO := &dto.MedicalScaleDTO{
		Code:              code,
		Title:             req.Title,
		QuestionnaireCode: req.QuestionnaireCode,
	}

	// 更新医学量表
	scale, err := h.editor.EditBasicInfo(c.Request.Context(), medicalScaleDTO)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, &response.MedicalScaleResponse{
		Data: h.convertDTOToVM(scale),
	})
}

// UpdateFactor 更新医学量表因子
// @Summary 更新医学量表因子
// @Description 更新医学量表的因子信息，如果因子不存在则创建新因子
// @Tags MedicalScale
// @Accept json
// @Produce json
// @Param code path string true "医学量表代码"
// @Param request body request.UpdateMedicalScaleFactorRequest true "更新因子请求"
// @Success 200 {object} response.MedicalScaleResponse
// @Router /api/v1/medical-scales/{code}/factors [put]
func (h *MedicalScaleHandler) UpdateFactor(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		h.ErrorResponse(c, errors.WithCode(errorCode.ErrValidation, "医学量表代码不能为空"))
		return
	}

	var req request.UpdateMedicalScaleFactorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.ErrorResponse(c, errors.WithCode(errorCode.ErrBind, "参数验证失败"))
		return
	}

	// 转换为因子DTO
	factorDTO := dto.FactorDTO{
		Code:       req.Code,
		Title:      req.Title,
		FactorType: req.FactorType,
	}

	// 处理计算规则
	factorDTO.CalculationRule = &dto.CalculationRuleDTO{
		FormulaType: req.CalculationRule.FormulaType,
		SourceCodes: req.CalculationRule.SourceCodes,
	}

	// 处理解读规则（取第一个解读规则）
	if len(req.InterpretRules) > 0 {
		interpretRule := req.InterpretRules[0]
		factorDTO.InterpretRule = &dto.InterpretRuleDTO{
			ScoreRange: dto.ScoreRangeDTO{
				MinScore: interpretRule.ScoreRange.MinScore,
				MaxScore: interpretRule.ScoreRange.MaxScore,
			},
			Content: interpretRule.Content,
		}
	}

	// 更新因子
	scale, err := h.editor.UpdateFactors(c.Request.Context(), code, []dto.FactorDTO{factorDTO})
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	c.JSON(http.StatusOK, &response.MedicalScaleResponse{
		Data: h.convertDTOToVM(scale),
	})
}

// Get 获取医学量表详情
// @Summary 获取医学量表详情
// @Description 获取医学量表的完整信息，包括基础信息和因子列表
// @Tags MedicalScale
// @Accept json
// @Produce json
// @Param code path string true "医学量表代码"
// @Success 200 {object} response.MedicalScaleResponse
// @Router /api/v1/medical-scales/{code} [get]
func (h *MedicalScaleHandler) Get(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		h.ErrorResponse(c, errors.WithCode(errorCode.ErrValidation, "医学量表代码不能为空"))
		return
	}

	// 查询医学量表
	scale, err := h.queryer.GetMedicalScaleByCode(c.Request.Context(), code)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	if scale == nil {
		h.ErrorResponse(c, errors.WithCode(errorCode.ErrMedicalScaleNotFound, "医学量表不存在"))
		return
	}

	c.JSON(http.StatusOK, &response.MedicalScaleResponse{
		Data: h.convertDTOToVM(scale),
	})
}

// convertDTOToVM 将DTO转换为视图模型
func (h *MedicalScaleHandler) convertDTOToVM(dto *dto.MedicalScaleDTO) *viewmodel.MedicalScaleVM {
	if dto == nil {
		return nil
	}

	vm := &viewmodel.MedicalScaleVM{
		ID:                dto.ID,
		Code:              dto.Code,
		Title:             dto.Title,
		QuestionnaireCode: dto.QuestionnaireCode,
		Factors:           make([]viewmodel.FactorVM, 0, len(dto.Factors)),
	}

	for _, factor := range dto.Factors {
		factorVM := viewmodel.FactorVM{
			Code:       factor.Code,
			Title:      factor.Title,
			FactorType: factor.FactorType,
		}

		// 处理计算规则
		if factor.CalculationRule != nil {
			factorVM.CalculationRule = viewmodel.CalculationRuleVM{
				FormulaType: factor.CalculationRule.FormulaType,
				SourceCodes: factor.CalculationRule.SourceCodes,
			}
		}

		// 处理解读规则
		if factor.InterpretRule != nil {
			factorVM.InterpretRules = []viewmodel.InterpretRuleVM{
				{
					ScoreRange: viewmodel.ScoreRangeVM{
						MinScore: factor.InterpretRule.ScoreRange.MinScore,
						MaxScore: factor.InterpretRule.ScoreRange.MaxScore,
					},
					Content: factor.InterpretRule.Content,
				},
			}
		}

		vm.Factors = append(vm.Factors, factorVM)
	}

	return vm
}
