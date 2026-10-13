package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	planentry "github.com/FangcunMount/qs-server/internal/apiserver/port/planentry"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
)

// Only fixed dependency names reach startup logs. The returned coded error keeps
// its public safe message, while the diagnostic formatter retains the detail.
func missingPlanDependency(component, dependency string) error {
	logger.L(context.Background()).Errorw("required Plan dependency is missing", "component", component, "dependency", dependency)
	return errors.WithCode(code.ErrModuleInitializationFailed, "%s: %s is required", component, dependency)
}

func validateTaskServiceDependencies(component string, tasks domainplan.AssessmentTaskRepository, plans domainplan.AssessmentPlanRepository, enrollments domainplan.EnrollmentRepository, tx apptransaction.Runner, entry planentry.Generator, publisher event.EventPublisher) error {
	switch {
	case tasks == nil:
		return missingPlanDependency(component, "task repository")
	case plans == nil:
		return missingPlanDependency(component, "plan repository")
	case enrollments == nil:
		return missingPlanDependency(component, "enrollment repository")
	case tx == nil:
		return missingPlanDependency(component, "transaction runner")
	case entry == nil:
		return missingPlanDependency(component, "entry generator")
	case publisher == nil:
		return missingPlanDependency(component, "event publisher")
	}
	return nil
}
