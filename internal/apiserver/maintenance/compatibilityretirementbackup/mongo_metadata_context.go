package compatibilityretirementbackup

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)

// Mongo does not permit listCollections/listIndexes and administrative
// topology commands in a transaction. The pinned driver's public context
// constructor masks only its own session value when supplied nil; it keeps
// the original deadline, cancellation and all other host binding values.
// No session implementation, client or transaction is created or ended here.
// Call this only for metadata. Bodies and the actual migration head retain
// their caller's original snapshot context and are checked separately.
func mongoMetadataContext(ctx context.Context) context.Context {
	if mongo.SessionFromContext(ctx) == nil {
		return ctx
	}
	return mongo.NewSessionContext(ctx, nil)
}
