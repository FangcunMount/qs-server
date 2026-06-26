package personalitysession

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
)

type StartSessionRequest struct {
	ModelCode string `json:"model_code" binding:"required"`
	TesteeID  uint64 `json:"testee_id" binding:"required"`
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
	Model            personalitymodel.PersonalityModelSummaryResponse `json:"model"`
	Questionnaire    questionnaire.QuestionnaireResponse            `json:"questionnaire"`
	SubmitContract   SubmitContractResponse                         `json:"submit_contract"`
	Endpoints        SessionEndpointsResponse                       `json:"endpoints"`
}

func buildSubmitContract(model *personalitymodel.PersonalityModelResponse, testeeID uint64) SubmitContractResponse {
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
		WaitReport:              fmt.Sprintf("/api/v1/personality-assessments/{assessment_id}/wait-report?testee_id=%s", testeeIDStr),
		Report:                  fmt.Sprintf("/api/v1/personality-assessments/{assessment_id}/report?testee_id=%s", testeeIDStr),
	}
}
