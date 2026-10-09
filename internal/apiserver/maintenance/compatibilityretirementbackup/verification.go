package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"go.mongodb.org/mongo-driver/mongo"
)

type RestoreVerification struct {
	observation Verification
	verified    bool
}

func (*RestoreVerification) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*RestoreVerification) String() string {
	return "opaque actual restore verification; not retirement approval"
}
func (v *RestoreVerification) Summary() Verification {
	if v == nil || !v.verified {
		return Verification{SourceOriginAuthentication: "host_binding_required", ForeignKeyBusinessClosure: "not_proven", Isolation: "host_runtime_inspection_required"}
	}
	return v.observation
}
func RestoreSQL(ctx context.Context, conn *sql.Conn, a *Archive) (*RestoreVerification, error) {
	v, e := restoreSQL(ctx, conn, a)
	if e != nil {
		return nil, e
	}
	return &RestoreVerification{observation: v, verified: true}, nil
}
func RestoreMongo(ctx context.Context, db *mongo.Database, a *Archive) (*RestoreVerification, error) {
	// The isolated destination cannot observe the source's retained UUIDs.
	// New-profile callers must explicitly supply the real original handle.
	if a != nil && a.data.Inventory.Bindings["mongodb"].NamespaceAnchor != nil {
		return nil, ErrIdentity
	}
	v, e := restoreMongo(ctx, db, a)
	if e != nil {
		return nil, e
	}
	return &RestoreVerification{observation: v, verified: true}, nil
}

// RestoreMongoWithOriginal borrows distinct original and isolated handles. It
// validates the approved original identity/head/kept UUIDs before and after the
// isolated restore, without comparing UUIDs across databases, owning sessions,
// creating connections, or granting production writer/DDL authority.
func RestoreMongoWithOriginal(ctx context.Context, original, isolated *mongo.Database, a *Archive) (*RestoreVerification, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrBudget
	}
	if original == nil || isolated == nil || a == nil || original == isolated || original.Name() == isolated.Name() || mongo.SessionFromContext(ctx) != nil {
		return nil, ErrIdentity
	}
	q, cancel, err := boundedRestore(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if err = verifyRestoreOriginalMongo(q, original, a); err != nil {
		return nil, err
	}
	v, restoreErr := restoreMongo(q, isolated, a)
	// Always attempt the after-observation. Preserve the actual restore error
	// if both the restore and the original metadata observation fail.
	afterErr := verifyRestoreOriginalMongo(q, original, a)
	if restoreErr != nil {
		return nil, restoreErr
	}
	if afterErr != nil {
		return nil, afterErr
	}
	return &RestoreVerification{observation: v, verified: true}, nil
}

func verifyRestoreOriginalMongo(ctx context.Context, original *mongo.Database, a *Archive) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrBudget
	}
	if original == nil || a == nil || original.Name() != a.data.SourceMongoNamespace || mongo.SessionFromContext(ctx) != nil {
		return ErrIdentity
	}
	if err := a.verifyAssets(ctx); err != nil {
		return err
	}
	collections, _, err := mongoCatalog(ctx, original)
	if err != nil {
		return err
	}
	_, err = mongoState(ctx, original, a.data.Inventory.Bindings["mongodb"], collections)
	return err
}
