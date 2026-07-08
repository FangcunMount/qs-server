package personalitysession

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

type StartSessionRequest struct {
	ModelCode string `json:"model_code" binding:"required"`
	TesteeID  uint64 `json:"testee_id" binding:"required"`
}

// UnmarshalJSON 支持 testee_id 为字符串或数字（小程序侧大整数常以字符串传输）。
func (r *StartSessionRequest) UnmarshalJSON(data []byte) error {
	type Alias StartSessionRequest
	aux := &struct {
		TesteeID json.RawMessage `json:"testee_id"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.TesteeID) == 0 {
		return fmt.Errorf("testee_id must be a string or number")
	}
	if aux.TesteeID[0] == '"' {
		var text string
		if err := json.Unmarshal(aux.TesteeID, &text); err != nil {
			return fmt.Errorf("invalid testee_id format: %w", err)
		}
		testeeID, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid testee_id format: %w", err)
		}
		r.TesteeID = testeeID
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(aux.TesteeID, &number); err != nil {
		return fmt.Errorf("testee_id must be a string or number")
	}
	testeeID, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid testee_id format: %w", err)
	}
	r.TesteeID = testeeID
	return nil
}

type SubmitContractResponse struct {
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	TesteeID             string `json:"testee_id"`
}

type SessionEndpointsResponse struct {
	SubmitAnswerSheet       string `json:"submit_answer_sheet"`
	AssessmentByAnswerSheet string `json:"assessment_by_answer_sheet"`
	WaitReport              string `json:"wait_report"`
	Report                  string `json:"report"`
}

type StartSessionResponse struct {
	Model          typologymodel.TypologyModelSummaryResponse `json:"model"`
	Questionnaire  questionnaire.QuestionnaireResponse        `json:"questionnaire"`
	SubmitContract SubmitContractResponse                     `json:"submit_contract"`
	Endpoints      SessionEndpointsResponse                   `json:"endpoints"`
}

func buildSubmitContract(model *typologymodel.TypologyModelResponse, testeeID uint64) SubmitContractResponse {
	return SubmitContractResponse{
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		TesteeID:             strconv.FormatUint(testeeID, 10),
	}
}

func buildEndpoints(testeeID uint64) SessionEndpointsResponse {
	testeeIDStr := strconv.FormatUint(testeeID, 10)
	return SessionEndpointsResponse{
		SubmitAnswerSheet:       "/api/v1/answersheets",
		AssessmentByAnswerSheet: "/api/v1/answersheets/{answersheet_id}/assessment",
		WaitReport:              fmt.Sprintf("/api/v1/typology-assessments/{assessment_id}/wait-report?testee_id=%s", testeeIDStr),
		Report:                  fmt.Sprintf("/api/v1/typology-assessments/{assessment_id}/report?testee_id=%s", testeeIDStr),
	}
}
