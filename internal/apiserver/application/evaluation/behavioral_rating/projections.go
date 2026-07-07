package behavioralrating

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/projection"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
)

// ApplyFactorProjections enriches a raw scale outcome with composite rollup and Brief-2 norm projection.
func ApplyFactorProjections(
	outcome *assessment.AssessmentOutcome,
	snapshot *behavioralsnapshot.Snapshot,
	subject brief2norm.Subject,
) *assessment.AssessmentOutcome {
	if outcome == nil || snapshot == nil {
		return outcome
	}
	outcome = projection.CompositeProjection{Factors: snapshot.Factors}.Apply(outcome)
	outcome = EnrichBrief2Outcome(outcome, snapshot, subject)
	return projection.HierarchyProjection{Factors: snapshot.Factors}.Apply(outcome)
}
