package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// QuestionnaireHandler 问卷处理器接口
type QuestionnaireHandler interface {
	// List 获取问卷列表
	List(c *gin.Context)
	// Get 获取问卷详情
	Get(c *gin.Context)
	// GetRaw 获取原始问卷
	GetRaw(c *gin.Context)
}

// questionnaireHandler 问卷处理器实现
type questionnaireHandler struct {
	client            grpc.QuestionnaireClient
	validationService validation.Service
}

// NewQuestionnaireHandler 创建新的问卷处理器
func NewQuestionnaireHandler(client grpc.QuestionnaireClient, validationService validation.Service) QuestionnaireHandler {
	return &questionnaireHandler{
		client:            client,
		validationService: validationService,
	}
}

// List 获取问卷列表
func (h *questionnaireHandler) List(c *gin.Context) {
	// 获取查询参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")

	// 构建请求
	req := &questionnaire.ListQuestionnairesRequest{
		Page:     int32(page),
		PageSize: int32(pageSize),
		Status:   status,
	}

	// 调用 GRPC 服务
	resp, err := h.client.ListQuestionnaires(c.Request.Context(), req)
	if err != nil {
		log.L(c).Errorf("Failed to list questionnaires: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取问卷列表失败",
			"error":   err.Error(),
		})
		return
	}

	// 转换响应格式
	questionnaires := make([]gin.H, 0, len(resp.Questionnaires))
	for _, q := range resp.Questionnaires {
		questionnaires = append(questionnaires, gin.H{
			"code":        q.Code,
			"title":       q.Title,
			"description": q.Description,
			"status":      q.Status,
			"created_at":  q.CreatedAt,
			"updated_at":  q.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"questionnaires": questionnaires,
			"total":          resp.Total,
			"page":           page,
			"page_size":      pageSize,
		},
		"message": "获取问卷列表成功",
	})
}

// Get 获取问卷详情
func (h *questionnaireHandler) Get(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "问卷代码不能为空",
		})
		return
	}

	// 校验问卷代码
	if err := h.validationService.ValidateQuestionnaireCode(c.Request.Context(), code); err != nil {
		log.L(c).Errorf("Invalid questionnaire code: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的问卷代码",
			"error":   err.Error(),
		})
		return
	}

	// 调用 GRPC 服务
	resp, err := h.client.GetQuestionnaire(c.Request.Context(), code)
	if err != nil {
		log.L(c).Errorf("Failed to get questionnaire: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取问卷详情失败",
			"error":   err.Error(),
		})
		return
	}

	// 转换响应格式
	questionnaire := gin.H{
		"code":        resp.Questionnaire.Code,
		"title":       resp.Questionnaire.Title,
		"description": resp.Questionnaire.Description,
		"status":      resp.Questionnaire.Status,
		"questions":   h.convertQuestions(resp.Questionnaire.Questions),
		"created_at":  resp.Questionnaire.CreatedAt,
		"updated_at":  resp.Questionnaire.UpdatedAt,
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"data":    questionnaire,
		"message": "获取问卷详情成功",
	})
}

// GetRaw 获取原始问卷（用于问卷收集系统）
func (h *questionnaireHandler) GetRaw(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "问卷代码不能为空",
		})
		return
	}

	// 校验问卷代码
	if err := h.validationService.ValidateQuestionnaireCode(c.Request.Context(), code); err != nil {
		log.L(c).Errorf("Invalid questionnaire code: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的问卷代码",
			"error":   err.Error(),
		})
		return
	}

	// 调用 GRPC 服务
	resp, err := h.client.GetQuestionnaire(c.Request.Context(), code)
	if err != nil {
		log.L(c).Errorf("Failed to get questionnaire: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取问卷详情失败",
			"error":   err.Error(),
		})
		return
	}

	// 返回原始问卷数据（用于小程序等收集系统）
	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"code":        resp.Questionnaire.Code,
			"title":       resp.Questionnaire.Title,
			"description": resp.Questionnaire.Description,
			"questions":   h.convertQuestionsForCollection(resp.Questionnaire.Questions),
		},
		"message": "获取原始问卷成功",
	})
}

// convertQuestions 转换问题格式（详细版）
func (h *questionnaireHandler) convertQuestions(questions []*questionnaire.Question) []gin.H {
	result := make([]gin.H, 0, len(questions))
	for _, q := range questions {
		question := gin.H{
			"code":        q.Code,
			"title":       q.Title,
			"type":        q.Type,
			"tips":        q.Tips,
			"placeholder": q.Placeholder,
		}

		// 添加选项（如果有）
		if len(q.Options) > 0 {
			options := make([]gin.H, 0, len(q.Options))
			for _, opt := range q.Options {
				options = append(options, gin.H{
					"code":    opt.Code,
					"content": opt.Content,
					"score":   opt.Score,
				})
			}
			question["options"] = options
		}

		// 添加验证规则（如果有）
		if len(q.ValidationRules) > 0 {
			rules := make([]gin.H, 0, len(q.ValidationRules))
			for _, rule := range q.ValidationRules {
				rules = append(rules, gin.H{
					"rule_type":    rule.RuleType,
					"target_value": rule.TargetValue,
				})
			}
			question["validation_rules"] = rules
		}

		result = append(result, question)
	}
	return result
}

// convertQuestionsForCollection 转换问题格式（收集版）
func (h *questionnaireHandler) convertQuestionsForCollection(questions []*questionnaire.Question) []gin.H {
	result := make([]gin.H, 0, len(questions))
	for _, q := range questions {
		question := gin.H{
			"code":        q.Code,
			"title":       q.Title,
			"type":        q.Type,
			"tips":        q.Tips,
			"placeholder": q.Placeholder,
		}

		// 只添加必要的选项信息
		if len(q.Options) > 0 {
			options := make([]gin.H, 0, len(q.Options))
			for _, opt := range q.Options {
				options = append(options, gin.H{
					"code":    opt.Code,
					"content": opt.Content,
				})
			}
			question["options"] = options
		}

		result = append(result, question)
	}
	return result
}
