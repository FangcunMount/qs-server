// Package participant owns participant-triggered AI explanation capability,
// request and query use cases.
package participant

import (
	"context"
	"errors"
	"fmt"
	"time"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	appsource "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var (
	ErrAccessMismatch = errors.New("AI explanation does not belong to participant assessment")
	ErrConfiguration  = errors.New("AI explanation runtime configuration is invalid")
	ErrInvalidRequest = errors.New("AI explanation participant request is invalid")
)

type Actor struct {
	SubjectID string
	TesteeID  uint64
}

type RequestInput struct {
	AssessmentID meta.ID
	Locale       string
	FocusAreas   []string
}

type GetInput struct {
	AssessmentID meta.ID
	GenerationID meta.ID
}

type Status string

const (
	StatusReady         Status = "ready"
	StatusNotReady      Status = "not_ready"
	StatusNotApplicable Status = "not_applicable"
	StatusPending       Status = "pending"
	StatusGenerating    Status = "generating"
	StatusGenerated     Status = "generated"
	StatusFailed        Status = "failed"
)

type SourceState string

const (
	SourceStateCurrent     SourceState = "current"
	SourceStateStale       SourceState = "stale"
	SourceStateUnavailable SourceState = "unavailable"
	SourceStateUnknown     SourceState = "unknown"
)

type Failure struct {
	Code        string
	SafeMessage string
	Retryable   bool
}

type Result struct {
	Status         Status
	ReasonCode     string
	GenerationID   meta.ID
	ArtifactID     meta.ID
	SourceReportID meta.ID
	SourceState    SourceState
	Content        *domainoutput.Content
	Failure        *Failure
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Access interface {
	AuthorizeOwnAssessment(ctx context.Context, testeeID, assessmentID uint64) error
}

type Service interface {
	Capability(context.Context, Actor, RequestInput) (*Result, error)
	Request(context.Context, Actor, RequestInput) (*Result, error)
	Get(context.Context, Actor, GetInput) (*Result, error)
}

type service struct {
	access      Access
	sources     appsource.Resolver
	profiles    domainprofile.Repository
	prompts     appport.PromptPackageResolver
	routes      appport.ProviderRouteResolver
	requests    appport.RequestCommitter
	generations domaingeneration.Repository
	runs        domainrun.Repository
	artifacts   domainartifact.Repository
	catalog     interpretationreadmodel.BatchReportMetadataReader
	now         func() time.Time
	newID       func() meta.ID
}

func NewService(
	access Access,
	sources appsource.Resolver,
	profiles domainprofile.Repository,
	prompts appport.PromptPackageResolver,
	routes appport.ProviderRouteResolver,
	requests appport.RequestCommitter,
	generations domaingeneration.Repository,
	runs domainrun.Repository,
	artifacts domainartifact.Repository,
	catalog interpretationreadmodel.BatchReportMetadataReader,
) (Service, error) {
	if access == nil || sources == nil || profiles == nil || prompts == nil || routes == nil || requests == nil || generations == nil || runs == nil || artifacts == nil || catalog == nil {
		return nil, fmt.Errorf("AI explanation participant service dependencies are required")
	}
	return &service{
		access: access, sources: sources, profiles: profiles, prompts: prompts, routes: routes, requests: requests,
		generations: generations, runs: runs, artifacts: artifacts, catalog: catalog, now: time.Now, newID: meta.New,
	}, nil
}

type prepared struct {
	input         *appinput.Result
	profile       *domainprofile.AIExplanationProfile
	prompt        appport.PromptPackage
	providerRoute appport.ProviderRoute
	source        *appsource.Current
}

func (s *service) Capability(ctx context.Context, actor Actor, input RequestInput) (*Result, error) {
	if err := validateRequest(actor, input); err != nil {
		return nil, err
	}
	if err := s.access.AuthorizeOwnAssessment(ctx, actor.TesteeID, input.AssessmentID.Uint64()); err != nil {
		return nil, err
	}
	prepared, unavailable, err := s.prepare(ctx, input)
	if err != nil || unavailable != nil {
		return unavailable, err
	}
	return &Result{Status: StatusReady, SourceReportID: prepared.source.Report.ID(), SourceState: SourceStateCurrent}, nil
}

func (s *service) Request(ctx context.Context, actor Actor, input RequestInput) (*Result, error) {
	if err := validateRequest(actor, input); err != nil {
		return nil, err
	}
	if err := s.access.AuthorizeOwnAssessment(ctx, actor.TesteeID, input.AssessmentID.Uint64()); err != nil {
		return nil, err
	}
	prepared, unavailable, err := s.prepare(ctx, input)
	if err != nil || unavailable != nil {
		return unavailable, err
	}
	association := prepared.source.Report.Association()
	profileRef := aiexplanation.ProfileRef{ID: prepared.profile.ProfileID(), Version: prepared.profile.Version(), Fingerprint: prepared.profile.Fingerprint()}
	key := domaingeneration.Key{
		SourceReportID: prepared.source.Report.ID(), Audience: policy.AudienceParticipant, Profile: profileRef,
		InputFingerprint: prepared.input.Snapshot.Fingerprint(), ExecutionSpecFingerprint: prepared.providerRoute.ExecutionSpec.Fingerprint,
	}
	if existing, findErr := s.generations.FindByKey(ctx, key); findErr == nil {
		observeParticipantRequest("reused")
		return s.resultFromGeneration(ctx, existing, SourceStateCurrent)
	} else if !errors.Is(findErr, domaingeneration.ErrNotFound) {
		observeParticipantRequest("error")
		return nil, fmt.Errorf("check existing AI explanation request: %w", findErr)
	}
	generationRecord, err := domaingeneration.New(domaingeneration.NewInput{
		ID: s.newID(), Key: key,
		Association: aiexplanation.Association{OrgID: association.OrgID, AssessmentID: association.AssessmentID, TesteeID: association.TesteeID},
		RequestedBy: aiexplanation.ActorRef{Kind: "participant", ID: actor.SubjectID},
		Input:       prepared.input.Snapshot, Prompt: prepared.prompt.Ref, ExecutionSpec: prepared.providerRoute.ExecutionSpec, CreatedAt: s.now(),
	})
	if err != nil {
		observeParticipantRequest("error")
		return nil, err
	}
	if err := s.requests.CommitRequested(ctx, generationRecord); err != nil {
		if !errors.Is(err, domaingeneration.ErrAlreadyExists) && !isParticipantCapacityExceeded(err) {
			observeParticipantRequest("error")
			return nil, fmt.Errorf("commit AI explanation request: %w", err)
		}
		existing, findErr := s.generations.FindByKey(ctx, key)
		if findErr == nil {
			observeParticipantRequest("reused")
			return s.resultFromGeneration(ctx, existing, SourceStateCurrent)
		}
		if isParticipantCapacityExceeded(err) && errors.Is(findErr, domaingeneration.ErrNotFound) {
			observeParticipantRequest("capacity_rejected")
			return nil, err
		}
		if findErr != nil {
			observeParticipantRequest("error")
			return nil, fmt.Errorf("load concurrent AI explanation request: %w", findErr)
		}
	}
	observeParticipantRequest("created")
	return s.resultFromGeneration(ctx, generationRecord, SourceStateCurrent)
}

func isParticipantCapacityExceeded(err error) bool {
	return errors.Is(err, domaingeneration.ErrOrgDailyBudgetExceeded) ||
		errors.Is(err, domaingeneration.ErrUserDailyBudgetExceeded) ||
		errors.Is(err, domaingeneration.ErrAssessmentDailyBudgetExceeded)
}

func (s *service) Get(ctx context.Context, actor Actor, input GetInput) (*Result, error) {
	if actor.TesteeID == 0 || actor.SubjectID == "" || input.AssessmentID.IsZero() || input.GenerationID.IsZero() {
		return nil, fmt.Errorf("participant, assessment and AI explanation generation are required")
	}
	if err := s.access.AuthorizeOwnAssessment(ctx, actor.TesteeID, input.AssessmentID.Uint64()); err != nil {
		return nil, err
	}
	generationRecord, err := s.generations.FindByID(ctx, input.GenerationID)
	if err != nil {
		return nil, err
	}
	association := generationRecord.Association()
	if association.AssessmentID != input.AssessmentID || association.TesteeID != actor.TesteeID {
		return nil, ErrAccessMismatch
	}
	return s.resultFromGeneration(ctx, generationRecord, s.sourceState(ctx, input.AssessmentID, generationRecord.Key().SourceReportID))
}

func (s *service) prepare(ctx context.Context, input RequestInput) (*prepared, *Result, error) {
	current, err := s.sources.ResolveCurrent(ctx, input.AssessmentID)
	if err != nil {
		switch {
		case errors.Is(err, appsource.ErrNotReady):
			return nil, &Result{Status: StatusNotReady, ReasonCode: "standard_report_not_ready", SourceState: SourceStateUnavailable}, nil
		case errors.Is(err, appsource.ErrNotApplicable):
			return nil, &Result{Status: StatusNotApplicable, ReasonCode: "source_not_supported", SourceState: SourceStateCurrent}, nil
		default:
			return nil, nil, err
		}
	}
	model := current.Outcome.Model()
	profileRecord, err := domainprofile.Resolve(ctx, s.profiles, domainprofile.ResolveQuery{
		Audience: policy.AudienceParticipant, ModelKind: model.Kind, DecisionKind: current.Outcome.Runtime().DecisionKind,
		ModelCode: model.Code, ModelVersion: model.Version,
	})
	if err != nil {
		switch {
		case errors.Is(err, domainprofile.ErrNotFound):
			return nil, &Result{Status: StatusNotApplicable, ReasonCode: "profile_unresolved", SourceReportID: current.Report.ID(), SourceState: SourceStateCurrent}, nil
		case errors.Is(err, domainprofile.ErrAmbiguousSelector):
			return nil, &Result{Status: StatusNotApplicable, ReasonCode: "profile_mismatch", SourceReportID: current.Report.ID(), SourceState: SourceStateCurrent}, nil
		default:
			return nil, nil, err
		}
	}
	assembled, err := appinput.Assemble(appinput.Request{
		Source: current, Profile: profileRecord, Audience: policy.AudienceParticipant, Locale: input.Locale, FocusAreas: input.FocusAreas,
	})
	if err != nil {
		switch {
		case errors.Is(err, appinput.ErrNotApplicable):
			return nil, &Result{Status: StatusNotApplicable, ReasonCode: "not_applicable", SourceReportID: current.Report.ID(), SourceState: SourceStateCurrent}, nil
		case errors.Is(err, appinput.ErrProfileMismatch):
			return nil, &Result{Status: StatusNotApplicable, ReasonCode: "profile_mismatch", SourceReportID: current.Report.ID(), SourceState: SourceStateCurrent}, nil
		case errors.Is(err, appinput.ErrInvalidInput):
			return nil, nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
		default:
			return nil, nil, err
		}
	}
	definition := profileRecord.Definition()
	prompt, err := s.prompts.ResolvePromptPackage(ctx, definition.GenerationPolicy.PromptTemplateID, definition.GenerationPolicy.PromptVersion)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: resolve Prompt package: %v", ErrConfiguration, err)
	}
	if err := prompt.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid Prompt package: %v", ErrConfiguration, err)
	}
	if prompt.Ref.TemplateID != definition.GenerationPolicy.PromptTemplateID || prompt.Ref.Version != definition.GenerationPolicy.PromptVersion {
		return nil, nil, fmt.Errorf("%w: Prompt package identity mismatch", ErrConfiguration)
	}
	providerRoute, err := s.routes.ResolveProviderRoute(ctx, definition.GenerationPolicy.ProviderRoute)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: resolve provider route: %v", ErrConfiguration, err)
	}
	if err := providerRoute.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid provider route: %v", ErrConfiguration, err)
	}
	if providerRoute.ExecutionSpec.Route != definition.GenerationPolicy.ProviderRoute {
		return nil, nil, fmt.Errorf("%w: provider route identity mismatch", ErrConfiguration)
	}
	return &prepared{input: assembled, profile: profileRecord, prompt: prompt, providerRoute: providerRoute, source: current}, nil, nil
}

func (s *service) resultFromGeneration(ctx context.Context, generationRecord *domaingeneration.AIExplanationGeneration, sourceState SourceState) (*Result, error) {
	if generationRecord == nil {
		return nil, fmt.Errorf("AI explanation generation is required")
	}
	result := &Result{
		GenerationID: generationRecord.ID(), SourceReportID: generationRecord.Key().SourceReportID, SourceState: sourceState,
		CreatedAt: generationRecord.CreatedAt(), UpdatedAt: generationRecord.UpdatedAt(),
	}
	switch generationRecord.Status() {
	case domaingeneration.StatusPending:
		result.Status = StatusPending
	case domaingeneration.StatusGenerating:
		result.Status = StatusGenerating
	case domaingeneration.StatusGenerated:
		result.Status = StatusGenerated
		result.ArtifactID = generationRecord.ArtifactID()
		artifactRecord, err := s.artifacts.FindByID(ctx, generationRecord.ArtifactID())
		if err != nil {
			return nil, fmt.Errorf("load generated AI explanation artifact: %w", err)
		}
		if err := validateArtifact(generationRecord, artifactRecord); err != nil {
			return nil, err
		}
		content := artifactRecord.Content()
		result.Content = &content
	case domaingeneration.StatusFailed:
		result.Status = StatusFailed
		latest, err := s.runs.FindLatestByGenerationID(ctx, generationRecord.ID())
		if err != nil {
			return nil, fmt.Errorf("load failed AI explanation run: %w", err)
		}
		if failure := latest.Failure(); failure != nil {
			result.Failure = &Failure{Code: failure.Code, SafeMessage: failure.SafeMessage, Retryable: failure.Retryable}
		}
	default:
		return nil, fmt.Errorf("unsupported AI explanation generation status %s", generationRecord.Status())
	}
	return result, nil
}

func (s *service) sourceState(ctx context.Context, assessmentID, sourceReportID meta.ID) SourceState {
	metadataByAssessment, err := s.catalog.GetCurrentReportMetadataByAssessmentIDs(ctx, []uint64{assessmentID.Uint64()})
	if err != nil {
		return SourceStateUnknown
	}
	metadata, ok := metadataByAssessment[assessmentID.Uint64()]
	if !ok || metadata.Status != interpretationreadmodel.CurrentReportMetadataFound || metadata.SourceKind != "artifact" || metadata.SourceID == 0 {
		return SourceStateUnavailable
	}
	if metadata.SourceID == sourceReportID.Uint64() {
		return SourceStateCurrent
	}
	return SourceStateStale
}

func validateArtifact(generationRecord *domaingeneration.AIExplanationGeneration, artifactRecord *domainartifact.AIExplanationArtifact) error {
	if artifactRecord == nil || artifactRecord.GenerationID() != generationRecord.ID() || artifactRecord.ID() != generationRecord.ArtifactID() {
		return fmt.Errorf("AI explanation artifact association mismatch")
	}
	if artifactRecord.Source().ReportID != generationRecord.Key().SourceReportID || artifactRecord.Audience() != generationRecord.Key().Audience {
		return fmt.Errorf("AI explanation artifact source mismatch")
	}
	return nil
}

func validateRequest(actor Actor, input RequestInput) error {
	if actor.SubjectID == "" || actor.TesteeID == 0 || input.AssessmentID.IsZero() || input.Locale == "" || len(input.FocusAreas) > 3 {
		return fmt.Errorf("%w: participant, assessment, locale and at most three focus areas are required", ErrInvalidRequest)
	}
	return nil
}
