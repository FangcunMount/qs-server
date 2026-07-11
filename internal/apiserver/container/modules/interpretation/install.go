package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

// InstallHost extends the shared compose seam with report module bindings.
type InstallHost interface {
	compose.Host
	SetReportModule(*Module)
}

// InstallFrom wires and registers the report module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	catalog, err := host.DefaultEvaluationCatalog()
	if err != nil {
		return err
	}
	module, err := Wire(WireInput{
		MongoDB:            host.MongoDB(),
		TopicResolver:      host.TopicResolver(),
		MongoLimiter:       host.MongoLimiter(),
		OpsHandle:          host.CacheHandle(cacheplane.FamilyOps),
		ModelDescriptors:   catalog.Descriptors,
		ReportStatusConfig: host.ReportStatusConfig(),
	})
	if err != nil {
		return err
	}
	host.SetReportModule(module)
	host.RegisterModule("interpretation", module)
	host.Printf("📦 Interpretation module initialized\n")
	return nil
}
