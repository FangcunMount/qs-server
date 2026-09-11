package aibridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func artifactEvent(t *testing.T) Event {
	t.Helper()
	e := Event{SessionID: uuid.NewString(), Status: "completed"}
	content := `{"schema_version":"ai-explanation-output/v1"}`
	hash := sha256.Sum256([]byte(content))
	d := "sha256:" + strings.Repeat("a", 64)
	a := Artifact{ID: uuid.NewString(), SessionID: e.SessionID, RunID: uuid.NewString(), EvidenceSetID: uuid.NewString(), EvidenceFingerprint: strings.Repeat("a", 64), InvocationID: uuid.NewString(), ProviderRequestID: "provider-1", ContentJSON: content, ContentFingerprint: "sha256:" + hex.EncodeToString(hash[:]), InputFingerprint: d, ProfileID: "profile", ProfileVersion: "v6", ProfileFingerprint: d, PromptFingerprint: d, RouteFingerprint: d, OutputValidatorVersion: "v1", SafetyValidatorVersion: "v2", AssessmentID: "42", ReportID: "99", SourceVersion: "standard-v1:101", SchemaVersion: "qs-ai-artifact/v1"}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	e.ArtifactJSON = string(raw)
	return e
}
func TestArtifactEnvelopeIntegrity(t *testing.T) {
	original := artifactEvent(t)
	if _, err := ValidateArtifact(original); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing", "session", "hash", "schema", "nonterminal", "mixed", "unknown", "oversize"} {
		t.Run(name, func(t *testing.T) {
			e := original
			switch name {
			case "missing":
				e.ArtifactJSON = ""
			case "session":
				e.SessionID = uuid.NewString()
			case "hash":
				e.ArtifactJSON = strings.Replace(e.ArtifactJSON, "ai-explanation-output/v1", "ai-explanation-output/v2", 1)
			case "schema":
				e.ArtifactJSON = strings.Replace(e.ArtifactJSON, "qs-ai-artifact/v1", "qs-ai-artifact/v2", 1)
			case "nonterminal":
				e.Status = "running"
			case "mixed":
				e.FailureCode = "failure"
			case "unknown":
				e.ArtifactJSON = strings.TrimSuffix(e.ArtifactJSON, "}") + `,"surprise":true}`
			case "oversize":
				e.ArtifactJSON = strings.Repeat("x", 131073)
			}
			if _, err := ValidateArtifact(e); err == nil {
				t.Fatal("invalid artifact accepted")
			}
		})
	}
}
func TestLegacyEventEncodingOmitsArtifact(t *testing.T) {
	raw, err := json.Marshal(Event{Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "artifact_json") {
		t.Fatal("legacy event hash changed")
	}
	if _, err := ValidateArtifact(Event{Status: "running"}); err != nil {
		t.Fatal(err)
	}
}
