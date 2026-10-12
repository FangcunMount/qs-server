package interpretation

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

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
