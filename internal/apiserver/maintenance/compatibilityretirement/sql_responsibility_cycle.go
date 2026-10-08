package retirement

import (
	"context"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
)

// SQLResponsibilitySnapshot wraps one complete host-owned RR read-only cycle.
// It authenticates current SQL observations, not editable historical DTOs. The
// coordinator must still authenticate a complete approved source copy, resolve
// original business facts, jointly cover Mongo/AI/Inbox and fence live writers.
type SQLResponsibilitySnapshot struct {
	cycle *sqlevaluation.SQLHistoricalResponsibilityCycle
}

func (*SQLResponsibilitySnapshot) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SQLResponsibilitySnapshot) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*SQLResponsibilitySnapshot) String() string {
	return "private current SQL responsibility snapshot; not retirement approval"
}
func (*SQLResponsibilitySnapshot) GoString() string {
	return "private current SQL responsibility snapshot; not retirement approval"
}

func PrepareSQLResponsibilitySnapshot(ctx context.Context, expectedIdentityHash string, limits sqlevaluation.SQLResponsibilityLimits) (*SQLResponsibilitySnapshot, error) {
	cycle, err := sqlevaluation.PrepareSQLHistoricalResponsibilityCycle(ctx, expectedIdentityHash, limits)
	if err != nil {
		return nil, err
	}
	return &SQLResponsibilitySnapshot{cycle: cycle}, nil
}

type SQLSourceResponsibilityView struct {
	AssessmentID, OrgID                                                                                        uint64
	Observations                                                                                               []sqlevaluation.SQLResponsibilityObservation
	BlockingReasons                                                                                            []string
	SourceAuthenticationRequired, OriginalBusinessFactsRequired, ExternalCoverageRequired, WriterFenceRequired bool
}

func (SQLSourceResponsibilityView) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (SQLSourceResponsibilityView) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (SQLSourceResponsibilityView) String() string               { return "private SQL responsibility view" }
func (SQLSourceResponsibilityView) GoString() string             { return "private SQL responsibility view" }

// This is a lookup, not source authentication or a new source event. There are
// no RM table reads per old row: owner and event indexes come from one cycle.
// Business point readers are deliberately separate; the original point facts
// API scans responsibilities and must not be called per source row at scale.
func (s *SQLResponsibilitySnapshot) ForUntrustedSQLSource(ctx context.Context, source *DecodedSourceEvent) (SQLSourceResponsibilityView, error) {
	if s == nil || s.cycle == nil {
		return SQLSourceResponsibilityView{}, ErrSQLOwnerResolution
	}
	id, err := sqlSourceAssessment(source)
	if err != nil {
		return SQLSourceResponsibilityView{}, err
	}
	if err := ctx.Err(); err != nil {
		return SQLSourceResponsibilityView{}, err
	}
	v := SQLSourceResponsibilityView{AssessmentID: id, OrgID: source.OrgID, SourceAuthenticationRequired: true, OriginalBusinessFactsRequired: true, ExternalCoverageRequired: true, WriterFenceRequired: true}
	seen := map[string]bool{}
	add := func(rows []sqlevaluation.SQLResponsibilityObservation) {
		for _, row := range rows {
			key := row.Store + ":" + row.PrimaryKeySHA256
			if seen[key] {
				continue
			}
			seen[key] = true
			v.Observations = append(v.Observations, row)
			if sqlResponsibilitySourceCollision(row, source) {
				v.BlockingReasons = append(v.BlockingReasons, "current_event_reuses_original_source_identity")
			}
			if row.Invalid || row.ScopeClass == "retirement_related" && (row.Unfinished || row.LeasePresent) || row.OrgID != source.OrgID || row.AssessmentID != 0 && row.AssessmentID != id {
				v.BlockingReasons = append(v.BlockingReasons, "current_sql_responsibility_unclosed_or_conflicting")
			}
			if row.OwnerUnproven || row.ScopeClass == "coordination_required" {
				v.BlockingReasons = append(v.BlockingReasons, "external_owner_or_responsibility_coverage_required")
			}
		}
	}
	add(s.cycle.ForAssessment(id))
	add(s.cycle.ForEvent(source.EventID))
	add(s.cycle.ForOrganizationActions(source.OrgID))
	return v, nil
}

func sqlResponsibilitySourceCollision(row sqlevaluation.SQLResponsibilityObservation, source *DecodedSourceEvent) bool {
	return source != nil && row.EventID == source.EventID && row.EventType != "" && (row.EventType != source.EventType || row.OwnerKind != source.AggregateType || row.OwnerID != source.AggregateID)
}

// Check the borrowed transaction at batch boundaries, not for every old row.
// Indexed lookups use the completed immutable cycle and make no SQL calls.
func (s *SQLResponsibilitySnapshot) ValidateBorrowedSnapshot(ctx context.Context) error {
	if s == nil || s.cycle == nil {
		return ErrSQLOwnerResolution
	}
	return s.cycle.ValidateBorrowedSnapshot(ctx)
}

func (s *SQLResponsibilitySnapshot) Report() sqlevaluation.SQLResponsibilityCycleReport {
	if s == nil {
		return sqlevaluation.SQLResponsibilityCycleReport{}
	}
	return s.cycle.Report()
}
func (s *SQLResponsibilitySnapshot) RecheckFresh(ctx context.Context) (sqlevaluation.SQLResponsibilityFreshReport, error) {
	if s == nil || s.cycle == nil {
		return sqlevaluation.SQLResponsibilityFreshReport{}, ErrSQLOwnerResolution
	}
	return s.cycle.RecheckFresh(ctx)
}
