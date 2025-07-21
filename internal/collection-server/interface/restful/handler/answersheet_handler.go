package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	answersheetapp "github.com/yshujie/questionnaire-scale/internal/collection-server/application/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/interface/restful/mapper"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/interface/restful/request"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// AnswersheetHandler 答卷处理器接口
type AnswersheetHandler interface {
	Submit(c *gin.Context)
	Get(c *gin.Context)
	List(c *gin.Context)
}

// answersheetHandler 答卷处理器实现
type answersheetHandler struct {
	answersheetService answersheetapp.Service
	answersheetClient  grpc.AnswersheetClient
	mapper             *mapper.AnswersheetMapper
}

// NewAnswersheetHandler 创建新的答卷处理器
func NewAnswersheetHandler(answersheetService answersheetapp.Service, answersheetClient grpc.AnswersheetClient) AnswersheetHandler {
	return &answersheetHandler{
		answersheetService: answersheetService,
		answersheetClient:  answersheetClient,
		mapper:             mapper.NewAnswersheetMapper(),
	}
}

// Submit 提交答卷
func (h *answersheetHandler) Submit(c *gin.Context) {
	var req request.AnswersheetSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.L(c).Errorf("Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	serviceReq := h.mapper.ToServiceRequest(&req)
	serviceResponse, err := h.answersheetService.SubmitAnswersheet(c.Request.Context(), serviceReq)
	if err != nil {
		log.L(c).Errorf("Failed to submit answersheet: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "提交答卷失败",
			"error":   err.Error(),
		})
		return
	}

	response := h.mapper.ToSubmitResponse(serviceResponse, &req)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "答卷提交成功",
		"data":    response,
	})
}

// Get 获取答卷详情
func (h *answersheetHandler) Get(c *gin.Context) {
	var req request.AnswersheetGetRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "答卷ID格式错误",
			"error":   err.Error(),
		})
		return
	}

	id, err := h.mapper.ValidateAndConvertID(req.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的答卷ID",
			"error":   err.Error(),
		})
		return
	}

	resp, err := h.answersheetClient.GetAnswersheet(c.Request.Context(), id)
	if err != nil {
		log.L(c).Errorf("Failed to get answersheet: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取答卷详情失败",
			"error":   err.Error(),
		})
		return
	}

	if resp.AnswerSheet == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "答卷不存在",
		})
		return
	}

	answersheetResponse := h.mapper.ToAnswersheetResponse(resp.AnswerSheet)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"data":    answersheetResponse,
		"message": "获取答卷详情成功",
	})
}

// List 获取答卷列表
func (h *answersheetHandler) List(c *gin.Context) {
	var req request.AnswersheetListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	grpcReq := h.mapper.ToGRPCListRequest(&req)
	resp, err := h.answersheetClient.ListAnswersheets(c.Request.Context(), grpcReq)
	if err != nil {
		log.L(c).Errorf("Failed to list answersheets: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取答卷列表失败",
			"error":   err.Error(),
		})
		return
	}

	listResponse := h.mapper.ToAnswersheetListResponse(resp, &req)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"data":    listResponse,
		"message": "获取答卷列表成功",
	})
}
