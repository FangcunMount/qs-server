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
	v, e := restoreMongo(ctx, db, a)
	if e != nil {
		return nil, e
	}
	return &RestoreVerification{observation: v, verified: true}, nil
}
