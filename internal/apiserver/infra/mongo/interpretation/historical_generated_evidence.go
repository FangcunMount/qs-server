package interpretation

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type HistoricalGeneratedBaseline struct {
	retirementBaseline
	history *retirementevidence.HistoricalSetSnapshot
	payload eventoutcome.ReportGeneratedPayload
}

func (b *HistoricalGeneratedBaseline) Payload() eventoutcome.ReportGeneratedPayload {
	if b == nil {
		return eventoutcome.ReportGeneratedPayload{}
	}
	copy := &GeneratedRetirementEvidenceBaseline{payload: b.payload}
	return copy.Payload()
}

// HistoricalGeneratedPayload binds an explicitly selected immutable original
// report/run, never replacing its identities with today's generation winner.
// The caller must independently prove the current generation responsibility is
// closed and the original artifact/run association is unique.
func HistoricalGeneratedPayload(g ReportGenerationPO, a InterpretReportPO, r InterpretationRunPO, fact *evaluationfact.Record) (eventoutcome.ReportGeneratedPayload, error) {
	if fact == nil || g.DomainID.IsZero() || g.Status != "generated" || g.Version == 0 || a.DomainID.IsZero() || r.DomainID.IsZero() || a.GenerationID != g.DomainID.Uint64() || a.OutcomeID != g.OutcomeID || a.InterpretationRunID != r.DomainID.Uint64() || r.GenerationID != g.DomainID.Uint64() || r.Status != "succeeded" || r.Attempt < 1 || a.OrgID != fact.OrgID() || a.AssessmentID != fact.AssessmentID().Uint64() || a.TesteeID != fact.TesteeID() || a.ReportType != g.ReportType || a.TemplateVersion != g.TemplateVersion || a.Model == nil || fact.ID().Uint64() != g.OutcomeID {
		return eventoutcome.ReportGeneratedPayload{}, fmt.Errorf("%w: original generated graph is incomplete or conflicting", retirementevidence.ErrUnverifiable)
	}
	p := eventoutcome.ReportGeneratedPayload{OrgID: a.OrgID, GenerationID: g.DomainID.String(), RunID: r.DomainID.String(), ReportID: a.DomainID.String(), AssessmentID: strconv.FormatUint(a.AssessmentID, 10), OutcomeID: strconv.FormatUint(a.OutcomeID, 10), TesteeID: a.TesteeID, Attempt: uint(r.Attempt), ReportType: a.ReportType, TemplateVersion: a.TemplateVersion, BuilderIdentity: a.BuilderIdentity, ContentSchemaVersion: a.ContentSchemaVersion, GeneratedAt: a.GeneratedAt, Model: eventoutcome.ModelIdentity{Kind: a.Model.Kind, Algorithm: a.Model.Algorithm, Code: a.Model.Code, Version: a.Model.Version, Title: a.Model.Title}}
	if v := a.PrimaryScore; v != nil {
		p.PrimaryScore = &eventoutcome.ScoreValue{Kind: v.Kind, Value: v.Value, Label: v.Label, Max: v.Max}
	}
	if v := a.Level; v != nil {
		p.Level = &eventoutcome.ResultLevel{Code: v.Code, Label: v.Label, Severity: v.Severity}
	}
	if _, err := eventevidencebinding.Generated(p); err != nil {
		return eventoutcome.ReportGeneratedPayload{}, fmt.Errorf("%w: %v", retirementevidence.ErrUnverifiable, err)
	}
	return p, nil
}

func (s *RetirementEvidenceStore) PrepareHistoricalGenerated(ctx context.Context, generationID, reportID, runID uint64) (*HistoricalGeneratedBaseline, error) {
	if s == nil || s.db == nil || generationID == 0 || reportID == 0 || runID == 0 {
		return nil, retirementevidence.ErrUnverifiable
	}
	if err := RequireHistoricalArtifactUniqueness(ctx, s.db.Collection("interpret_report_artifacts")); err != nil {
		return nil, err
	}
	history, err := retirementevidence.ReadHistoricalSet(ctx, s.db.Collection("report_generations"), bson.M{"domain_id": generationID, "deleted_at": nil}, "historical_generated_evidence")
	if err != nil {
		return nil, err
	}
	var g ReportGenerationPO
	if err := history.Decode(&g); err != nil {
		return nil, err
	}
	if g.DomainID.Uint64() != generationID || g.Status != "generated" || g.ReportID == 0 || g.LatestRunID == 0 || g.Version == 0 {
		return nil, retirementevidence.ErrUnverifiable
	}
	fact, err := s.outcome(ctx, g.OutcomeID)
	if err != nil {
		return nil, err
	}
	dependencies := []retirementDependency{}
	readGraph := func(selectedReport, selectedRun uint64) (InterpretReportPO, InterpretationRunPO, error) {
		var a InterpretReportPO
		artifact, err := s.read(ctx, "interpret_report_artifacts", selectedReport, "", &a)
		if err != nil {
			return a, InterpretationRunPO{}, err
		}
		var r InterpretationRunPO
		run, err := s.read(ctx, "interpretation_runs", selectedRun, "", &r)
		if err != nil {
			return a, r, err
		}
		if a.DomainID.Uint64() != selectedReport || r.DomainID.Uint64() != selectedRun {
			return a, r, retirementevidence.ErrUnverifiable
		}
		dependencies = append(dependencies, retirementDependency{artifact, "interpret_report_artifacts"}, retirementDependency{run, "interpretation_runs"})
		return a, r, nil
	}
	currentArtifact, currentRun, err := readGraph(g.ReportID, g.LatestRunID)
	if err != nil {
		return nil, err
	}
	if _, err := HistoricalGeneratedPayload(g, currentArtifact, currentRun, fact); err != nil {
		return nil, err
	}
	a, r := currentArtifact, currentRun
	if reportID != g.ReportID || runID != g.LatestRunID {
		a, r, err = readGraph(reportID, runID)
		if err != nil {
			return nil, err
		}
	}
	// An ambiguous artifact graph is not resolved by choosing a first result.
	cursor, err := s.db.Collection("interpret_report_artifacts").Find(ctx, bson.M{"generation_id": generationID, "interpretation_run_id": runID, "deleted_at": nil}, options.Find().SetLimit(2).SetProjection(bson.M{"domain_id": 1}))
	if err != nil {
		return nil, err
	}
	var matches []struct {
		ID uint64 `bson:"domain_id"`
	}
	if err := cursor.All(ctx, &matches); err != nil {
		return nil, err
	}
	if len(matches) != 1 || matches[0].ID != reportID {
		return nil, fmt.Errorf("%w: original report graph is ambiguous", retirementevidence.ErrUnverifiable)
	}
	payload, err := HistoricalGeneratedPayload(g, a, r, fact)
	if err != nil {
		return nil, err
	}
	binding, err := eventevidencebinding.Generated(payload)
	if err != nil {
		return nil, err
	}
	return &HistoricalGeneratedBaseline{retirementBaseline: retirementBaseline{dependencies: dependencies, binding: binding, outcomeID: g.OutcomeID, outcomeIdentity: retirementOutcomeIdentity(fact)}, history: history, payload: payload}, nil
}

// RequireHistoricalArtifactUniqueness proves the deployed immutable-artifact
// contract instead of relying on an unlocked first-result query. Constructors
// and the original lifecycle migration own this existing index; maintenance
// does not silently create or repair it.
func RequireHistoricalArtifactUniqueness(ctx context.Context, collection *mongo.Collection) error {
	if collection == nil {
		return retirementevidence.ErrUnverifiable
	}
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}
	var indexes []struct {
		Key     bson.D `bson:"key"`
		Unique  bool   `bson:"unique"`
		Partial bson.D `bson:"partialFilterExpression"`
		Sparse  bool   `bson:"sparse"`
	}
	if err := cursor.All(ctx, &indexes); err != nil {
		return err
	}
	for _, index := range indexes {
		if index.Unique && !index.Sparse && len(index.Partial) == 0 && len(index.Key) == 1 && index.Key[0].Key == "generation_id" && (index.Key[0].Value == int32(1) || index.Key[0].Value == int64(1)) {
			return nil
		}
	}
	return fmt.Errorf("%w: immutable original artifact uniqueness is not proved", retirementevidence.ErrUnverifiable)
}

func (s *RetirementEvidenceStore) BackfillHistoricalGenerated(ctx context.Context, baseline *HistoricalGeneratedBaseline, entry evidence.HistoricalReferenceEntryV1) error {
	if baseline == nil {
		return retirementevidence.ErrUnverifiable
	}
	if err := entry.Validate(); err != nil {
		return err
	}
	if entry.EventType != eventcatalog.InterpretationReportGenerated || entry.Proof.BusinessBindingSHA256 != baseline.binding || entry.Run == nil || entry.Run.RunID != baseline.payload.RunID || entry.Run.Attempt != baseline.payload.Attempt {
		return retirementevidence.ErrConflict
	}
	if err := s.recheck(ctx, "report_generations", &baseline.retirementBaseline); err != nil {
		return err
	}
	return baseline.history.Append(ctx, s.db.Collection("report_generations"), entry)
}
