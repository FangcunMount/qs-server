// Package subjectexport owns the participant-authorized projection used for
// personal-data export. It deliberately exposes immutable final artifacts and
// their release provenance, never internal execution or capacity records.
package subjectexport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var ErrInvalidQuery = errors.New("invalid AI explanation subject export query")

const (
	SchemaVersionV1 = "ai-explanation-subject-export/v1"
	DefaultPageSize = 50
	MaxPageSize     = 100
	cursorVersionV1 = "v1"
)

type Subject struct {
	OrgID    int64
	TesteeID meta.ID
}

func (s Subject) Validate() error {
	if s.OrgID <= 0 || s.TesteeID.IsZero() {
		return fmt.Errorf("AI explanation export organization and Testee are required")
	}
	return nil
}

type Query struct {
	Subject  Subject
	PageSize int
	Cursor   string
}

type ReadQuery struct {
	Subject          Subject
	SnapshotAt       time.Time
	AfterGeneratedAt time.Time
	AfterArtifactID  meta.ID
	Limit            int
}

type Reader interface {
	ListParticipantArtifacts(context.Context, ReadQuery) ([]*artifact.AIExplanationArtifact, error)
}

type SourceReceipt struct {
	AssessmentID         meta.ID
	ReportID             meta.ID
	OutcomeID            meta.ID
	ReportType           string
	TemplateVersion      string
	ContentSchemaVersion string
	BuilderIdentity      string
	ReportGeneratedAt    time.Time
}

type ReleaseReceipt struct {
	ProfileID                 string
	ProfileVersion            string
	ProfileFingerprint        string
	PromptTemplateID          string
	PromptVersion             string
	PromptFingerprint         string
	PromptGitBlobSHA          string
	ProviderRoute             string
	ProviderRouteRevision     string
	ResolvedProvider          string
	ResolvedModel             string
	ExecutionSpecFingerprint  string
	InputSchema               string
	OutputSchema              string
	SafetyPolicy              string
	SchemaValidatorVersion    string
	ReferenceValidatorVersion string
	ProfileValidatorVersion   string
	SafetyValidatorVersion    string
	ValidatedAt               time.Time
}

type Item struct {
	GenerationID meta.ID
	ArtifactID   meta.ID
	Source       SourceReceipt
	Release      ReleaseReceipt
	Content      output.Content
	GeneratedAt  time.Time
}

type Page struct {
	SchemaVersion string
	Subject       Subject
	ExportedAt    time.Time
	SnapshotAt    time.Time
	Items         []Item
	NextCursor    string
}

type Service struct {
	reader Reader
	now    func() time.Time
}

func NewService(reader Reader, now func() time.Time) (*Service, error) {
	if reader == nil || now == nil {
		return nil, fmt.Errorf("AI explanation subject export dependencies are required")
	}
	return &Service{reader: reader, now: now}, nil
}

// Export returns one stable page from a snapshot fixed by the first request.
// Authorization is intentionally owned by the participant transport so this
// use case can never be reused as an identity oracle.
func (s *Service) Export(ctx context.Context, query Query) (*Page, error) {
	if s == nil || s.reader == nil || s.now == nil {
		return nil, fmt.Errorf("AI explanation subject export is not configured")
	}
	if err := query.Subject.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidQuery, err)
	}
	pageSize, err := normalizePageSize(query.PageSize)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidQuery, err)
	}
	exportedAt := s.now().UTC()
	if exportedAt.IsZero() {
		return nil, fmt.Errorf("AI explanation export time is required")
	}
	state := exportCursor{Version: cursorVersionV1, OrgID: query.Subject.OrgID, TesteeID: query.Subject.TesteeID, SnapshotAt: exportedAt}
	if strings.TrimSpace(query.Cursor) != "" {
		state, err = decodeCursor(query.Cursor, query.Subject, exportedAt)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidQuery, err)
		}
	}
	values, err := s.reader.ListParticipantArtifacts(ctx, ReadQuery{
		Subject: query.Subject, SnapshotAt: state.SnapshotAt,
		AfterGeneratedAt: state.AfterGeneratedAt, AfterArtifactID: state.AfterArtifactID,
		Limit: pageSize + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("read participant AI explanation export: %w", err)
	}
	if len(values) > pageSize+1 {
		return nil, fmt.Errorf("AI explanation export reader exceeded requested limit")
	}
	hasMore := len(values) > pageSize
	if hasMore {
		values = values[:pageSize]
	}
	items := make([]Item, 0, len(values))
	var previous *artifact.AIExplanationArtifact
	for _, value := range values {
		if err := validateArtifact(query.Subject, state, previous, value); err != nil {
			return nil, err
		}
		items = append(items, projectArtifact(value))
		previous = value
	}
	nextCursor := ""
	if hasMore && len(values) > 0 {
		last := values[len(values)-1]
		state.AfterGeneratedAt = last.GeneratedAt().UTC()
		state.AfterArtifactID = last.ID()
		nextCursor, err = encodeCursor(state)
		if err != nil {
			return nil, err
		}
	}
	return &Page{
		SchemaVersion: SchemaVersionV1, Subject: query.Subject, ExportedAt: exportedAt,
		SnapshotAt: state.SnapshotAt, Items: items, NextCursor: nextCursor,
	}, nil
}

func normalizePageSize(value int) (int, error) {
	if value == 0 {
		return DefaultPageSize, nil
	}
	if value < 1 || value > MaxPageSize {
		return 0, fmt.Errorf("AI explanation export page size must be between 1 and %d", MaxPageSize)
	}
	return value, nil
}

type exportCursor struct {
	Version          string    `json:"v"`
	OrgID            int64     `json:"org_id"`
	TesteeID         meta.ID   `json:"testee_id"`
	SnapshotAt       time.Time `json:"snapshot_at"`
	AfterGeneratedAt time.Time `json:"after_generated_at"`
	AfterArtifactID  meta.ID   `json:"after_artifact_id"`
}

func encodeCursor(value exportCursor) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode AI explanation export cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(raw string, subject Subject, now time.Time) (exportCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return exportCursor{}, fmt.Errorf("AI explanation export cursor is invalid")
	}
	var value exportCursor
	if err := json.Unmarshal(payload, &value); err != nil {
		return exportCursor{}, fmt.Errorf("AI explanation export cursor is invalid")
	}
	if value.Version != cursorVersionV1 || value.OrgID != subject.OrgID || value.TesteeID != subject.TesteeID || value.SnapshotAt.IsZero() || value.SnapshotAt.After(now.Add(time.Minute)) || value.AfterGeneratedAt.IsZero() || value.AfterArtifactID.IsZero() || value.AfterGeneratedAt.After(value.SnapshotAt) {
		return exportCursor{}, fmt.Errorf("AI explanation export cursor is invalid")
	}
	value.SnapshotAt = value.SnapshotAt.UTC()
	value.AfterGeneratedAt = value.AfterGeneratedAt.UTC()
	return value, nil
}

func validateArtifact(subject Subject, cursor exportCursor, previous, value *artifact.AIExplanationArtifact) error {
	if value == nil || value.ID().IsZero() || value.GenerationID().IsZero() || value.Audience() != policy.AudienceParticipant {
		return fmt.Errorf("AI explanation export contains an invalid participant artifact")
	}
	source := value.Source()
	if source.Association.OrgID != subject.OrgID || source.Association.TesteeID != subject.TesteeID.Uint64() {
		return fmt.Errorf("AI explanation export contains a cross-subject artifact")
	}
	generatedAt := value.GeneratedAt().UTC()
	if generatedAt.After(cursor.SnapshotAt) {
		return fmt.Errorf("AI explanation export contains an artifact outside the snapshot")
	}
	if !cursor.AfterGeneratedAt.IsZero() && (generatedAt.After(cursor.AfterGeneratedAt) || (generatedAt.Equal(cursor.AfterGeneratedAt) && value.ID().Uint64() >= cursor.AfterArtifactID.Uint64())) {
		return fmt.Errorf("AI explanation export reader did not honor the cursor")
	}
	if previous != nil {
		previousAt := previous.GeneratedAt().UTC()
		if generatedAt.After(previousAt) || (generatedAt.Equal(previousAt) && value.ID().Uint64() >= previous.ID().Uint64()) {
			return fmt.Errorf("AI explanation export artifacts are not deterministically ordered")
		}
	}
	return nil
}

func projectArtifact(value *artifact.AIExplanationArtifact) Item {
	source := value.Source()
	profile := value.Profile()
	prompt := value.Prompt()
	execution := value.ExecutionSpec()
	validation := value.Validation()
	return Item{
		GenerationID: value.GenerationID(), ArtifactID: value.ID(),
		Source: SourceReceipt{
			AssessmentID: source.Association.AssessmentID, ReportID: source.ReportID, OutcomeID: source.OutcomeID,
			ReportType: source.ReportType, TemplateVersion: source.TemplateVersion,
			ContentSchemaVersion: source.ContentSchemaVersion, BuilderIdentity: source.BuilderIdentity,
			ReportGeneratedAt: source.ReportGeneratedAt.UTC(),
		},
		Release: ReleaseReceipt{
			ProfileID: profile.ID, ProfileVersion: profile.Version, ProfileFingerprint: profile.Fingerprint.String(),
			PromptTemplateID: prompt.TemplateID, PromptVersion: prompt.Version, PromptFingerprint: prompt.Fingerprint.String(), PromptGitBlobSHA: prompt.GitBlobSHA,
			ProviderRoute: execution.Route, ProviderRouteRevision: execution.RouteRevision,
			ResolvedProvider: execution.ResolvedProvider, ResolvedModel: execution.ResolvedModel,
			ExecutionSpecFingerprint: execution.Fingerprint.String(), InputSchema: value.InputSchema(), OutputSchema: value.OutputSchema(),
			SafetyPolicy: value.SafetyPolicy(), SchemaValidatorVersion: validation.SchemaValidatorVersion,
			ReferenceValidatorVersion: validation.ReferenceValidatorVersion, ProfileValidatorVersion: validation.ProfileValidatorVersion,
			SafetyValidatorVersion: validation.SafetyValidatorVersion, ValidatedAt: validation.ValidatedAt.UTC(),
		},
		Content: value.Content(), GeneratedAt: value.GeneratedAt().UTC(),
	}
}
