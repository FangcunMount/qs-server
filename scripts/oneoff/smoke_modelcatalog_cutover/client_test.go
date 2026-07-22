package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSmokeClientRunsSupportedFamiliesEndToEnd(t *testing.T) {
	tests := []struct {
		name          string
		model         publishedModel
		reportPrefix  string
		wantSession   bool
		withNorm      bool
		wantNormCount int
		questionnaire questionnaire
	}{
		{
			name:          "scale",
			model:         publishedModel{modelIdentity: modelIdentity{Kind: "scale", Algorithm: "scale_default", Code: "ISI7", Version: "1.0.0"}, QuestionnaireCode: "ISI7", QuestionnaireVersion: "1.0.0"},
			reportPrefix:  "/api/v1/assessments/",
			questionnaire: smokeQuestionnaire("ISI7", "1.0.0"),
		},
		{
			name:          "typology",
			model:         publishedModel{modelIdentity: modelIdentity{Kind: "typology", SubKind: "typology", Algorithm: "personality_typology", Code: "MBTI_OEJTS", Version: "v64"}, QuestionnaireCode: "MBTI_OEJTS", QuestionnaireVersion: "8.0.1"},
			reportPrefix:  "/api/v1/typology-assessments/",
			wantSession:   true,
			questionnaire: smokeQuestionnaire("MBTI_OEJTS", "8.0.1"),
		},
		{
			name:          "behavioral",
			model:         publishedModel{modelIdentity: modelIdentity{Kind: "behavioral_rating", Algorithm: "brief2", Code: "gXkk9W", Version: "v22"}, QuestionnaireCode: "gXkk9W", QuestionnaireVersion: "7.0.1"},
			reportPrefix:  "/api/v1/behavior-assessments/",
			withNorm:      true,
			wantNormCount: 1,
			questionnaire: smokeQuestionnaire("gXkk9W", "7.0.1"),
		},
		{
			name:          "cognitive",
			model:         publishedModel{modelIdentity: modelIdentity{Kind: "cognitive", Algorithm: "spm", Code: "SPM", Version: "1.0.0"}, QuestionnaireCode: "SPM", QuestionnaireVersion: "1.0.0"},
			reportPrefix:  "/api/v1/behavior-assessments/",
			withNorm:      true,
			wantNormCount: 1,
			questionnaire: smokeQuestionnaire("SPM", "1.0.0"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, calls := newSmokeServer(t, test.model, test.questionnaire, test.reportPrefix, test.wantSession, test.withNorm)
			defer server.Close()
			client := newSmokeClient(server.URL, "token", time.Millisecond)
			result := client.runCase(context.Background(), "1001", test.model.Code, "smoke")
			if !result.Passed {
				t.Fatalf("runCase() failed at %s: %s", result.FailedStep, result.Error)
			}
			if result.AnswerSheetID != "2001" || result.AssessmentID != "3001" || result.Level.Code != "low" {
				t.Fatalf("result = %#v", result)
			}
			if result.NormReferenceCount != test.wantNormCount {
				t.Fatalf("norm refs = %d, want %d", result.NormReferenceCount, test.wantNormCount)
			}
			calls.assertSeen(t, test.reportPrefix+"3001/report-status", test.reportPrefix+"3001/report")
			if test.wantSession {
				calls.assertSeen(t, "/api/v1/typology-assessment-sessions")
			} else {
				calls.assertSeen(t, "/api/v1/questionnaires/"+test.questionnaire.Code)
			}
		})
	}
}

func TestSmokeClientFailsClosedWhenNormReferenceIsMissing(t *testing.T) {
	model := publishedModel{modelIdentity: modelIdentity{Kind: "behavioral_rating", Algorithm: "brief2", Code: "gXkk9W", Version: "v22"}, QuestionnaireCode: "gXkk9W", QuestionnaireVersion: "7.0.1"}
	server, _ := newSmokeServer(t, model, smokeQuestionnaire("gXkk9W", "7.0.1"), "/api/v1/behavior-assessments/", false, false)
	defer server.Close()
	client := newSmokeClient(server.URL, "token", time.Millisecond)
	result := client.runCase(context.Background(), "1001", model.Code, "smoke")
	if result.Passed || result.FailedStep != "report" || !strings.Contains(result.Error, "norm_reference") {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildAnswersCoversSupportedQuestionTypes(t *testing.T) {
	questions := []question{
		{Code: "r", Type: "Radio", Options: []questionOption{{Code: "A"}, {Code: "B"}}},
		{Code: "c", Type: "Checkbox", Options: []questionOption{{Code: "A"}, {Code: "B"}}, ValidationRules: []validationRule{{RuleType: "min_selections", TargetValue: "2"}}},
		{Code: "t", Type: "Text", ValidationRules: []validationRule{{RuleType: "min_length", TargetValue: "4"}}},
		{Code: "n", Type: "Number", ValidationRules: []validationRule{{RuleType: "min_value", TargetValue: "3"}, {RuleType: "max_value", TargetValue: "5"}}},
		{Code: "s", Type: "Section"},
	}
	answers, err := buildAnswers(questions)
	if err != nil {
		t.Fatalf("buildAnswers() error = %v", err)
	}
	if len(answers) != 4 {
		t.Fatalf("answers = %#v", answers)
	}
	if answers[1].Value != `["B","A"]` {
		t.Fatalf("checkbox value = %s", answers[1].Value)
	}
}

func TestReadFirstTokenSupportsPerfTokenFile(t *testing.T) {
	path := t.TempDir() + "/tokens.json"
	if err := os.WriteFile(path, []byte(`["token-a","token-b"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := readFirstToken(path)
	if err != nil || token != "token-a" {
		t.Fatalf("readFirstToken() = %q, %v", token, err)
	}
}

type callRecorder struct {
	mu    sync.Mutex
	paths []string
}

func (r *callRecorder) add(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
}

func (r *callRecorder) assertSeen(t *testing.T, paths ...string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, path := range paths {
		found := false
		for _, seen := range r.paths {
			if seen == path {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("path %s not seen; calls=%v", path, r.paths)
		}
	}
}

func newSmokeServer(t *testing.T, model publishedModel, q questionnaire, reportPrefix string, typology, withNorm bool) (*httptest.Server, *callRecorder) {
	t.Helper()
	calls := &callRecorder{}
	readinessCalls := 0
	reportCalls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.add(r.URL.Path)
		if r.URL.Path != "/readyz" && r.Header.Get("Authorization") != "Bearer token" {
			writeEnvelope(w, http.StatusUnauthorized, nil)
			return
		}
		switch {
		case r.URL.Path == "/readyz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/assessment-models/"+model.Code:
			writeEnvelope(w, http.StatusOK, model)
		case r.URL.Path == "/api/v1/typology-assessment-sessions" && typology:
			var session typologySession
			session.Model = model
			session.Questionnaire = q
			session.Submit.QuestionnaireCode = q.Code
			session.Submit.QuestionnaireVersion = q.Version
			session.Submit.TesteeID = "1001"
			writeEnvelope(w, http.StatusOK, session)
		case r.URL.Path == "/api/v1/questionnaires/"+q.Code && !typology:
			if r.URL.Query().Get("version") != q.Version {
				t.Errorf("questionnaire version = %q", r.URL.Query().Get("version"))
			}
			writeEnvelope(w, http.StatusOK, q)
		case r.URL.Path == "/api/v1/answersheets":
			var request submitRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode submit: %v", err)
			}
			if request.TesteeID != "1001" || request.QuestionnaireCode != q.Code || len(request.Answers) != 1 {
				t.Errorf("submit request = %#v", request)
			}
			writeEnvelope(w, http.StatusAccepted, submitResponse{Status: "accepted", RequestID: "req", AnswerSheetID: "2001"})
		case r.URL.Path == "/api/v1/answersheets/2001/assessment-readiness":
			readinessCalls++
			if readinessCalls == 1 {
				writeEnvelope(w, http.StatusOK, readinessResponse{Status: "pending", AnswerSheetID: "2001", NextPollAfterMs: 1})
				return
			}
			writeEnvelope(w, http.StatusOK, readinessResponse{Status: "ready", AnswerSheetID: "2001", AssessmentID: "3001"})
		case r.URL.Path == reportPrefix+"3001/report-status":
			reportCalls++
			if reportCalls == 1 {
				writeEnvelope(w, http.StatusOK, reportStatus{Status: "processing", NextPollAfterMs: 1})
				return
			}
			writeEnvelope(w, http.StatusOK, reportStatus{Status: "interpreted", Model: &model.modelIdentity, Level: &levelResult{Code: "low", Label: "较低"}})
		case r.URL.Path == reportPrefix+"3001/report":
			report := reportResponse{AssessmentID: "3001", Model: model.modelIdentity, Level: &levelResult{Code: "low", Label: "较低"}, Conclusion: "smoke conclusion"}
			if withNorm {
				report.Dimensions = append(report.Dimensions, struct {
					NormReference *struct {
						TableVersion string `json:"table_version"`
						FormVariant  string `json:"form_variant"`
					} `json:"norm_reference"`
				}{NormReference: &struct {
					TableVersion string `json:"table_version"`
					FormVariant  string `json:"form_variant"`
				}{TableVersion: "norm-v1", FormVariant: "standard"}})
			}
			writeEnvelope(w, http.StatusOK, report)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			writeEnvelope(w, http.StatusNotFound, nil)
		}
	})
	return httptest.NewServer(handler), calls
}

func smokeQuestionnaire(code, version string) questionnaire {
	return questionnaire{Code: code, Version: version, Questions: []question{{Code: "Q1", Type: "Radio", Options: []questionOption{{Code: "A"}, {Code: "B"}}}}}
}

func writeEnvelope(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": map[bool]int{true: 0, false: status}[status < 400], "message": http.StatusText(status), "data": data})
}
