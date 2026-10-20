package eventtransaction

import (
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

const owner = "qs.mysql"
const revision = "m3-1"

func freeze(q port.Request, resolver catalog.TopicResolver) (transaction.FrozenPlan, error) {
	return shared.Freeze(q, resolver, owner, revision)
}

// Artifact retains the original private-plan codec and original consumer wire.
func Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return shared.Artifact(p, owner, revision)
}
