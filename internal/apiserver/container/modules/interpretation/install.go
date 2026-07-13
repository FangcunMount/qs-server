package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with report module bindings.
type InstallHost interface {
	compose.Host
	SetReportModule(*Module)
}

// InstallFrom wires and registers the report module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	module, err := Wire(WireInput{
		MongoDB:            host.MongoDB(),
		MongoLimiter:       host.MongoLimiter(),
		OpsHandle:          host.CacheHandle(redisruntime.FamilyOps),
		ReportStatusConfig: host.ReportStatusConfig(),
		OutboxProfile:      host.EventProfile(eventcatalog.OutboxProfileMongoDomain),
	})
	if err != nil {
		return err
	}
	host.SetReportModule(module)
	host.RegisterModule("interpretation", module)
	host.Printf("📦 Interpretation module initialized\n")
	return nil
}
