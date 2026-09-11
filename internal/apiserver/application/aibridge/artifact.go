package aibridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
)

// Artifact is the versioned result envelope owned by qs-ai. QS checks transport
// integrity and request correlation; it does not rerun AI generation rules.
type Artifact struct {
	ID                     string `json:"id"`
	SessionID              string `json:"session_id"`
	RunID                  string `json:"run_id"`
	EvidenceSetID          string `json:"evidence_set_id"`
	EvidenceFingerprint    string `json:"evidence_fingerprint"`
	InvocationID           string `json:"invocation_id"`
	ProviderRequestID      string `json:"provider_request_id"`
	ContentJSON            string `json:"content_json"`
	ContentFingerprint     string `json:"content_fingerprint"`
	InputFingerprint       string `json:"input_fingerprint"`
	ProfileID              string `json:"profile_id"`
	ProfileVersion         string `json:"profile_version"`
	ProfileFingerprint     string `json:"profile_fingerprint"`
	PromptFingerprint      string `json:"prompt_fingerprint"`
	RouteFingerprint       string `json:"route_fingerprint"`
	OutputValidatorVersion string `json:"output_validator_version"`
	SafetyValidatorVersion string `json:"safety_validator_version"`
	AssessmentID           string `json:"assessment_id"`
	ReportID               string `json:"report_id"`
	SourceVersion          string `json:"source_version"`
	SchemaVersion          string `json:"schema_version"`
}

func digest(s string, prefixed bool) bool {
	if prefixed {
		if !strings.HasPrefix(s, "sha256:") {
			return false
		}
		s = strings.TrimPrefix(s, "sha256:")
	}
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func ValidateArtifact(e Event) (*Artifact, error) {
	if e.Status != "completed" {
		if e.ArtifactJSON != "" {
			return nil, ErrInvalid
		}
		return nil, nil
	}
	if len(e.ArtifactJSON) == 0 || len(e.ArtifactJSON) > 131072 || e.QuestionID != "" || e.Question != "" || e.CanSkip || e.FailureCode != "" {
		return nil, ErrInvalid
	}
	var a Artifact
	dec := json.NewDecoder(bytes.NewBufferString(e.ArtifactJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return nil, ErrInvalid
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, ErrInvalid
	}
	if a.SchemaVersion != "qs-ai-artifact/v1" || a.SessionID != e.SessionID || !validID(a.ID) || !validID(a.RunID) || !validID(a.EvidenceSetID) || !validID(a.InvocationID) || !validNumber(a.AssessmentID) || !validNumber(a.ReportID) {
		return nil, ErrInvalid
	}
	if !digest(a.EvidenceFingerprint, false) {
		return nil, ErrInvalid
	}
	for _, value := range []string{a.ContentFingerprint, a.InputFingerprint, a.ProfileFingerprint, a.PromptFingerprint, a.RouteFingerprint} {
		if !digest(value, true) {
			return nil, ErrInvalid
		}
	}
	for _, value := range []string{a.ProviderRequestID, a.ProfileID, a.ProfileVersion, a.OutputValidatorVersion, a.SafetyValidatorVersion, a.SourceVersion} {
		if strings.TrimSpace(value) == "" || len(value) > 255 {
			return nil, ErrInvalid
		}
	}
	sum := sha256.Sum256([]byte(a.ContentJSON))
	if a.ContentFingerprint != "sha256:"+hex.EncodeToString(sum[:]) {
		return nil, ErrInvalid
	}
	var content struct {
		SchemaVersion string `json:"schema_version"`
	}
	if json.Unmarshal([]byte(a.ContentJSON), &content) != nil || content.SchemaVersion != "ai-explanation-output/v1" {
		return nil, ErrInvalid
	}
	return &a, nil
}
