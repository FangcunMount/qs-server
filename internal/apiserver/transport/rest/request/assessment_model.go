package request

import "encoding/json"

type CreateAssessmentModelRequest struct {
	Code                 string   `json:"code"`
	Kind                 string   `json:"kind" valid:"required~模型类型不能为空"`
	Title                string   `json:"title" valid:"required~模型标题不能为空"`
	Description          string   `json:"description"`
	Category             string   `json:"category"`
	Tags                 []string `json:"tags"`
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
}

type UpdateAssessmentModelBasicInfoRequest struct {
	Title       string   `json:"title" valid:"required~模型标题不能为空"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
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
