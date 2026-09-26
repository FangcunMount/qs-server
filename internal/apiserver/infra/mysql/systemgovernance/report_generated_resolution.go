package systemgovernance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/attentionprojection"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReportGeneratedResolutionReader interface {
	interpretationreadmodel.BatchReportMetadataReader
	GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error)
}

// NewReportGeneratedResolutionVerifier proves the durable effects of exactly
// one interpretation.report.generated event. It never publishes, repairs, or
// grants resolution rights to another event type. The Mongo reader must return
// metadata from a catalog/source association that has already passed its
// fail-closed consistency check.
func NewReportGeneratedResolutionVerifier(reports ReportGeneratedResolutionReader) DeliveryResolutionVerifier {
	return func(ctx context.Context, tx *gorm.DB, subject DeliveryResolutionSubject) (DeliveryResolutionEvidence, error) {
		if reports == nil || tx == nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("report and MySQL evidence readers are required")
		}
		env, err := eventcodec.DecodeEnvelope([]byte(subject.PayloadJSON))
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("decode report event: %w", err)
		}
		if env.ID != subject.EventID || env.EventType != eventcatalog.InterpretationReportGenerated {
			return DeliveryResolutionEvidence{}, fmt.Errorf("report event identity or type changed")
		}
		var data eventoutcome.ReportGeneratedPayload
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("decode report event data: %w", err)
		}
		assessmentID, err := parseResolutionID(data.AssessmentID)
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("invalid report assessment ID: %w", err)
		}
		reportID, err := parseResolutionID(data.ReportID)
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("invalid report ID: %w", err)
		}
		outcomeID, err := parseResolutionID(data.OutcomeID)
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("invalid report outcome ID: %w", err)
		}
		generationID, err := parseResolutionID(data.GenerationID)
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("invalid report generation ID: %w", err)
		}
		runID, err := parseResolutionID(data.RunID)
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("invalid report run ID: %w", err)
		}
		if data.OrgID != subject.OrgID || data.TesteeID == 0 || env.AggregateID != data.GenerationID {
			return DeliveryResolutionEvidence{}, fmt.Errorf("report event organization, subject, or aggregate mismatch")
		}

		// Hold the assessment state stable until the resolution transaction
		// commits. A current report alone would not make report-status return
		// completed if the authoritative assessment were not evaluated.
		var assessment struct {
			OrgID    int64  `gorm:"column:org_id"`
			TesteeID uint64 `gorm:"column:testee_id"`
			Status   string `gorm:"column:status"`
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("assessment").
			Select("org_id", "testee_id", "status").
			Where("id = ? AND deleted_at IS NULL", assessmentID).Take(&assessment).Error; err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("lock assessment fact: %w", err)
		}
		if assessment.OrgID != subject.OrgID || assessment.TesteeID != data.TesteeID || assessment.Status != "evaluated" {
			return DeliveryResolutionEvidence{}, fmt.Errorf("assessment fact does not prove report visibility")
		}

		ledger, err := attentionprojection.NewMySQLStore(tx)
		if err != nil {
			return DeliveryResolutionEvidence{}, err
		}
		record, err := ledger.GetByEventID(ctx, subject.EventID)
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("read attention projection evidence: %w", err)
		}
		if record.Status != attentionprojection.StatusSucceeded || record.ReportID != data.ReportID ||
			record.AssessmentID != data.AssessmentID || record.TesteeID != data.TesteeID ||
			record.RiskLevel != eventoutcome.AttentionRiskLevel(data.Level) ||
			record.MarkKeyFocus != eventoutcome.LevelIsHighRisk(data.Level) {
			return DeliveryResolutionEvidence{}, fmt.Errorf("attention projection does not prove the original event effect")
		}

		// Bound the cross-store read while the MySQL rows are locked. Mongo
		// catalog/source identity may later advance to a newer report; this
		// checks the current user-visible report at the resolution decision.
		readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		metadata, err := reports.GetCurrentReportMetadataByAssessmentIDs(readCtx, []uint64{assessmentID})
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("read current report fact: %w", err)
		}
		fact, ok := metadata[assessmentID]
		if !ok || fact.Status != interpretationreadmodel.CurrentReportMetadataFound ||
			fact.SourceKind != "artifact" || fact.SourceID != reportID ||
			fact.OrgID != subject.OrgID || fact.TesteeID != data.TesteeID ||
			fact.OutcomeID != outcomeID || fact.GenerationID != generationID || fact.RunID != runID {
			return DeliveryResolutionEvidence{}, fmt.Errorf("current report fact does not match the original event")
		}
		row, err := reports.GetReportByAssessmentID(readCtx, assessmentID)
		if err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("read participant report: %w", err)
		}
		if row == nil || row.ReportID != reportID || row.AssessmentID != assessmentID {
			return DeliveryResolutionEvidence{}, fmt.Errorf("participant report no longer matches the original event")
		}
		if (row.Level == nil) != (data.Level == nil) ||
			(row.Level != nil && (row.Level.Code != data.Level.Code || row.Level.Label != data.Level.Label || row.Level.Severity != data.Level.Severity)) {
			return DeliveryResolutionEvidence{}, fmt.Errorf("participant report level differs from the original event")
		}
		if _, err := (reportprojection.Mapper{}).FromRow(readCtx, *row, policy.AudienceParticipant); err != nil {
			return DeliveryResolutionEvidence{}, fmt.Errorf("participant report cannot be projected: %w", err)
		}
		return DeliveryResolutionEvidence{
			Kind:                "report_generated_current_fact_and_attention",
			Reference:           fmt.Sprintf("event:%s/report:%d/assessment:%d", subject.EventID, reportID, assessmentID),
			AllEffectsConfirmed: true,
		}, nil
	}
}

func parseResolutionID(raw string) (uint64, error) {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("nonzero decimal ID required")
	}
	return id, nil
}
