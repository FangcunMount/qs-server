package handler

import (
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/mapper"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/viewmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/errors"
	"github.com/gin-gonic/gin"
)

// AnswerSheetHandler 答卷处理器
type AnswerSheetHandler struct {
	*BaseHandler
	saver   port.AnswerSheetSaver
	queryer port.AnswerSheetQueryer
	mapper  *mapper.AnswerSheetMapper
}

// NewAnswerSheetHandler 创建答卷处理器
func NewAnswerSheetHandler(saver port.AnswerSheetSaver, queryer port.AnswerSheetQueryer) *AnswerSheetHandler {
	return &AnswerSheetHandler{
		BaseHandler: &BaseHandler{},
		saver:       saver,
		queryer:     queryer,
		mapper:      mapper.NewAnswerSheetMapper(),
	}
}

// Save 保存答卷
// @Summary 保存答卷
// @Description 保存答卷
// @Tags answersheet
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param request body viewmodel.SaveAnswerSheetRequest true "保存答卷请求"
// @Success 200 {object} response.Response
// @Router /v1/answersheets [post]
func (h *AnswerSheetHandler) Save(c *gin.Context) {
	var req viewmodel.SaveAnswerSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.ErrorResponse(c, errors.WrapC(err, code.ErrBind, "参数绑定失败"))
		return
	}

	dto := h.mapper.ToAnswerSheetDTO(req)
	savedDTO, err := h.saver.SaveOriginalAnswerSheet(c.Request.Context(), dto)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	h.SuccessResponse(c, gin.H{
		"id": savedDTO.ID,
	})
}

// List 获取答卷列表
// @Summary 获取答卷列表
// @Description 获取答卷列表
// @Tags answersheet
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param questionnaire_code query string false "问卷编码"
// @Param questionnaire_version query string false "问卷版本"
// @Param writer_id query integer false "填写人ID"
// @Param testee_id query integer false "被试ID"
// @Param page query integer true "页码"
// @Param page_size query integer true "每页数量"
// @Success 200 {object} response.Response{data=response.ListAnswerSheetsResponse}
// @Router /v1/answersheets [get]
func (h *AnswerSheetHandler) List(c *gin.Context) {
	var req viewmodel.ListAnswerSheetsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.ErrorResponse(c, errors.WrapC(err, code.ErrBind, "参数绑定失败"))
		return
	}

	filter := h.mapper.ToAnswerSheetFilterDTO(req)
	sheets, total, err := h.queryer.GetAnswerSheetList(c.Request.Context(), filter, req.Page, req.PageSize)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	var vms []viewmodel.AnswerSheetViewModel
	for _, sheet := range sheets {
		vms = append(vms, h.mapper.ToAnswerSheetViewModel(sheet))
	}

	h.SuccessResponse(c, gin.H{
		"total": total,
		"items": vms,
	})
}

// Get 获取答卷详情
// @Summary 获取答卷详情
// @Description 获取答卷详情
// @Tags answersheet
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer 用户令牌"
// @Param id path integer true "答卷ID"
// @Success 200 {object} response.Response{data=response.GetAnswerSheetResponse}
// @Router /v1/answersheets/{id} [get]
func (h *AnswerSheetHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		h.ErrorResponse(c, errors.WithCode(code.ErrValidation, "无效的答卷ID"))
		return
	}

	detail, err := h.queryer.GetAnswerSheetByID(c.Request.Context(), id)
	if err != nil {
		h.ErrorResponse(c, err)
		return
	}

	vm := h.mapper.ToAnswerSheetDetailViewModel(*detail)
	h.SuccessResponse(c, vm)
}
