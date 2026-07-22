package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type smokeClient struct {
	baseURL      string
	token        string
	pollInterval time.Duration
	httpClient   *http.Client
}

type responseEnvelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Reference string          `json:"reference,omitempty"`
	Data      json.RawMessage `json:"data"`
}

type modelIdentity struct {
	Kind            string `json:"kind"`
	SubKind         string `json:"sub_kind,omitempty"`
	Algorithm       string `json:"algorithm,omitempty"`
	Code            string `json:"code"`
	Version         string `json:"version,omitempty"`
	ProductChannel  string `json:"product_channel,omitempty"`
	AlgorithmFamily string `json:"algorithm_family,omitempty"`
	DecisionKind    string `json:"decision_kind,omitempty"`
}

type publishedModel struct {
	modelIdentity
	Title                string `json:"title"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
}

type questionnaire struct {
	Code      string     `json:"code"`
	Version   string     `json:"version"`
	Title     string     `json:"title"`
	Questions []question `json:"questions"`
}

type question struct {
	Code            string           `json:"code"`
	Type            string           `json:"type"`
	Options         []questionOption `json:"options"`
	ValidationRules []validationRule `json:"validation_rules"`
}

type questionOption struct {
	Code    string `json:"code"`
	Content string `json:"content"`
}

type validationRule struct {
	RuleType    string `json:"rule_type"`
	TargetValue string `json:"target_value"`
}

type typologySession struct {
	Model         publishedModel `json:"model"`
	Questionnaire questionnaire  `json:"questionnaire"`
	Submit        struct {
		QuestionnaireCode    string `json:"questionnaire_code"`
		QuestionnaireVersion string `json:"questionnaire_version"`
		TesteeID             string `json:"testee_id"`
	} `json:"submit_contract"`
}

type answer struct {
	QuestionCode string `json:"question_code"`
	QuestionType string `json:"question_type"`
	Score        uint32 `json:"score"`
	Value        string `json:"value"`
}

type submitRequest struct {
	QuestionnaireCode    string   `json:"questionnaire_code"`
	QuestionnaireVersion string   `json:"questionnaire_version"`
	IdempotencyKey       string   `json:"idempotency_key"`
	Title                string   `json:"title"`
	TesteeID             string   `json:"testee_id"`
	Answers              []answer `json:"answers"`
}

type submitResponse struct {
	Status        string `json:"status"`
	RequestID     string `json:"request_id"`
	AnswerSheetID string `json:"answersheet_id"`
}

type readinessResponse struct {
	Status          string `json:"status"`
	AnswerSheetID   string `json:"answersheet_id"`
	AssessmentID    string `json:"assessment_id"`
	NextPollAfterMs int    `json:"next_poll_after_ms"`
}

type levelResult struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type reportStatus struct {
	Status          string         `json:"status"`
	Stage           string         `json:"stage"`
	Message         string         `json:"message"`
	Reason          string         `json:"reason"`
	NextPollAfterMs int            `json:"next_poll_after_ms"`
	Model           *modelIdentity `json:"model"`
	Level           *levelResult   `json:"level"`
}

type reportResponse struct {
	AssessmentID string        `json:"assessment_id"`
	Model        modelIdentity `json:"model"`
	Level        *levelResult  `json:"level"`
	Conclusion   string        `json:"conclusion"`
	Dimensions   []struct {
		NormReference *struct {
			TableVersion string `json:"table_version"`
			FormVariant  string `json:"form_variant"`
		} `json:"norm_reference"`
	} `json:"dimensions"`
}

type smokeRunResult struct {
	StartedAt         time.Time         `json:"started_at"`
	FinishedAt        time.Time         `json:"finished_at"`
	CollectionBaseURL string            `json:"collection_base_url"`
	TesteeID          string            `json:"testee_id"`
	Passed            bool              `json:"passed"`
	Results           []smokeCaseResult `json:"results"`
}

type smokeCaseResult struct {
	Model              modelIdentity `json:"model"`
	QuestionnaireCode  string        `json:"questionnaire_code,omitempty"`
	QuestionnaireVer   string        `json:"questionnaire_version,omitempty"`
	AnswerCount        int           `json:"answer_count,omitempty"`
	RequestID          string        `json:"request_id,omitempty"`
	AnswerSheetID      string        `json:"answersheet_id,omitempty"`
	AssessmentID       string        `json:"assessment_id,omitempty"`
	ReportStatus       string        `json:"report_status,omitempty"`
	Level              levelResult   `json:"level,omitempty"`
	NormReferenceCount int           `json:"norm_reference_count"`
	Passed             bool          `json:"passed"`
	FailedStep         string        `json:"failed_step,omitempty"`
	Error              string        `json:"error,omitempty"`
	Duration           string        `json:"duration"`
}

func newSmokeClient(baseURL, token string, pollInterval time.Duration) *smokeClient {
	return &smokeClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		token:        token,
		pollInterval: pollInterval,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *smokeClient) checkReady(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/readyz", nil)
	if err != nil {
		return err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("GET /readyz returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *smokeClient) runCase(ctx context.Context, testeeID, modelCode, title string) (result smokeCaseResult) {
	startedAt := time.Now()
	result.Model.Code = modelCode
	step := "catalog"
	defer func() { result.Duration = time.Since(startedAt).Round(time.Millisecond).String() }()

	model, err := c.getPublishedModel(ctx, modelCode)
	if err != nil {
		return failCase(result, step, err)
	}
	result.Model = model.modelIdentity

	step = "questionnaire"
	q, err := c.resolveQuestionnaire(ctx, testeeID, model)
	if err != nil {
		return failCase(result, step, err)
	}
	result.QuestionnaireCode = q.Code
	result.QuestionnaireVer = q.Version

	step = "answers"
	answers, err := buildAnswers(q.Questions)
	if err != nil {
		return failCase(result, step, err)
	}
	result.AnswerCount = len(answers)

	step = "submit"
	submitted, err := c.submit(ctx, submitRequest{
		QuestionnaireCode:    q.Code,
		QuestionnaireVersion: q.Version,
		IdempotencyKey:       newIdempotencyKey(model.Code),
		Title:                title,
		TesteeID:             testeeID,
		Answers:              answers,
	})
	if err != nil {
		return failCase(result, step, err)
	}
	result.RequestID = submitted.RequestID
	result.AnswerSheetID = submitted.AnswerSheetID

	step = "assessment_readiness"
	assessmentID, err := c.waitForAssessment(ctx, testeeID, submitted.AnswerSheetID)
	if err != nil {
		return failCase(result, step, err)
	}
	result.AssessmentID = assessmentID

	step = "report_status"
	status, err := c.waitForReport(ctx, testeeID, assessmentID, model.Kind)
	if err != nil {
		return failCase(result, step, err)
	}
	result.ReportStatus = status.Status

	step = "report"
	report, err := c.getReport(ctx, testeeID, assessmentID, model.Kind)
	if err != nil {
		return failCase(result, step, err)
	}
	if err := validateReport(model, assessmentID, report); err != nil {
		return failCase(result, step, err)
	}
	result.Level = *report.Level
	result.NormReferenceCount = countNormReferences(report)
	result.Passed = true
	return result
}

func (c *smokeClient) getPublishedModel(ctx context.Context, code string) (publishedModel, error) {
	var model publishedModel
	err := c.doJSON(ctx, http.MethodGet, "/api/v1/assessment-models/"+url.PathEscape(code), nil, &model, http.StatusOK)
	if err != nil {
		return publishedModel{}, err
	}
	if model.Code == "" || model.Kind == "" || model.Version == "" {
		return publishedModel{}, errors.New("published model identity is incomplete")
	}
	if model.QuestionnaireCode == "" || model.QuestionnaireVersion == "" {
		return publishedModel{}, errors.New("published model questionnaire binding is incomplete")
	}
	return model, nil
}

func (c *smokeClient) resolveQuestionnaire(ctx context.Context, testeeID string, model publishedModel) (questionnaire, error) {
	if model.Kind == "typology" {
		var session typologySession
		err := c.doJSON(ctx, http.MethodPost, "/api/v1/typology-assessment-sessions", map[string]string{
			"model_code": model.Code,
			"testee_id":  testeeID,
		}, &session, http.StatusOK)
		if err != nil {
			return questionnaire{}, err
		}
		if err := compareIdentity(model.modelIdentity, session.Model.modelIdentity); err != nil {
			return questionnaire{}, fmt.Errorf("session model identity: %w", err)
		}
		if session.Submit.QuestionnaireCode != session.Questionnaire.Code || session.Submit.QuestionnaireVersion != session.Questionnaire.Version {
			return questionnaire{}, errors.New("typology session submit contract does not match frozen questionnaire")
		}
		return validateQuestionnaire(session.Questionnaire, model)
	}

	path := "/api/v1/questionnaires/" + url.PathEscape(model.QuestionnaireCode) + "?version=" + url.QueryEscape(model.QuestionnaireVersion)
	var q questionnaire
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &q, http.StatusOK); err != nil {
		return questionnaire{}, err
	}
	return validateQuestionnaire(q, model)
}

func validateQuestionnaire(q questionnaire, model publishedModel) (questionnaire, error) {
	if q.Code != model.QuestionnaireCode || q.Version != model.QuestionnaireVersion {
		return questionnaire{}, fmt.Errorf("questionnaire identity mismatch: got %s@%s, want %s@%s", q.Code, q.Version, model.QuestionnaireCode, model.QuestionnaireVersion)
	}
	if len(q.Questions) == 0 {
		return questionnaire{}, errors.New("questionnaire has no questions")
	}
	return q, nil
}

func (c *smokeClient) submit(ctx context.Context, request submitRequest) (submitResponse, error) {
	var response submitResponse
	err := c.doJSON(ctx, http.MethodPost, "/api/v1/answersheets", request, &response, http.StatusAccepted)
	if err != nil {
		return submitResponse{}, err
	}
	if response.Status != "accepted" || response.AnswerSheetID == "" {
		return submitResponse{}, fmt.Errorf("unexpected submit response: status=%q answersheet_id=%q", response.Status, response.AnswerSheetID)
	}
	return response, nil
}

func (c *smokeClient) waitForAssessment(ctx context.Context, testeeID, answerSheetID string) (string, error) {
	path := fmt.Sprintf("/api/v1/answersheets/%s/assessment-readiness?testee_id=%s", url.PathEscape(answerSheetID), url.QueryEscape(testeeID))
	for {
		var readiness readinessResponse
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &readiness, http.StatusOK); err != nil {
			return "", err
		}
		switch readiness.Status {
		case "ready":
			if readiness.AssessmentID == "" {
				return "", errors.New("assessment readiness returned ready without assessment_id")
			}
			return readiness.AssessmentID, nil
		case "pending":
			if err := sleepContext(ctx, c.pollDelay(readiness.NextPollAfterMs)); err != nil {
				return "", fmt.Errorf("wait for assessment: %w", err)
			}
		default:
			return "", fmt.Errorf("unexpected assessment readiness status %q", readiness.Status)
		}
	}
}

func (c *smokeClient) waitForReport(ctx context.Context, testeeID, assessmentID, kind string) (reportStatus, error) {
	path := reportBasePath(kind, assessmentID) + "/report-status?testee_id=" + url.QueryEscape(testeeID)
	for {
		var status reportStatus
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &status, http.StatusOK); err != nil {
			return reportStatus{}, err
		}
		switch status.Status {
		case "interpreted":
			return status, nil
		case "failed":
			return reportStatus{}, fmt.Errorf("report failed: stage=%s reason=%s message=%s", status.Stage, status.Reason, status.Message)
		case "processing", "pending", "evaluating", "":
			if err := sleepContext(ctx, c.pollDelay(status.NextPollAfterMs)); err != nil {
				return reportStatus{}, fmt.Errorf("wait for report: %w", err)
			}
		default:
			return reportStatus{}, fmt.Errorf("unexpected report status %q", status.Status)
		}
	}
}

func (c *smokeClient) getReport(ctx context.Context, testeeID, assessmentID, kind string) (reportResponse, error) {
	path := reportBasePath(kind, assessmentID) + "/report?testee_id=" + url.QueryEscape(testeeID)
	var report reportResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &report, http.StatusOK); err != nil {
		return reportResponse{}, err
	}
	return report, nil
}

func reportBasePath(kind, assessmentID string) string {
	prefix := "/api/v1/assessments/"
	switch kind {
	case "typology":
		prefix = "/api/v1/typology-assessments/"
	case "behavioral_rating", "cognitive":
		prefix = "/api/v1/behavior-assessments/"
	}
	return prefix + url.PathEscape(assessmentID)
}

func validateReport(model publishedModel, assessmentID string, report reportResponse) error {
	if report.AssessmentID != assessmentID {
		return fmt.Errorf("report assessment_id mismatch: got %q, want %q", report.AssessmentID, assessmentID)
	}
	if err := compareIdentity(model.modelIdentity, report.Model); err != nil {
		return fmt.Errorf("report model identity: %w", err)
	}
	if report.Level == nil || strings.TrimSpace(report.Level.Code) == "" || strings.TrimSpace(report.Level.Label) == "" {
		return errors.New("report must contain separate non-empty level code and label")
	}
	if utf8.RuneCountInString(report.Level.Code) > 64 {
		return fmt.Errorf("report level code is not a short outcome code: length=%d", utf8.RuneCountInString(report.Level.Code))
	}
	if strings.TrimSpace(report.Conclusion) == "" {
		return errors.New("report conclusion is empty")
	}
	if (model.Kind == "behavioral_rating" || model.Kind == "cognitive") && countNormReferences(report) == 0 {
		return errors.New("normative report contains no concrete norm_reference.table_version")
	}
	return nil
}

func compareIdentity(want, got modelIdentity) error {
	if want.Kind != got.Kind || want.SubKind != got.SubKind || want.Algorithm != got.Algorithm || want.Code != got.Code || want.Version != got.Version {
		return fmt.Errorf("got %s/%s/%s/%s@%s, want %s/%s/%s/%s@%s",
			got.Kind, got.SubKind, got.Algorithm, got.Code, got.Version,
			want.Kind, want.SubKind, want.Algorithm, want.Code, want.Version)
	}
	return nil
}

func countNormReferences(report reportResponse) int {
	count := 0
	for _, dimension := range report.Dimensions {
		if dimension.NormReference != nil && strings.TrimSpace(dimension.NormReference.TableVersion) != "" {
			count++
		}
	}
	return count
}

func (c *smokeClient) pollDelay(serverMilliseconds int) time.Duration {
	delay := c.pollInterval
	if serverDelay := time.Duration(serverMilliseconds) * time.Millisecond; serverDelay > delay {
		delay = serverDelay
	}
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

func (c *smokeClient) doJSON(ctx context.Context, method, path string, input, output any, expectedStatus int) error {
	var body io.Reader
	if input != nil {
		contents, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(contents)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("User-Agent", "qs-server-modelcatalog-cutover-smoke/1.0")
	request.Header.Set("X-Request-ID", newRequestID())

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(contents, &envelope); err != nil {
		return fmt.Errorf("HTTP %d returned invalid JSON: %w", response.StatusCode, err)
	}
	if response.StatusCode != expectedStatus || envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = strings.TrimSpace(string(contents))
		}
		return fmt.Errorf("%s %s returned HTTP %d code=%d: %s", method, path, response.StatusCode, envelope.Code, truncate(message, 500))
	}
	if output == nil {
		return nil
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("%s %s returned no data", method, path)
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("decode %s %s data: %w", method, path, err)
	}
	return nil
}

func newIdempotencyKey(modelCode string) string {
	return fmt.Sprintf("modelcatalog-smoke-%s-%d-%s", sanitize(modelCode), time.Now().UnixMilli(), randomHex(4))
}

func newRequestID() string {
	return fmt.Sprintf("modelcatalog-smoke-%d-%s", time.Now().UnixNano(), randomHex(4))
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func sanitize(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func failCase(result smokeCaseResult, step string, err error) smokeCaseResult {
	result.Passed = false
	result.FailedStep = step
	result.Error = err.Error()
	return result
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
