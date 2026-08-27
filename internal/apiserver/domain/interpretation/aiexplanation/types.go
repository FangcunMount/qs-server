// Package aiexplanation owns shared value objects for the optional AI
// explanation capability. Standard Interpretation reports remain the
// authoritative result and are referenced as immutable sources only.
package aiexplanation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const (
	InputSchemaVersionV1                    = "ai-explanation-input/v1"
	OutputSchemaVersionV1                   = "ai-explanation-output/v1"
	ProfileSchemaVersionV1                  = "ai-explanation-profile/v1"
	SemanticEvaluationOutputSchemaVersionV1 = "ai-explanation-semantic-evaluation-output/v1"
)

var (
	fingerprintPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	versionPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
	routePattern       = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
)

// Fingerprint is a lowercase SHA-256 content identity.
type Fingerprint string

func NewFingerprint(payload []byte) Fingerprint {
	digest := sha256.Sum256(payload)
	return Fingerprint("sha256:" + hex.EncodeToString(digest[:]))
}

func ParseFingerprint(value string) (Fingerprint, error) {
	value = strings.TrimSpace(value)
	if !fingerprintPattern.MatchString(value) {
		return "", fmt.Errorf("AI explanation fingerprint is invalid")
	}
	return Fingerprint(value), nil
}

func (f Fingerprint) Validate() error {
	_, err := ParseFingerprint(string(f))
	return err
}

func (f Fingerprint) String() string { return string(f) }

func ValidateVersion(value string) error {
	if !versionPattern.MatchString(value) {
		return fmt.Errorf("AI explanation version is invalid")
	}
	return nil
}

func ValidateRouteKey(value string) error {
	if !routePattern.MatchString(value) {
		return fmt.Errorf("AI explanation provider route is invalid")
	}
	return nil
}

// Association is an authorization/query correlation copied from the source
// report. It grants no authority to mutate the Assessment or standard report.
type Association struct {
	OrgID        int64
	AssessmentID meta.ID
	TesteeID     uint64
}

func (a Association) Validate() error {
	if a.OrgID == 0 || a.AssessmentID.IsZero() || a.TesteeID == 0 {
		return fmt.Errorf("AI explanation organization, assessment and testee association are required")
	}
	return nil
}

// ActorRef records who initiated a manual generation without participating in
// the semantic idempotency key.
type ActorRef struct {
	Kind string
	ID   string
}

func (a ActorRef) Validate() error {
	switch strings.TrimSpace(a.Kind) {
	case "participant", "clinician", "system":
	default:
		return fmt.Errorf("AI explanation requester kind is invalid")
	}
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("AI explanation requester id is required")
	}
	return nil
}

// ProfileRef freezes the exact published Profile release used by a generation.
type ProfileRef struct {
	ID          string
	Version     string
	Fingerprint Fingerprint
}

func (r ProfileRef) Validate() error {
	if strings.TrimSpace(r.ID) == "" || !versionPattern.MatchString(r.Version) {
		return fmt.Errorf("AI explanation profile id and version are invalid")
	}
	return r.Fingerprint.Validate()
}

// PromptRef freezes the exact executable Prompt package.
type PromptRef struct {
	TemplateID  string
	Version     string
	Fingerprint Fingerprint
	GitBlobSHA  string
}

func (r PromptRef) Validate() error {
	if strings.TrimSpace(r.TemplateID) == "" || !versionPattern.MatchString(r.Version) {
		return fmt.Errorf("AI explanation prompt id and version are invalid")
	}
	if err := r.Fingerprint.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.GitBlobSHA) == "" {
		return fmt.Errorf("AI explanation prompt git blob sha is required")
	}
	return nil
}

// ProviderExecutionSpec is a server-only frozen resolution of a logical route.
// It contains no endpoint or credential.
type ProviderExecutionSpec struct {
	Route            string
	RouteRevision    string
	ResolvedProvider string
	ResolvedModel    string
	Fingerprint      Fingerprint
}

func (s ProviderExecutionSpec) Validate() error {
	if !routePattern.MatchString(s.Route) || !versionPattern.MatchString(s.RouteRevision) {
		return fmt.Errorf("AI explanation provider route and revision are invalid")
	}
	if strings.TrimSpace(s.ResolvedProvider) == "" || strings.TrimSpace(s.ResolvedModel) == "" {
		return fmt.Errorf("AI explanation resolved provider and model are required")
	}
	return s.Fingerprint.Validate()
}

// ProviderReceipt records non-secret execution evidence returned by a provider.
type ProviderReceipt struct {
	InvocationID string
	RequestID    string
	Provider     string
	Model        string
	InputTokens  int64
	OutputTokens int64
	Latency      time.Duration
}

func (r ProviderReceipt) Validate() error {
	if strings.TrimSpace(r.InvocationID) == "" || strings.TrimSpace(r.Provider) == "" || strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("AI explanation provider invocation, provider and model are required")
	}
	if r.InputTokens < 0 || r.OutputTokens < 0 || r.Latency < 0 {
		return fmt.Errorf("AI explanation provider usage is invalid")
	}
	return nil
}

func ValidateAudience(audience policy.Audience) error {
	switch audience {
	case policy.AudienceParticipant, policy.AudienceClinician:
		return nil
	default:
		return fmt.Errorf("AI explanation audience is invalid")
	}
}
