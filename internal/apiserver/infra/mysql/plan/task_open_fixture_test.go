package plan_test

import (
	"testing"
	"time"

	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	mysqlplan "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Opening contracts exercise current Plan and Enrollment admission against the
// same disposable MySQL as the Task CAS and reminder transaction.
func persistOpeningScope(t *testing.T, db *gorm.DB, task *domainplan.AssessmentTask) {
	t.Helper()
	parent, err := domainplan.NewAssessmentPlan(task.GetOrgID(), task.GetScaleCode(), domainplan.PlanScheduleByDay, 1, 1)
	require.NoError(t, err)
	parent.RestoreFromRepository(task.GetPlanID(), domainplan.PlanStatusActive)
	require.NoError(t, mysqlplan.NewPlanRepository(db).Save(t.Context(), parent))
	enrollment := domainplan.NewEnrollment(task.GetOrgID(), parent.GetID(), task.GetTesteeID(), 1, task.GetPlannedAt(), time.Now())
	require.NoError(t, mysqlplan.NewEnrollmentRepository(db).Save(t.Context(), enrollment))
	task.AssignEnrollment(enrollment.ID())
}
