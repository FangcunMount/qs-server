package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/application/validation"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/infrastructure/grpc"
	internalpubsub "github.com/yshujie/questionnaire-scale/internal/pkg/pubsub"
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
	publisher         pubsub.Publisher
}

// NewAnswersheetHandler 创建新的答卷处理器
func NewAnswersheetHandler(client grpc.AnswersheetClient, validationService validation.Service, publisher pubsub.Publisher) AnswersheetHandler {
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
		// 根据问题类型处理答案值
		var valueStr string
		var err error

		switch answer.QuestionType {
		case "single_choice", "multiple_choice":
			// 选择题答案应该是字符串或字符串数组
			valueStr, err = h.convertChoiceAnswer(answer.Value)
		case "text", "textarea":
			// 文本类答案直接转换为字符串
			if str, ok := answer.Value.(string); ok {
				valueStr = str
			} else {
				err = fmt.Errorf("invalid text answer type: %T", answer.Value)
			}
		case "number", "rating":
			// 数值类答案需要特殊处理
			valueStr, err = h.convertNumberAnswer(answer.Value)
		default:
			// 其他类型答案统一转换为JSON
			valueBytes, e := json.Marshal(answer.Value)
			if e != nil {
				err = fmt.Errorf("failed to marshal answer value: %v", e)
			} else {
				valueStr = string(valueBytes)
			}
		}

		if err != nil {
			log.L(c).Errorf("Failed to convert answer value: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": "答案格式转换失败",
				"error":   err.Error(),
			})
			return
		}

		answers = append(answers, &answersheet.Answer{
			QuestionCode: answer.QuestionID,
			QuestionType: answer.QuestionType,
			Value:        valueStr,
		})
	}

	// 构建 GRPC 请求
	grpcReq := &answersheet.SaveAnswerSheetRequest{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: "1.0", // 添加问卷版本
		Title:                req.Title,
		WriterId:             req.WriterID,
		TesteeId:             req.TesteeID,
		Answers:              answers,
	}

	// 调用 GRPC 服务
	resp, err := h.client.SaveAnswersheet(c.Request.Context(), grpcReq)
	log.L(c).Infow("SaveAnswersheet", "resp", resp)
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
		// 创建答卷已保存数据
		answersheetData := &internalpubsub.AnswersheetSavedData{
			ResponseID:      strconv.FormatUint(resp.Id, 10),
			QuestionnaireID: req.QuestionnaireCode,
			UserID:          strconv.FormatUint(req.TesteeID, 10),
			SubmittedAt:     time.Now().Unix(),
		}

		// 创建答卷已保存消息
		message := internalpubsub.NewAnswersheetSavedMessage(
			internalpubsub.SourceCollectionServer,
			answersheetData,
		)

		if err := h.publisher.Publish(c.Request.Context(), "answersheet.saved", message); err != nil {
			log.L(c).Errorf("Failed to publish answersheet saved message: %v", err)
			// 不影响主流程，只记录错误
		} else {
			log.L(c).Infof("Published answersheet saved message for response ID: %s", answersheetData.ResponseID)
		}
	}

	// 返回响应
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "答卷保存成功",
		"data": gin.H{
			"id": resp.Id,
		},
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

	log.L(c).Infof("GetAnswersheet: %d", id)

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

	log.L(c).Infof("GetAnswersheet result: %v", resp.AnswerSheet)
	if resp.AnswerSheet == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "答卷不存在",
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

// convertChoiceAnswer 转换选择题答案
func (h *answersheetHandler) convertChoiceAnswer(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []interface{}:
		// 将选项数组转换为JSON字符串
		bytes, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("failed to marshal choice options: %v", err)
		}
		return string(bytes), nil
	default:
		return "", fmt.Errorf("invalid choice answer type: %T", value)
	}
}

// convertNumberAnswer 转换数值类答案
func (h *answersheetHandler) convertNumberAnswer(value interface{}) (string, error) {
	switch v := value.(type) {
	case float64:
		return fmt.Sprintf("%f", v), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case string:
		// 尝试解析字符串为数值
		if _, err := strconv.ParseFloat(v, 64); err == nil {
			return v, nil
		}
		return "", fmt.Errorf("invalid number string: %s", v)
	default:
		return "", fmt.Errorf("invalid number answer type: %T", value)
	}
}
