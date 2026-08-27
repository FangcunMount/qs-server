package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with report module bindings.
type InstallHost interface {
	compose.Host
	SetReportModule(*Module)
	AIExplanationOptions() *apiserveroptions.AIExplanationOptions
}

// InstallFrom wires and registers the report module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	module, err := Wire(WireInput{
		MySQLDB:            host.MySQLDB(),
		MySQLLimiter:       host.MySQLLimiter(),
		MongoDB:            host.MongoDB(),
		MongoLimiter:       host.MongoLimiter(),
		OpsHandle:          host.CacheHandle(redisruntime.FamilyOps),
		ReportStatusConfig: host.ReportStatusConfig(),
		OutboxProfile:      host.EventProfile(eventcatalog.OutboxProfileMongoDomain),
		RunLeaseDuration:   host.InterpretationRunLeaseDuration(),
		AIExplanation:      host.AIExplanationOptions(),
	})
	if err != nil {
		return err
	}
	host.SetReportModule(module)
	host.RegisterModule("interpretation", module)
	host.Printf("📦 Interpretation module initialized\n")
	return nil
}
