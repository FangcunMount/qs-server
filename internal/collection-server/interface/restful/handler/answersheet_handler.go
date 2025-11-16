package handler

import (
	"fmt"
	"net/http"

	answersheetapp "github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/infrastructure/grpc"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/mapper"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/response"
	"github.com/FangcunMount/qs-server/pkg/log"
	"github.com/gin-gonic/gin"
)

// AnswersheetHandler 答卷处理器接口
type AnswersheetHandler interface {
	Submit(c *gin.Context)
	Get(c *gin.Context)
	List(c *gin.Context)
}

// answersheetHandler 答卷处理器
type answersheetHandler struct {
	answersheetService answersheetapp.Service
	answersheetClient  grpc.AnswersheetClient
	mapper             *mapper.AnswersheetMapper
}

// NewAnswersheetHandler 创建答卷处理器
func NewAnswersheetHandler(answersheetService answersheetapp.Service, answersheetClient grpc.AnswersheetClient) *answersheetHandler {
	return &answersheetHandler{
		answersheetService: answersheetService,
		answersheetClient:  answersheetClient,
		mapper:             mapper.NewAnswersheetMapper(),
	}
}

// Submit 提交答卷
func (h *answersheetHandler) Submit(c *gin.Context) {
	ctx := c.Request.Context()

	var req request.AnswersheetSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.L(ctx).Errorf("Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, response.ErrorResponse{
			Error:   "INVALID_REQUEST",
			Message: fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// 记录接收到的请求
	log.L(ctx).Infof("Received answersheet submit request: questionnaire_code=%s, testee_name=%s, answer_count=%d",
		req.QuestionnaireCode, req.TesteeInfo.Name, len(req.Answers))

	// 记录详细的答案信息（可选，用于调试）
	if log.L(ctx).V(1).Enabled() {
		for i, answer := range req.Answers {
			log.L(ctx).V(1).Infof("Answer[%d]: question_code=%s, question_type=%s, value=%v",
				i, answer.QuestionCode, answer.QuestionType, answer.Value)
		}
	}

	log.L(ctx).Info("Starting answersheet conversion...")
	// 直接转换请求（问题类型已在请求中提供）
	serviceReq := h.mapper.ToServiceRequest(&req)
	log.L(ctx).Info("Answersheet conversion completed")

	log.L(ctx).Info("Calling answersheet application service...")
	// 调用应用服务
	serviceResponse, err := h.answersheetService.SubmitAnswersheet(ctx, serviceReq)
	if err != nil {
		log.L(ctx).Errorf("Failed to submit answersheet: %v", err)
		c.JSON(http.StatusInternalServerError, response.ErrorResponse{
			Error:   "SUBMISSION_FAILED",
			Message: fmt.Sprintf("Failed to submit answersheet: %v", err),
		})
		return
	}

	log.L(ctx).Infof("Answersheet submitted successfully: id=%s, status=%s",
		serviceResponse.ID, serviceResponse.Status)

	// 转换响应
	resp := h.mapper.ToSubmitResponse(serviceResponse, &req)

	log.L(ctx).Infof("Returning response: questionnaire_code=%s, submission_time=%v",
		resp.QuestionnaireCode, resp.SubmissionTime)

	c.JSON(http.StatusOK, resp)
}

// Get 获取答卷详情
func (h *answersheetHandler) Get(c *gin.Context) {
	// 暂时返回未实现
	c.JSON(http.StatusNotImplemented, response.ErrorResponse{
		Error:   "NOT_IMPLEMENTED",
		Message: "Get answersheet detail not implemented yet",
	})
}

// List 获取答卷列表
func (h *answersheetHandler) List(c *gin.Context) {
	// 暂时返回未实现
	c.JSON(http.StatusNotImplemented, response.ErrorResponse{
		Error:   "NOT_IMPLEMENTED",
		Message: "List answersheets not implemented yet",
	})
}
