package request

import (
	"encoding/json"
	"fmt"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ============= Assessment 相关请求 =============

// CreateAssessmentRequest 创建测评请求
type CreateAssessmentRequest struct {
	TesteeID             uint64  `json:"testee_id" valid:"required"`             // 受试者ID
	QuestionnaireCode    string  `json:"questionnaire_code" valid:"required"`    // 问卷编码（唯一标识）
	QuestionnaireVersion string  `json:"questionnaire_version" valid:"required"` // 问卷版本
	AnswerSheetID        uint64  `json:"answer_sheet_id" valid:"required"`       // 答卷ID
	ModelKind            *string `json:"model_kind"`                             // 解释模型类型（可选）
	ModelAlgorithm       *string `json:"model_algorithm"`                        // 解释模型算法（可选）
	ModelCode            *string `json:"model_code"`                             // 解释模型编码（可选）
	ModelVersion         *string `json:"model_version"`                          // 解释模型版本（可选）
	ModelTitle           *string `json:"model_title"`                            // 解释模型标题（可选）
	OriginType           string  `json:"origin_type" valid:"required"`           // 来源类型：adhoc/plan
	OriginID             *string `json:"origin_id"`                              // 来源ID（可选）
}

// SubmitAssessmentRequest 提交测评请求
type SubmitAssessmentRequest struct {
	AssessmentID uint64 `json:"assessment_id" valid:"required"` // 测评ID
}

// ListAssessmentsRequest 查询测评列表请求
type ListAssessmentsRequest struct {
	Page     int    `form:"page" json:"page"`           // 页码
	PageSize int    `form:"page_size" json:"page_size"` // 每页数量
	Status   string `form:"status" json:"status"`       // 状态筛选
	TesteeID uint64 `form:"testee_id" json:"testee_id"` // 受试者ID筛选
}

// ============= Score 相关请求 =============

// GetFactorTrendRequest 获取因子趋势请求
type GetFactorTrendRequest struct {
	TesteeID   uint64 `form:"testee_id" json:"testee_id" valid:"required"`     // 受试者ID
	FactorCode string `form:"factor_code" json:"factor_code" valid:"required"` // 因子编码
	Limit      int    `form:"limit" json:"limit"`                              // 返回记录数限制
}

// ============= Report 相关请求 =============

// ListReportsRequest 查询报告列表请求
type ListReportsRequest struct {
	TesteeID uint64 `form:"testee_id" json:"testee_id"` // 受试者ID
	Page     int    `form:"page" json:"page"`           // 页码
	PageSize int    `form:"page_size" json:"page_size"` // 每页数量
}

// ============= Evaluation 相关请求 =============

// BatchEvaluateRequest 批量评估请求
type BatchEvaluateRequest struct {
	AssessmentIDs []uint64 `json:"assessment_ids" valid:"required"` // 测评ID列表
}

// UnmarshalJSON accepts exact decimal strings and existing numeric IDs so a
// browser never needs to round a snowflake ID through a JavaScript Number.
func (r *BatchEvaluateRequest) UnmarshalJSON(data []byte) error {
	var wire struct {
		IDs []meta.ID `json:"assessment_ids"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if len(wire.IDs) == 0 {
		return fmt.Errorf("assessment_ids is required")
	}
	r.AssessmentIDs = make([]uint64, len(wire.IDs))
	for i, id := range wire.IDs {
		if id.IsZero() {
			return fmt.Errorf("assessment id must be positive")
		}
		r.AssessmentIDs[i] = id.Uint64()
	}
	return nil
}
