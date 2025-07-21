package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	questionnaireapp "github.com/yshujie/questionnaire-scale/internal/collection-server/application/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/interface/restful/mapper"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/interface/restful/request"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// QuestionnaireHandler 问卷处理器接口
type QuestionnaireHandler interface {
	List(c *gin.Context)
	Get(c *gin.Context)
	GetRaw(c *gin.Context)
}

// questionnaireHandler 问卷处理器实现
type questionnaireHandler struct {
	questionnaireService questionnaireapp.Service
	questionnaireClient  grpc.QuestionnaireClient // 保留用于List操作
	mapper               *mapper.QuestionnaireMapper
}

// NewQuestionnaireHandler 创建新的问卷处理器
func NewQuestionnaireHandler(questionnaireService questionnaireapp.Service, questionnaireClient grpc.QuestionnaireClient) QuestionnaireHandler {
	return &questionnaireHandler{
		questionnaireService: questionnaireService,
		questionnaireClient:  questionnaireClient,
		mapper:               mapper.NewQuestionnaireMapper(),
	}
}

// List 获取问卷列表
func (h *questionnaireHandler) List(c *gin.Context) {
	var req request.QuestionnaireListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	// 使用mapper构建gRPC请求
	grpcReq := h.mapper.ToGRPCListRequest(&req)

	// 调用 GRPC 服务
	resp, err := h.questionnaireClient.ListQuestionnaires(c.Request.Context(), grpcReq)
	if err != nil {
		log.L(c).Errorf("Failed to list questionnaires: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取问卷列表失败",
			"error":   err.Error(),
		})
		return
	}

	// 使用mapper转换响应格式
	listResponse := h.mapper.ToQuestionnaireListResponse(resp, &req)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"data":    listResponse,
		"message": "获取问卷列表成功",
	})
}

// Get 获取问卷详情
func (h *questionnaireHandler) Get(c *gin.Context) {
	var req request.QuestionnaireGetRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "问卷代码格式错误",
			"error":   err.Error(),
		})
		return
	}

	code, err := h.mapper.ValidateAndGetCode(req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的问卷代码",
			"error":   err.Error(),
		})
		return
	}

	// 使用应用服务验证问卷代码
	if err := h.questionnaireService.ValidateQuestionnaireCode(c.Request.Context(), code); err != nil {
		log.L(c).Errorf("Invalid questionnaire code: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的问卷代码",
			"error":   err.Error(),
		})
		return
	}

	// 调用 GRPC 服务获取详情
	resp, err := h.questionnaireClient.GetQuestionnaire(c.Request.Context(), code)
	if err != nil {
		log.L(c).Errorf("Failed to get questionnaire: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取问卷详情失败",
			"error":   err.Error(),
		})
		return
	}

	if resp.Questionnaire == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "问卷不存在",
		})
		return
	}

	// 使用mapper转换响应格式
	questionnaireResponse := h.mapper.ToQuestionnaireResponse(resp.Questionnaire)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"data":    questionnaireResponse,
		"message": "获取问卷详情成功",
	})
}

// GetRaw 获取原始问卷（用于问卷收集系统）
func (h *questionnaireHandler) GetRaw(c *gin.Context) {
	var req request.QuestionnaireGetRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "问卷代码格式错误",
			"error":   err.Error(),
		})
		return
	}

	code, err := h.mapper.ValidateAndGetCode(req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的问卷代码",
			"error":   err.Error(),
		})
		return
	}

	// 使用应用服务验证问卷代码
	if err := h.questionnaireService.ValidateQuestionnaireCode(c.Request.Context(), code); err != nil {
		log.L(c).Errorf("Invalid questionnaire code: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的问卷代码",
			"error":   err.Error(),
		})
		return
	}

	// 调用 GRPC 服务获取详情
	resp, err := h.questionnaireClient.GetQuestionnaire(c.Request.Context(), code)
	if err != nil {
		log.L(c).Errorf("Failed to get questionnaire: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取问卷详情失败",
			"error":   err.Error(),
		})
		return
	}

	if resp.Questionnaire == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "问卷不存在",
		})
		return
	}

	// GetRaw返回原始格式，直接使用gRPC数据
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"data":    resp.Questionnaire,
		"message": "获取问卷成功",
	})
}
