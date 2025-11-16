package mapper

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	answersheetapp "github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/request"
	"github.com/FangcunMount/qs-server/internal/collection-server/interface/restful/response"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswersheetMapper 答卷映射器，负责不同层级间的数据转换
type AnswersheetMapper struct{}

// NewAnswersheetMapper 创建新的答卷映射器
func NewAnswersheetMapper() *AnswersheetMapper {
	return &AnswersheetMapper{}
}

// ToServiceRequest 将HTTP请求转换为应用服务请求
func (m *AnswersheetMapper) ToServiceRequest(req *request.AnswersheetSubmitRequest) *answersheetapp.SubmitRequest {
	// 转换答案，直接使用请求中的问题类型
	answers := make([]*answersheetapp.Answer, len(req.Answers))
	for i, answer := range req.Answers {
		answers[i] = &answersheetapp.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType, // 直接使用请求中的问题类型
			Value:        answer.Value,
		}
	}

	return &answersheetapp.SubmitRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Title:             req.QuestionnaireCode, // 使用问卷代码作为默认title
		TesteeInfo: &answersheetapp.TesteeInfo{
			Name:   req.TesteeInfo.Name,
			Gender: req.TesteeInfo.Gender,
			Age:    req.TesteeInfo.Age,
			Email:  req.TesteeInfo.Email,
			Phone:  req.TesteeInfo.Phone,
		},
		Answers: answers,
	}
}

// ToSubmitResponse 将应用服务响应转换为HTTP响应
func (m *AnswersheetMapper) ToSubmitResponse(
	serviceResp *answersheetapp.SubmitResponse,
	req *request.AnswersheetSubmitRequest,
) *response.AnswersheetSubmitResponse {
	return &response.AnswersheetSubmitResponse{
		ID:                serviceResp.ID,
		QuestionnaireCode: req.QuestionnaireCode,
		Status:            serviceResp.Status,
		SubmissionTime:    serviceResp.CreatedAt,
		ValidationStatus:  "valid",
		Message:           serviceResp.Message,
	}
}

// ToAnswersheetResponse 将gRPC答卷数据转换为HTTP详情响应
func (m *AnswersheetMapper) ToAnswersheetResponse(as *answersheet.AnswerSheet) *response.AnswersheetResponse {
	// 转换答案
	answers := make([]response.Answer, len(as.Answers))
	for i, answer := range as.Answers {
		// 尝试解析 JSON 值
		var value interface{}
		if err := json.Unmarshal([]byte(answer.Value), &value); err != nil {
			value = answer.Value
		}

		points := int(answer.Score)
		answers[i] = response.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        value,
			Points:       &points,
		}
	}

	// 解析时间
	createdAt, _ := time.Parse(time.RFC3339, as.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, as.UpdatedAt)

	score := float64(as.Score)

	return &response.AnswersheetResponse{
		ID:                meta.ID(as.Id),
		QuestionnaireCode: as.QuestionnaireCode,
		TesteeInfo: response.TesteeResponseInfo{
			Name: as.TesteeName,
		},
		Answers:          answers,
		SubmissionTime:   createdAt,
		Status:           "submitted",
		TotalScore:       &score,
		ValidationStatus: "valid",
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
}

// ToAnswersheetItem 将gRPC答卷数据转换为HTTP列表项
func (m *AnswersheetMapper) ToAnswersheetItem(as *answersheet.AnswerSheet) response.AnswersheetItem {
	// 解析时间
	createdAt, _ := time.Parse(time.RFC3339, as.CreatedAt)
	submissionTime := createdAt

	return response.AnswersheetItem{
		ID:                meta.ID(as.Id),
		QuestionnaireCode: as.QuestionnaireCode,
		TesteeName:        as.TesteeName,
		SubmissionTime:    submissionTime,
		Status:            "submitted",
		ValidationStatus:  "valid",
	}
}

// ToAnswersheetListResponse 将gRPC答卷列表转换为HTTP列表响应
func (m *AnswersheetMapper) ToAnswersheetListResponse(
	grpcResp *answersheet.ListAnswerSheetsResponse,
	req *request.AnswersheetListRequest,
) *response.AnswersheetListResponse {
	// 转换答卷列表
	answersheets := make([]response.AnswersheetItem, len(grpcResp.AnswerSheets))
	for i, as := range grpcResp.AnswerSheets {
		answersheets[i] = m.ToAnswersheetItem(as)
	}

	// 计算总页数
	totalPages := int((grpcResp.Total + int64(req.PageSize) - 1) / int64(req.PageSize))

	return &response.AnswersheetListResponse{
		Total:        grpcResp.Total,
		Page:         req.Page,
		PageSize:     req.PageSize,
		TotalPages:   totalPages,
		Answersheets: answersheets,
	}
}

// ToGRPCListRequest 将HTTP列表请求转换为gRPC请求
func (m *AnswersheetMapper) ToGRPCListRequest(req *request.AnswersheetListRequest) *answersheet.ListAnswerSheetsRequest {
	grpcReq := &answersheet.ListAnswerSheetsRequest{
		QuestionnaireCode: req.QuestionnaireCode,
		Page:              int32(req.Page),
		PageSize:          int32(req.PageSize),
	}

	// 解析testee_id
	if !req.TesteeID.IsZero() {
		if testeeID, err := strconv.ParseUint(req.TesteeID.String(), 10, 64); err == nil {
			grpcReq.TesteeId = testeeID
		}
	}

	return grpcReq
}

// ValidateAndConvertID 验证并转换ID字符串
func (m *AnswersheetMapper) ValidateAndConvertID(idStr string) (uint64, error) {
	return strconv.ParseUint(idStr, 10, 64)
}
