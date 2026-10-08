package mongoconsistency

import (
	"context"
	"errors"
	"reflect"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func checkLegacySubmission(row sheetmongo.AnswerSheetPO) error {
	if row.LegacySubmissionEvidence == nil {
		return driftf("legacy conclusion is missing")
	}
	if err := row.LegacySubmissionEvidence.Validate(); err != nil {
		return conflict(err)
	}
	payload, err := sheetmongo.LegacySubmissionPayload(row)
	if err != nil {
		return conflict(err)
	}
	binding, err := eventevidencebinding.AnswerSheet(payload)
	if err != nil {
		return conflict(err)
	}
	for _, entry := range row.LegacySubmissionEvidence.Entries {
		if entry.EventType != eventcatalog.AnswerSheetSubmitted || entry.Run != nil || entry.Proof.BusinessBindingSHA256 != binding {
			return driftf("legacy submission reference disagrees with frozen business facts")
		}
	}
	return nil
}

func (s *Scanner) checkHistoricalSlot(ctx context.Context, collection string, id uint64, slot string, want *evidence.HistoricalReferenceSetV1) error {
	var raw bson.Raw
	if err := s.db.Collection(collection).FindOne(ctx, bson.M{"domain_id": id, "deleted_at": nil}, options.FindOne().SetProjection(bson.M{slot: 1})).Decode(&raw); err != nil {
		return err
	}
	actual, err := retirementevidence.DecodeHistoricalSetDocument(raw, slot)
	if err != nil {
		return conflict(err)
	}
	if !reflect.DeepEqual(actual, want) {
		return driftf("historical conclusion changed after the bounded observation")
	}
	return nil
}

// Historical graph checks do not query rm_outbox, perform a full message
// fingerprint check, or substitute the latest report for an original run.
func (s *Scanner) checkHistoricalGenerated(ctx context.Context, g interpretmongo.ReportGenerationPO) error {
	if g.HistoricalGeneratedEvidence == nil {
		return driftf("historical generated conclusion is missing")
	}
	if err := g.HistoricalGeneratedEvidence.Validate(); err != nil {
		return conflict(err)
	}
	if err := s.checkHistoricalSlot(ctx, "report_generations", g.DomainID.Uint64(), "historical_generated_evidence", g.HistoricalGeneratedEvidence); err != nil {
		return err
	}
	if err := interpretmongo.RequireHistoricalArtifactUniqueness(ctx, s.db.Collection("interpret_report_artifacts")); err != nil {
		if errors.Is(err, retirementevidence.ErrUnverifiable) {
			return conflict(err)
		}
		return err
	}
	if _, err := s.generatedPayload(ctx, g); err != nil {
		return err
	}
	fact, err := s.outcome(ctx, g.OutcomeID)
	if err != nil {
		return err
	}
	for _, entry := range g.HistoricalGeneratedEvidence.Entries {
		if entry.EventType != eventcatalog.InterpretationReportGenerated || entry.Run == nil {
			return driftf("historical generated source has no original run")
		}
		runID, err := strconv.ParseUint(entry.Run.RunID, 10, 64)
		if err != nil || runID == 0 || strconv.FormatUint(runID, 10) != entry.Run.RunID {
			return driftf("historical run identity is invalid")
		}
		var run interpretmongo.InterpretationRunPO
		if err := s.db.Collection("interpretation_runs").FindOne(ctx, bson.M{"domain_id": runID, "deleted_at": nil}, options.FindOne().SetProjection(runAuditProjection)).Decode(&run); err != nil {
			return err
		}
		cursor, err := s.db.Collection("interpret_report_artifacts").Find(ctx, bson.M{"generation_id": g.DomainID.Uint64(), "interpretation_run_id": runID, "deleted_at": nil}, options.Find().SetProjection(artifactAuditProjection).SetLimit(2))
		if err != nil {
			return err
		}
		var artifacts []interpretmongo.InterpretReportPO
		if err := cursor.All(ctx, &artifacts); err != nil {
			return err
		}
		if len(artifacts) != 1 || run.Attempt < 1 || uint(run.Attempt) != entry.Run.Attempt {
			return driftf("original historical artifact graph is absent or ambiguous")
		}
		payload, err := interpretmongo.HistoricalGeneratedPayload(g, artifacts[0], run, fact)
		if err != nil {
			return conflict(err)
		}
		binding, err := eventevidencebinding.Generated(payload)
		if err != nil {
			return conflict(err)
		}
		if entry.Proof.BusinessBindingSHA256 != binding {
			return driftf("original historical report binding disagrees")
		}
	}
	return nil
}
