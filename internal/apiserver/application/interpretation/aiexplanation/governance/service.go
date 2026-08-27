// Package governance applies release controls to AI explanation Profiles.
// Runtime request paths may resolve only published Profiles; they never call
// this service or mutate Profile lifecycle state.
package governance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var (
	ErrPublishEvidenceRequired = errors.New("approved AI explanation Prompt evaluation evidence is required")
	ErrReleaseMismatch         = errors.New("AI explanation evaluation release does not match the Profile")
	ErrSelectorConflict        = errors.New("AI explanation published Profile selector conflicts at the same specificity")
	ErrProfileFingerprint      = errors.New("AI explanation Profile fingerprint does not match its definition")
)

type Service struct {
	profiles    domainprofile.Repository
	evaluations domainevaluation.Repository
	now         func() time.Time
	newID       func() meta.ID
}

func NewService(profiles domainprofile.Repository, evaluations domainevaluation.Repository, now func() time.Time, newIDs ...func() meta.ID) (*Service, error) {
	if profiles == nil || evaluations == nil {
		return nil, fmt.Errorf("AI explanation Profile and evaluation repositories are required")
	}
	if now == nil {
		now = time.Now
	}
	newID := meta.New
	if len(newIDs) > 0 && newIDs[0] != nil {
		newID = newIDs[0]
	}
	return &Service{profiles: profiles, evaluations: evaluations, now: now, newID: newID}, nil
}

type CreateDraftCommand struct {
	Definition          domainprofile.Definition
	ExpectedFingerprint aiexplanation.Fingerprint
	Actor               string
	Reason              string
}

func (s *Service) CreateDraft(ctx context.Context, command CreateDraftCommand) (*domainprofile.AIExplanationProfile, error) {
	if s == nil || s.newID == nil || strings.TrimSpace(command.Actor) == "" || strings.TrimSpace(command.Reason) == "" {
		return nil, fmt.Errorf("AI explanation Profile draft command is invalid")
	}
	if err := command.ExpectedFingerprint.Validate(); err != nil {
		return nil, ErrProfileFingerprint
	}
	profileRecord, err := domainprofile.NewDraftForRelease(
		s.newID(), command.Definition, command.Actor, command.Reason, s.now(),
	)
	if err != nil {
		return nil, err
	}
	if profileRecord.Fingerprint() != command.ExpectedFingerprint {
		return nil, ErrProfileFingerprint
	}
	if err := s.profiles.Save(ctx, profileRecord); err != nil {
		return nil, err
	}
	return profileRecord, nil
}

type PublishCommand struct {
	ProfileID       string
	ProfileVersion  string
	EvaluationRunID meta.ID
	Actor           string
	Reason          string
}

func (s *Service) Publish(ctx context.Context, command PublishCommand) (*domainprofile.AIExplanationProfile, error) {
	if s == nil || strings.TrimSpace(command.ProfileID) == "" || strings.TrimSpace(command.ProfileVersion) == "" || command.EvaluationRunID.IsZero() || strings.TrimSpace(command.Actor) == "" || strings.TrimSpace(command.Reason) == "" {
		return nil, fmt.Errorf("AI explanation Profile publish command is invalid")
	}
	profileRecord, err := s.profiles.FindByKey(ctx, command.ProfileID, command.ProfileVersion)
	if err != nil {
		return nil, err
	}
	if profileRecord.Status() != domainprofile.StatusDraft {
		return nil, domainprofile.ErrConflict
	}
	evidence, err := s.evaluations.FindByID(ctx, command.EvaluationRunID)
	if err != nil {
		return nil, err
	}
	if !evidence.IsPublishEvidence() {
		return nil, ErrPublishEvidenceRequired
	}
	if err := validateReleaseMatch(profileRecord, evidence); err != nil {
		return nil, err
	}
	if err := s.ensureSelectorSlotAvailable(ctx, profileRecord); err != nil {
		return nil, err
	}
	if err := profileRecord.Publish(command.EvaluationRunID, command.Actor, command.Reason, s.now()); err != nil {
		return nil, err
	}
	if err := s.profiles.Save(ctx, profileRecord); err != nil {
		return nil, err
	}
	return profileRecord, nil
}

type DisableCommand struct {
	ProfileID      string
	ProfileVersion string
	Actor          string
	Reason         string
}

func (s *Service) Disable(ctx context.Context, command DisableCommand) (*domainprofile.AIExplanationProfile, error) {
	if s == nil || strings.TrimSpace(command.ProfileID) == "" || strings.TrimSpace(command.ProfileVersion) == "" || strings.TrimSpace(command.Actor) == "" || strings.TrimSpace(command.Reason) == "" {
		return nil, fmt.Errorf("AI explanation Profile disable command is invalid")
	}
	profileRecord, err := s.profiles.FindByKey(ctx, command.ProfileID, command.ProfileVersion)
	if err != nil {
		return nil, err
	}
	if err := profileRecord.Disable(command.Actor, command.Reason, s.now()); err != nil {
		return nil, err
	}
	if err := s.profiles.Save(ctx, profileRecord); err != nil {
		return nil, err
	}
	return profileRecord, nil
}

func validateReleaseMatch(profileRecord *domainprofile.AIExplanationProfile, evidence *domainevaluation.PromptEvaluationRun) error {
	if profileRecord == nil || evidence == nil {
		return ErrReleaseMismatch
	}
	release := evidence.Release()
	definition := profileRecord.Definition()
	if release.Profile.ID != profileRecord.ProfileID() || release.Profile.Version != profileRecord.Version() || release.Profile.Fingerprint != profileRecord.Fingerprint() ||
		release.Prompt.TemplateID != definition.GenerationPolicy.PromptTemplateID || release.Prompt.Version != definition.GenerationPolicy.PromptVersion ||
		release.InputSchema.Version != definition.GenerationPolicy.InputSchemaVersion || release.OutputSchema.Version != definition.GenerationPolicy.OutputSchemaVersion ||
		release.Provider.Route != definition.GenerationPolicy.ProviderRoute {
		return ErrReleaseMismatch
	}
	return nil
}

func (s *Service) ensureSelectorSlotAvailable(ctx context.Context, candidate *domainprofile.AIExplanationProfile) error {
	selector := candidate.Selector()
	existing, err := s.profiles.ListPublishedByBaseSelector(ctx, selector.Audience, selector.ModelKind, selector.DecisionKind)
	if err != nil {
		return err
	}
	for _, published := range existing {
		if published == nil || published.Status() != domainprofile.StatusPublished || published.ID() == candidate.ID() {
			continue
		}
		if sameResolutionSlot(selector, published.Selector()) {
			return ErrSelectorConflict
		}
	}
	return nil
}

func sameResolutionSlot(left, right domainprofile.Selector) bool {
	if left.Audience != right.Audience || left.ModelKind != right.ModelKind || left.DecisionKind != right.DecisionKind || left.Specificity() != right.Specificity() {
		return false
	}
	switch left.Specificity() {
	case 0:
		return true
	case 1:
		return left.ModelCode != nil && right.ModelCode != nil && *left.ModelCode == *right.ModelCode
	case 2:
		return left.ModelCode != nil && right.ModelCode != nil && left.ModelVersion != nil && right.ModelVersion != nil && *left.ModelCode == *right.ModelCode && *left.ModelVersion == *right.ModelVersion
	default:
		return false
	}
}
