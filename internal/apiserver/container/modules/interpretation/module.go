package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
)

const Name = modules.PackageInterpretation

// TransactionOwner exposes the borrowed authority for read-only host audits.
// The module does not own its Start/Close lifecycle.
func (m *Module) TransactionOwner() *guarded.Owner {
	if m == nil {
		return nil
	}
	return m.transactionOwner
}
