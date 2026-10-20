package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
)

// InstallHost extends the shared compose seam with report module bindings.
type InstallHost interface {
	compose.Host
	SetReportModule(*Module)
	AIWorkflowOptions() *apiserveroptions.AIWorkflowOptions
}

// CommandTransactionInstallHost supplies the same qs.mysql owner used by the
// other MySQL domains. The module does not create/start/close another owner.
type CommandTransactionInstallHost interface {
	AICommandTransactionOwner() *mysqlEventTx.Committer
}

// InstallFrom wires and registers the report module using composition-root host inputs.
func InstallFrom(host InstallHost) error {
	var commandOwner *mysqlEventTx.Committer
	if transactions, ok := host.(CommandTransactionInstallHost); ok {
		commandOwner = transactions.AICommandTransactionOwner()
	}
	module, err := Wire(WireInput{
		CommandTransactionOwner: commandOwner,
		MySQLDB:                 host.MySQLDB(),
		MySQLLimiter:            host.MySQLLimiter(),
		MongoDB:                 host.MongoDB(),
		MongoLimiter:            host.MongoLimiter(),
		OpsHandle:               host.CacheHandle(redisruntime.FamilyOps),
		ReportStatusConfig:      host.ReportStatusConfig(),
		OutboxProfile:           host.EventProfile(eventcatalog.OutboxProfileMongoDomain),
		RunLeaseDuration:        host.InterpretationRunLeaseDuration(),
		AIWorkflow:              host.AIWorkflowOptions(),
	})
	if err != nil {
		return err
	}
	host.SetReportModule(module)
	host.RegisterModule("interpretation", module)
	host.Printf("📦 Interpretation module initialized\n")
	return nil
}
