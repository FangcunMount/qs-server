package plan

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
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
	var scaleRepo scaledefinition.Repository
	if infra := host.SurveyScaleInfra(); infra != nil {
		scaleRepo = infra.ScaleRepo
	}
	module, err := Wire(WireInput{
		MySQLDB:        host.MySQLDB(),
		EventPublisher: host.EventPublisher(),
		ScaleRepo:      scaleRepo,
		RedisClient:    host.CacheClient(cacheplane.FamilyObject),
		CacheBuilder:   host.CacheBuilder(cacheplane.FamilyObject),
		PlanPolicy:     host.CachePolicy(cachepolicy.PolicyPlan),
		EntryBaseURL:   host.PlanEntryBaseURL(),
		Observer:       host.CacheObserver(),
		MySQLLimiter:   host.MySQLLimiter(),
		TesteeAccess:   host.ActorPorts().TesteeAccess,
	})
	if err != nil {
		return err
	}
	host.SetPlanModule(module)
	host.RegisterModule("plan", module)
	host.Printf("📦 Plan module initialized\n")
	return nil
}
