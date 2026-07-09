package plan

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

// InstallHost extends the shared compose seam with plan module bindings.
type InstallHost interface {
	compose.Host
	SurveyScaleInfra() *surveymod.ScaleInfra
	SetPlanModule(*Module)
}

// InstallFrom wires and registers the plan module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	var assessmentModelRepo modelcatalogport.ModelRepository
	if infra := host.SurveyScaleInfra(); infra != nil {
		assessmentModelRepo = infra.AssessmentModelRepo
	}
	module, err := Wire(WireInput{
		MySQLDB:             host.MySQLDB(),
		EventPublisher:      host.EventPublisher(),
		AssessmentModelRepo: assessmentModelRepo,
		RedisClient:         host.CacheClient(cacheplane.FamilyObject),
		CacheBuilder:        host.CacheBuilder(cacheplane.FamilyObject),
		PlanPolicy:          host.CachePolicy(cachepolicy.PolicyPlan),
		EntryBaseURL:        host.PlanEntryBaseURL(),
		Observer:            host.CacheObserver(),
		MySQLLimiter:        host.MySQLLimiter(),
		TesteeAccess:        host.ActorPorts().TesteeAccess,
	})
	if err != nil {
		return err
	}
	host.SetPlanModule(module)
	host.RegisterModule("plan", module)
	host.Printf("📦 Plan module initialized\n")
	return nil
}
