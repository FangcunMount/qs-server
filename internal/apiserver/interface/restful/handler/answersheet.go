package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/mapper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/request"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/response"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// AnswersheetHandler 答卷处理器
type AnswersheetHandler struct {
	BaseHandler
	AnswersheetSaver   port.AnswerSheetSaver
	AnswersheetQueryer port.AnswerSheetQueryer
}

// NewAnswersheetHandler 创建答卷处理器
func NewAnswersheetHandler(
	answersheetSaver port.AnswerSheetSaver,
	answersheetQueryer port.AnswerSheetQueryer,
) *AnswersheetHandler {
	return &AnswersheetHandler{
		AnswersheetSaver:   answersheetSaver,
		AnswersheetQueryer: answersheetQueryer,
	}
}

// SaveAnswerSheet 保存答卷
func (h *AnswersheetHandler) SaveAnswerSheet(c *gin.Context) {
	var req request.SaveAnswerSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errors.WithCode(errCode.ErrInvalidJSON, "invalid request body")
		return
	}

	// 保存原始答卷
	asBO := answersheet.NewAnswerSheet(req.QuestionnaireCode, req.QuestionnaireVersion,
		answersheet.WithTitle(req.Title),
		answersheet.WithWriter(user.NewWriter(user.NewUserID(req.WriterID), "")),
		answersheet.WithTestee(user.NewTestee(user.NewUserID(req.TesteeID), "")),
		answersheet.WithAnswers(mapper.NewAnswerMapper().MapAnswersToBOs(req.Answers)),
	)
	answersheet, err := h.AnswersheetSaver.SaveOriginalAnswerSheet(c, asBO)
	if err != nil {
		errors.WithCode(errCode.ErrInternalServerError, "failed to save original answer sheet")
		return
	}

	response := response.SaveAnswerSheetResponse{
		ID: answersheet.GetID(),
	}

	h.SuccessResponse(c, response)
}

// GetAnswerSheet 获取答卷
func (h *AnswersheetHandler) GetAnswerSheet(c *gin.Context) {
	answersheetID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errors.WithCode(errCode.ErrInvalidJSON, "invalid request body")
		return
	}
	answersheet, err := h.AnswersheetQueryer.GetAnswerSheetByID(c, answersheetID)
	if err != nil {
		errors.WithCode(errCode.ErrInternalServerError, "failed to get answer sheet")
		return
	}

	response := response.GetAnswerSheetResponse{
		ID: answersheet.GetID(),
	}

	h.SuccessResponse(c, response)
}
