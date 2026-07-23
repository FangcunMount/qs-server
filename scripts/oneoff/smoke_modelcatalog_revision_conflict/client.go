package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type targetKind string

const (
	targetModel         targetKind = "assessment_model"
	targetQuestionnaire targetKind = "questionnaire"
)

type targetSpec struct {
	Kind targetKind
	Code string
}

func (s targetSpec) path() string {
	switch s.Kind {
	case targetModel:
		return "/api/v1/assessment-models/" + url.PathEscape(s.Code)
	case targetQuestionnaire:
		return "/api/v1/questionnaires/" + url.PathEscape(s.Code)
	default:
		return ""
	}
}

func (s targetSpec) updatePath() string { return s.path() + "/basic-info" }

type releaseState struct {
	WorkingStatus  string `json:"working_status"`
	WorkingVersion string `json:"working_version"`
	OnlineStatus   string `json:"online_status"`
	ActiveVersion  string `json:"active_version"`
}

type modelSnapshot struct {
	Code                 string       `json:"code"`
	Status               string       `json:"status"`
	Title                string       `json:"title"`
	Description          string       `json:"description"`
	SubKind              string       `json:"sub_kind"`
	Algorithm            string       `json:"algorithm"`
	ProductChannel       string       `json:"product_channel"`
	Category             string       `json:"category"`
	Stages               []string     `json:"stages"`
	ApplicableAges       []string     `json:"applicable_ages"`
	Reporters            []string     `json:"reporters"`
	Tags                 []string     `json:"tags"`
	QuestionnaireCode    string       `json:"questionnaire_code"`
	QuestionnaireVersion string       `json:"questionnaire_version"`
	ReleaseState         releaseState `json:"release_state"`
}

type modelBasicInfo struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	SubKind        string   `json:"sub_kind"`
	Algorithm      string   `json:"algorithm"`
	ProductChannel string   `json:"product_channel"`
	Category       string   `json:"category"`
	Stages         []string `json:"stages"`
	ApplicableAges []string `json:"applicable_ages"`
	Reporters      []string `json:"reporters"`
	Tags           []string `json:"tags"`
}

type questionnaireSnapshot struct {
	Code         string       `json:"code"`
	Status       string       `json:"status"`
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	ImgURL       string       `json:"img_url"`
	Type         string       `json:"type"`
	ReleaseState releaseState `json:"release_state"`
}

type questionnaireBasicInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ImgURL      string `json:"img_url"`
	Type        string `json:"type"`
}

type targetSnapshot struct {
	Spec          targetSpec
	Status        string
	ReleaseState  releaseState
	Model         modelSnapshot
	Questionnaire questionnaireSnapshot
}

func (s targetSnapshot) title() string {
	if s.Spec.Kind == targetModel {
		return s.Model.Title
	}
	return s.Questionnaire.Title
}

func (s targetSnapshot) requestWithTitle(title string) any {
	if s.Spec.Kind == targetModel {
		return modelBasicInfo{
			Title: title, Description: s.Model.Description, SubKind: s.Model.SubKind,
			Algorithm: s.Model.Algorithm, ProductChannel: s.Model.ProductChannel,
			Category: s.Model.Category, Stages: cloneStrings(s.Model.Stages),
			ApplicableAges: cloneStrings(s.Model.ApplicableAges), Reporters: cloneStrings(s.Model.Reporters),
			Tags: cloneStrings(s.Model.Tags),
		}
	}
	return questionnaireBasicInfo{
		Title: title, Description: s.Questionnaire.Description,
		ImgURL: s.Questionnaire.ImgURL, Type: s.Questionnaire.Type,
	}
}

func (s targetSnapshot) originalRequest() any { return s.requestWithTitle(s.title()) }

func (s targetSnapshot) basicInfoEquals(other targetSnapshot) bool {
	if s.Spec.Kind != other.Spec.Kind || s.Spec.Code != other.Spec.Code {
		return false
	}
	if s.Spec.Kind == targetQuestionnaire {
		return s.Questionnaire.Title == other.Questionnaire.Title &&
			s.Questionnaire.Description == other.Questionnaire.Description &&
			s.Questionnaire.ImgURL == other.Questionnaire.ImgURL &&
			s.Questionnaire.Type == other.Questionnaire.Type
	}
	return s.Model.Title == other.Model.Title &&
		s.Model.Description == other.Model.Description &&
		s.Model.SubKind == other.Model.SubKind &&
		s.Model.Algorithm == other.Model.Algorithm &&
		s.Model.ProductChannel == other.Model.ProductChannel &&
		s.Model.Category == other.Model.Category &&
		equalStrings(s.Model.Stages, other.Model.Stages) &&
		equalStrings(s.Model.ApplicableAges, other.Model.ApplicableAges) &&
		equalStrings(s.Model.Reporters, other.Model.Reporters) &&
		equalStrings(s.Model.Tags, other.Model.Tags)
}

type responseEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type httpResult struct {
	StatusCode int
	Code       int
	Message    string
	Data       json.RawMessage
	Err        error
}

type restClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func newRESTClient(baseURL, token string) *restClient {
	return &restClient{
		baseURL: strings.TrimRight(baseURL, "/"), token: token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *restClient) checkReady(ctx context.Context) error {
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

func (c *restClient) getSnapshot(ctx context.Context, spec targetSpec) (targetSnapshot, error) {
	result := c.doJSON(ctx, http.MethodGet, spec.path(), nil)
	if result.Err != nil {
		return targetSnapshot{}, result.Err
	}
	if result.StatusCode != http.StatusOK || result.Code != 0 {
		return targetSnapshot{}, fmt.Errorf("GET %s returned HTTP %d code=%d: %s",
			spec.path(), result.StatusCode, result.Code, truncate(result.Message, 500))
	}
	snapshot := targetSnapshot{Spec: spec}
	switch spec.Kind {
	case targetModel:
		if err := json.Unmarshal(result.Data, &snapshot.Model); err != nil {
			return targetSnapshot{}, fmt.Errorf("decode model response: %w", err)
		}
		snapshot.Status = snapshot.Model.Status
		snapshot.ReleaseState = snapshot.Model.ReleaseState
		if snapshot.Model.Code != spec.Code {
			return targetSnapshot{}, fmt.Errorf("model response code=%q, want %q", snapshot.Model.Code, spec.Code)
		}
	case targetQuestionnaire:
		if err := json.Unmarshal(result.Data, &snapshot.Questionnaire); err != nil {
			return targetSnapshot{}, fmt.Errorf("decode questionnaire response: %w", err)
		}
		snapshot.Status = snapshot.Questionnaire.Status
		snapshot.ReleaseState = snapshot.Questionnaire.ReleaseState
		if snapshot.Questionnaire.Code != spec.Code {
			return targetSnapshot{}, fmt.Errorf("questionnaire response code=%q, want %q", snapshot.Questionnaire.Code, spec.Code)
		}
	default:
		return targetSnapshot{}, fmt.Errorf("unsupported target kind %q", spec.Kind)
	}
	return snapshot, nil
}

func (c *restClient) update(ctx context.Context, snapshot targetSnapshot, input any) httpResult {
	return c.doJSON(ctx, http.MethodPut, snapshot.Spec.updatePath(), input)
}

func (c *restClient) doJSON(ctx context.Context, method, path string, input any) httpResult {
	var body io.Reader
	if input != nil {
		contents, err := json.Marshal(input)
		if err != nil {
			return httpResult{Err: err}
		}
		body = bytes.NewReader(contents)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return httpResult{Err: err}
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "qs-server-modelcatalog-revision-conflict-smoke/1.0")
	request.Header.Set("X-Request-ID", newRequestID())
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return httpResult{Err: err}
	}
	defer func() { _ = response.Body.Close() }()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return httpResult{StatusCode: response.StatusCode, Err: err}
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(contents, &envelope); err != nil {
		return httpResult{StatusCode: response.StatusCode, Err: fmt.Errorf("invalid JSON response: %w", err)}
	}
	return httpResult{
		StatusCode: response.StatusCode, Code: envelope.Code,
		Message: strings.TrimSpace(envelope.Message), Data: envelope.Data,
	}
}

func newRequestID() string {
	return fmt.Sprintf("modelcatalog-conflict-smoke-%d-%s", time.Now().UnixNano(), randomHex(4))
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
