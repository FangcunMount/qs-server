package request

import "encoding/json"

type CreateAssessmentModelRequest struct {
	Code                 string   `json:"code"`
	// 模型族；R128b canonical 为 typology，读兼容 personality。
	Kind                 string   `json:"kind" valid:"required~模型类型不能为空" example:"typology" enums:"typology,personality,behavioral_rating,cognitive,custom"`
	SubKind              string   `json:"sub_kind" example:"typology"`
	Algorithm            string   `json:"algorithm"`
	// 产品通道；人格测评为 typology。
	ProductChannel       string   `json:"product_channel" example:"typology" enums:"typology,personality"`
	Title                string   `json:"title" valid:"required~模型标题不能为空"`
	Description          string   `json:"description"`
	Category             string   `json:"category"`
	Tags                 []string `json:"tags"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
}

type UpdateAssessmentModelBasicInfoRequest struct {
	Title          string   `json:"title" valid:"required~模型标题不能为空"`
	Description    string   `json:"description"`
	SubKind        string   `json:"sub_kind"`
	Algorithm      string   `json:"algorithm"`
	ProductChannel string   `json:"product_channel"`
	Category       string   `json:"category"`
	Tags           []string `json:"tags"`
}

type BindAssessmentModelQuestionnaireRequest struct {
	QuestionnaireCode    string `json:"questionnaire_code" valid:"required~问卷编码不能为空"`
	QuestionnaireVersion string `json:"questionnaire_version" valid:"required~问卷版本不能为空"`
}

type UpdateAssessmentModelDefinitionRequest struct {
	Kind          string          `json:"kind"`
	SubKind       string          `json:"sub_kind"`
	Algorithm     string          `json:"algorithm"`
	PayloadFormat string          `json:"payload_format"`
	Payload       json.RawMessage `json:"payload" valid:"required~模型定义 payload 不能为空"`
}

type ApplyAssessmentModelCodesRequest struct {
	Target string `json:"target" valid:"required~编码目标不能为空"`
	Count  int    `json:"count"`
}

type PreviewAssessmentModelReportRequest struct {
	Answers  json.RawMessage `json:"answers,omitempty"`
	SampleID string          `json:"sample_id,omitempty"`
}
