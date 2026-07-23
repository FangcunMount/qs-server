package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRunObservesRevisionConflictAndRestoresBothDrafts(t *testing.T) {
	server := newConflictServer(t, 4, true)
	defer server.Close()
	output := filepath.Join(t.TempDir(), "evidence.json")
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"--api-base-url", server.URL,
		"--token", "token",
		"--model-code", "SMOKE_MODEL",
		"--questionnaire-code", "SMOKE_QUESTIONNAIRE",
		"--confirm-targets", "SMOKE_MODEL,SMOKE_QUESTIONNAIRE",
		"--concurrency", "4",
		"--rounds", "1",
		"--output", output,
		"--apply",
	}, &stdout, &stderr)
	if exitCode != exitOK {
		t.Fatalf("run()=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "MODELCATALOG_REVISION_CONFLICT_SMOKE_OK") {
		t.Fatalf("stdout=%s", stdout.String())
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var evidence runEvidence
	if err := json.Unmarshal(contents, &evidence); err != nil {
		t.Fatal(err)
	}
	if !evidence.Passed || len(evidence.Targets) != 2 {
		t.Fatalf("evidence=%#v", evidence)
	}
	for _, target := range evidence.Targets {
		if target.Successes != 1 || target.RevisionConflicts != 3 || !target.RestorePassed {
			t.Fatalf("target=%#v", target)
		}
	}
	server.assertRestored(t)
}

func TestRunFailsWhenConflictIsNotObservedButStillRestores(t *testing.T) {
	server := newConflictServer(t, 4, false)
	defer server.Close()
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"--api-base-url", server.URL,
		"--token", "token",
		"--model-code", "SMOKE_MODEL",
		"--questionnaire-code", "SMOKE_QUESTIONNAIRE",
		"--confirm-targets", "SMOKE_MODEL,SMOKE_QUESTIONNAIRE",
		"--concurrency", "4",
		"--rounds", "1",
		"--apply",
	}, &stdout, &stderr)
	if exitCode != exitFailed || !strings.Contains(stdout.String(), "SMOKE FAIL") {
		t.Fatalf("run()=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
	server.assertRestored(t)
}

func TestRunApplyRequiresExactTargetConfirmation(t *testing.T) {
	server := newConflictServer(t, 2, true)
	defer server.Close()
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"--api-base-url", server.URL,
		"--token", "token",
		"--model-code", "SMOKE_MODEL",
		"--questionnaire-code", "SMOKE_QUESTIONNAIRE",
		"--confirm-targets", "wrong-targets",
		"--apply",
	}, &stdout, &stderr)
	if exitCode != exitUnavailable || !strings.Contains(stderr.String(), "must exactly equal") {
		t.Fatalf("run()=%d stdout=%s stderr=%s", exitCode, stdout.String(), stderr.String())
	}
}

func TestValidateDedicatedDraftRejectsPublishedTarget(t *testing.T) {
	snapshot := targetSnapshot{
		Spec:         targetSpec{Kind: targetModel, Code: "SMOKE_MODEL"},
		Status:       "draft",
		ReleaseState: releaseState{WorkingStatus: "draft", OnlineStatus: "online", ActiveVersion: "v1"},
		Model:        modelSnapshot{Code: "SMOKE_MODEL", Title: "smoke model"},
	}
	if err := validateDedicatedDraft(snapshot); err == nil || !strings.Contains(err.Error(), "active_version") {
		t.Fatalf("validateDedicatedDraft() error=%v", err)
	}
}

func TestReadFirstTokenSupportsPerfTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	if err := os.WriteFile(path, []byte(`["token-a","token-b"]`), 0o600); err != nil {
		t.Fatal(err)
	}
	token, err := readFirstToken(path)
	if err != nil || token != "token-a" {
		t.Fatalf("readFirstToken()=%q,%v", token, err)
	}
}

type conflictServer struct {
	*httptest.Server
	state *conflictServerState
}

type conflictServerState struct {
	mu            sync.Mutex
	concurrency   int
	conflict      bool
	model         modelSnapshot
	questionnaire questionnaireSnapshot
	barriers      map[string]*requestBarrier
}

type requestBarrier struct {
	arrived int
	release chan struct{}
}

func newConflictServer(t *testing.T, concurrency int, conflict bool) *conflictServer {
	t.Helper()
	state := &conflictServerState{
		concurrency: concurrency,
		conflict:    conflict,
		barriers:    make(map[string]*requestBarrier),
		model: modelSnapshot{
			Code: "SMOKE_MODEL", Status: "draft", Title: "original model", Description: "model description",
			SubKind: "scale", Algorithm: "scale_default", ProductChannel: "medical_scale", Category: "smoke",
			Stages: []string{"child"}, ApplicableAges: []string{"6-12"}, Reporters: []string{"self"}, Tags: []string{"smoke"},
			QuestionnaireCode: "SMOKE_QUESTIONNAIRE", QuestionnaireVersion: "1.0.0",
			ReleaseState: releaseState{WorkingStatus: "draft", WorkingVersion: "v1", OnlineStatus: "offline"},
		},
		questionnaire: questionnaireSnapshot{
			Code: "SMOKE_QUESTIONNAIRE", Status: "draft", Title: "original questionnaire",
			Description: "questionnaire description", ImgURL: "https://example.invalid/image.png", Type: "Survey",
			ReleaseState: releaseState{WorkingStatus: "draft", WorkingVersion: "1.0.0", OnlineStatus: "offline"},
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			writeTestEnvelope(w, http.StatusUnauthorized, "unauthorized", nil)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/assessment-models/SMOKE_MODEL":
			state.mu.Lock()
			data := state.model
			state.mu.Unlock()
			writeTestEnvelope(w, http.StatusOK, "ok", data)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/assessment-models/SMOKE_MODEL/basic-info":
			var input modelBasicInfo
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Errorf("decode model: %v", err)
			}
			state.updateModel(w, input)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/questionnaires/SMOKE_QUESTIONNAIRE":
			state.mu.Lock()
			data := state.questionnaire
			state.mu.Unlock()
			writeTestEnvelope(w, http.StatusOK, "ok", data)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/questionnaires/SMOKE_QUESTIONNAIRE/basic-info":
			var input questionnaireBasicInfo
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Errorf("decode questionnaire: %v", err)
			}
			state.updateQuestionnaire(w, input)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			writeTestEnvelope(w, http.StatusNotFound, "not found", nil)
		}
	})
	return &conflictServer{Server: httptest.NewServer(handler), state: state}
}

func (s *conflictServerState) updateModel(w http.ResponseWriter, input modelBasicInfo) {
	if strings.HasPrefix(input.Title, "revision-smoke-") && s.conflict {
		winner := s.waitForConcurrent("model")
		if !winner {
			writeTestEnvelope(w, http.StatusConflict, "assessment model revision conflict; refresh and retry", nil)
			return
		}
	}
	s.mu.Lock()
	s.model.Title, s.model.Description = input.Title, input.Description
	s.model.SubKind, s.model.Algorithm = input.SubKind, input.Algorithm
	s.model.ProductChannel, s.model.Category = input.ProductChannel, input.Category
	s.model.Stages, s.model.ApplicableAges = cloneStrings(input.Stages), cloneStrings(input.ApplicableAges)
	s.model.Reporters, s.model.Tags = cloneStrings(input.Reporters), cloneStrings(input.Tags)
	data := s.model
	s.mu.Unlock()
	writeTestEnvelope(w, http.StatusOK, "ok", data)
}

func (s *conflictServerState) updateQuestionnaire(w http.ResponseWriter, input questionnaireBasicInfo) {
	if strings.HasPrefix(input.Title, "revision-smoke-") && s.conflict {
		winner := s.waitForConcurrent("questionnaire")
		if !winner {
			writeTestEnvelope(w, http.StatusConflict, "questionnaire revision conflict; refresh and retry", nil)
			return
		}
	}
	s.mu.Lock()
	s.questionnaire.Title, s.questionnaire.Description = input.Title, input.Description
	s.questionnaire.ImgURL, s.questionnaire.Type = input.ImgURL, input.Type
	data := s.questionnaire
	s.mu.Unlock()
	writeTestEnvelope(w, http.StatusOK, "ok", data)
}

func (s *conflictServerState) waitForConcurrent(key string) bool {
	s.mu.Lock()
	barrier := s.barriers[key]
	if barrier == nil {
		barrier = &requestBarrier{release: make(chan struct{})}
		s.barriers[key] = barrier
	}
	barrier.arrived++
	position := barrier.arrived
	if barrier.arrived == s.concurrency {
		close(barrier.release)
	}
	release := barrier.release
	s.mu.Unlock()
	<-release
	return position == 1
}

func (s *conflictServer) assertRestored(t *testing.T) {
	t.Helper()
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if s.state.model.Title != "original model" || s.state.model.Description != "model description" {
		t.Fatalf("model not restored: %#v", s.state.model)
	}
	if s.state.questionnaire.Title != "original questionnaire" || s.state.questionnaire.Description != "questionnaire description" {
		t.Fatalf("questionnaire not restored: %#v", s.state.questionnaire)
	}
}

func writeTestEnvelope(w http.ResponseWriter, status int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	code := 0
	if status >= 400 {
		code = status
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message, "data": data})
}

func (s *conflictServerState) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("model=%s questionnaire=%s", s.model.Title, s.questionnaire.Title)
}
