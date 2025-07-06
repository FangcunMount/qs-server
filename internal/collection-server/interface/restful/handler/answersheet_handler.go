package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	"github.com/yshujie/questionnaire-scale/pkg/pubsub"
)

// AnswersheetHandler 答卷处理器接口
type AnswersheetHandler interface {
	// Submit 提交答卷
	Submit(c *gin.Context)
	// Get 获取答卷详情
	Get(c *gin.Context)
	// List 获取答卷列表
	List(c *gin.Context)
}

// answersheetHandler 答卷处理器实现
type answersheetHandler struct {
	client            grpc.AnswersheetClient
	validationService validation.Service
	publisher         *pubsub.RedisPublisher
}

// NewAnswersheetHandler 创建新的答卷处理器
func NewAnswersheetHandler(client grpc.AnswersheetClient, validationService validation.Service, publisher *pubsub.RedisPublisher) AnswersheetHandler {
	return &answersheetHandler{
		client:            client,
		validationService: validationService,
		publisher:         publisher,
	}
}

// SubmitRequest 提交答卷请求
type SubmitRequest struct {
	QuestionnaireCode string                            `json:"questionnaire_code" binding:"required"`
	Title             string                            `json:"title"`
	WriterID          uint64                            `json:"writer_id"`
	TesteeID          uint64                            `json:"testee_id"`
	TesteeInfo        validation.TesteeInfo             `json:"testee_info"`
	Answers           []validation.AnswerValidationItem `json:"answers" binding:"required"`
}

// Submit 提交答卷
func (h *answersheetHandler) Submit(c *gin.Context) {
	var req SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.L(c).Errorf("Invalid request body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误",
			"error":   err.Error(),
		})
		return
	}

	// 构建校验请求
	validationReq := &validation.AnswersheetValidationRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Answers:           req.Answers,
		TesteeInfo:        req.TesteeInfo,
	}

	// 校验答卷
	if err := h.validationService.ValidateAnswersheet(c.Request.Context(), validationReq); err != nil {
		log.L(c).Errorf("Answersheet validation failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "答卷校验失败",
			"error":   err.Error(),
		})
		return
	}

	// 转换答案格式
	answers := make([]*answersheet.Answer, 0, len(req.Answers))
	for _, answer := range req.Answers {
		// 将答案值转换为 JSON 字符串
		valueBytes, err := json.Marshal(answer.Value)
		if err != nil {
			log.L(c).Errorf("Failed to marshal answer value: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "答案格式转换失败",
				"error":   err.Error(),
			})
			return
		}

		answers = append(answers, &answersheet.Answer{
			QuestionCode: answer.QuestionID,
			Value:        string(valueBytes),
		})
	}

	// 构建 GRPC 请求
	grpcReq := &answersheet.SaveAnswerSheetRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.Title,
		WriterId:          req.WriterID,
		TesteeId:          req.TesteeID,
		Answers:           answers,
	}

	// 调用 GRPC 服务
	resp, err := h.client.SaveAnswersheet(c.Request.Context(), grpcReq)
	if err != nil {
		log.L(c).Errorf("Failed to save answersheet: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "保存答卷失败",
			"error":   err.Error(),
		})
		return
	}

	// 发布答卷已保存消息
	if h.publisher != nil {
		message := &pubsub.ResponseSavedMessage{
			ResponseID:      strconv.FormatUint(resp.Id, 10),
			QuestionnaireID: req.QuestionnaireCode,
			UserID:          strconv.FormatUint(req.TesteeID, 10),
			SubmittedAt:     time.Now().Unix(),
		}

		if err := h.publisher.Publish(c.Request.Context(), "answersheet.saved", message); err != nil {
			log.L(c).Errorf("Failed to publish answersheet saved message: %v", err)
			// 不影响主流程，只记录错误
		} else {
			log.L(c).Infof("Published answersheet saved message for response ID: %s", resp.Id)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"id":      resp.Id,
			"message": resp.Message,
		},
		"message": "答卷提交成功",
	})
}

// Get 获取答卷详情
func (h *answersheetHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "答卷ID不能为空",
		})
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的答卷ID",
			"error":   err.Error(),
		})
		return
	}

	// 调用 GRPC 服务
	resp, err := h.client.GetAnswersheet(c.Request.Context(), id)
	if err != nil {
		log.L(c).Errorf("Failed to get answersheet: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取答卷详情失败",
			"error":   err.Error(),
		})
		return
	}

	// 转换响应格式
	answersheet := h.convertAnswersheet(resp.AnswerSheet)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"data":    answersheet,
		"message": "获取答卷详情成功",
	})
}

// List 获取答卷列表
func (h *answersheetHandler) List(c *gin.Context) {
	// 获取查询参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	questionnaireCode := c.Query("questionnaire_code")
	writerID, _ := strconv.ParseUint(c.Query("writer_id"), 10, 64)
	testeeID, _ := strconv.ParseUint(c.Query("testee_id"), 10, 64)

	// 构建请求
	req := &answersheet.ListAnswerSheetsRequest{
		QuestionnaireCode: questionnaireCode,
		WriterId:          writerID,
		TesteeId:          testeeID,
		Page:              int32(page),
		PageSize:          int32(pageSize),
	}

	// 调用 GRPC 服务
	resp, err := h.client.ListAnswersheets(c.Request.Context(), req)
	if err != nil {
		log.L(c).Errorf("Failed to list answersheets: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取答卷列表失败",
			"error":   err.Error(),
		})
		return
	}

	// 转换响应格式
	answersheets := make([]gin.H, 0, len(resp.AnswerSheets))
	for _, as := range resp.AnswerSheets {
		answersheets = append(answersheets, h.convertAnswersheetSummary(as))
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"answersheets": answersheets,
			"total":        resp.Total,
			"page":         page,
			"page_size":    pageSize,
		},
		"message": "获取答卷列表成功",
	})
}

// convertAnswersheet 转换答卷格式（详细版）
func (h *answersheetHandler) convertAnswersheet(as *answersheet.AnswerSheet) gin.H {
	// 转换答案
	answers := make([]gin.H, 0, len(as.Answers))
	for _, answer := range as.Answers {
		// 尝试解析 JSON 值
		var value interface{}
		if err := json.Unmarshal([]byte(answer.Value), &value); err != nil {
			// 如果解析失败，使用原始字符串
			value = answer.Value
		}

		answers = append(answers, gin.H{
			"question_code": answer.QuestionCode,
			"question_type": answer.QuestionType,
			"score":         answer.Score,
			"value":         value,
		})
	}

	return gin.H{
		"id":                    as.Id,
		"questionnaire_code":    as.QuestionnaireCode,
		"questionnaire_version": as.QuestionnaireVersion,
		"title":                 as.Title,
		"score":                 as.Score,
		"writer_id":             as.WriterId,
		"writer_name":           as.WriterName,
		"testee_id":             as.TesteeId,
		"testee_name":           as.TesteeName,
		"answers":               answers,
		"created_at":            as.CreatedAt,
		"updated_at":            as.UpdatedAt,
	}
}

// convertAnswersheetSummary 转换答卷格式（摘要版）
func (h *answersheetHandler) convertAnswersheetSummary(as *answersheet.AnswerSheet) gin.H {
	return gin.H{
		"id":                    as.Id,
		"questionnaire_code":    as.QuestionnaireCode,
		"questionnaire_version": as.QuestionnaireVersion,
		"title":                 as.Title,
		"score":                 as.Score,
		"writer_id":             as.WriterId,
		"writer_name":           as.WriterName,
		"testee_id":             as.TesteeId,
		"testee_name":           as.TesteeName,
		"created_at":            as.CreatedAt,
		"updated_at":            as.UpdatedAt,
	}
}
