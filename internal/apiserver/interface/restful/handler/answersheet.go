package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/port"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/dto"
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
	var req dto.SaveAnswerSheetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errors.WithCode(errCode.ErrInvalidJSON, "invalid request body")
		return
	}

	// 保存原始答卷
	asBO := answersheet.NewAnswerSheet(req.QuestionnaireCode, req.QuestionnaireVersion,
		answersheet.WithTitle(req.Title),
		answersheet.WithWriter(user.NewWriter(user.NewUserID(req.WriterID), "")),
		answersheet.WithTestee(user.NewTestee(user.NewUserID(req.TesteeID), "")),
		answersheet.WithAnswers(dto.NewAnswerMapper().MapAnswersToBOs(req.Answers)),
	)
	answersheet, err := h.AnswersheetSaver.SaveOriginalAnswerSheet(c, asBO)
	if err != nil {
		errors.WithCode(errCode.ErrInternalServerError, "failed to save original answer sheet")
		return
	}

	response := dto.SaveAnswerSheetResponse{
		ID: answersheet.ID,
	}

	h.SuccessResponse(c, response)
}

// GetAnswerSheet 获取答卷
func (h *AnswersheetHandler) GetAnswerSheet(c *gin.Context) {

}
