package retirement

import (
	"bytes"
	"context"
	"io"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// FreshRecheckAnchor retains only a validated original epoch's immutable facts
// and opaque full-copy authentication. It never retains the old SQL/Mongo row
// graphs, closes a borrowed scope, or proves that the host has ended that scope.
// There is deliberately no constructor accepting a report or editable DTO.
type FreshRecheckAnchor struct {
	binding                                                        *OriginCopyBinding
	sqlConnection                                                  any
	sqlCycleID, sqlIdentity, sqlHead, mongoIdentity, mongoMetadata string
	transaction                                                    mongoCycleTxn
	receipts                                                       [4]SourceCopyReceipt
	boundaries                                                     [4]SourceBoundary
	originalEpochHash, seal                                        string
}

func (*FreshRecheckAnchor) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*FreshRecheckAnchor) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*FreshRecheckAnchor) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*FreshRecheckAnchor) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (*FreshRecheckAnchor) String() string {
	return "private original origin recheck anchor; host lifecycle and approval unproven"
}
func (a *FreshRecheckAnchor) GoString() string { return a.String() }

func (a *FreshRecheckAnchor) factsDigest() (string, error) {
	if a == nil || a.binding == nil {
		return "", ErrSourceOrigin
	}
	return sourceOriginDigest(struct {
		Binding, SQLCycle, SQLIdentity, SQLHead, MongoIdentity, MongoMetadata, MongoTransaction, OriginalEpoch string
		Receipts                                                                                               [4]SourceCopyReceipt
		Boundaries                                                                                             [4]SourceBoundary
	}{a.binding.hash, a.sqlCycleID, a.sqlIdentity, a.sqlHead, a.mongoIdentity, a.mongoMetadata, mongoOwnerHashParts("actual-mongo-session-txn/v1", string(a.transaction.session), originMongoNumber(a.transaction.number)), a.originalEpochHash, a.receipts, a.boundaries})
}

func (a *FreshRecheckAnchor) intact() bool {
	if a == nil || a.binding == nil || a.sqlConnection == nil || a.sqlCycleID == "" || a.sqlIdentity == "" || a.sqlHead == "" || a.mongoIdentity == "" || a.mongoMetadata == "" || len(a.transaction.session) == 0 || a.originalEpochHash == "" || a.seal == "" {
		return false
	}
	digest, err := a.factsDigest()
	return err == nil && digest == a.seal
}

// FreezeFreshRecheckAnchor requires the original real borrowed SQL RRRO/Mongo
// snapshot to still be active. All original metadata/transaction checks run
// before any graphless anchor is issued. The caller may then release its graph
// references; transaction ending remains exclusively the host's responsibility.
func (e *SourceOriginEpoch) FreezeFreshRecheckAnchor(ctx context.Context) (*FreshRecheckAnchor, error) {
	if e == nil || e.hash == "" {
		return nil, ErrSourceOrigin
	}
	if err := e.validate(ctx); err != nil {
		return nil, err
	}
	a := &FreshRecheckAnchor{binding: e.binding, sqlConnection: e.sqlConnection, sqlCycleID: e.sqlCycleID, sqlIdentity: e.sqlIdentity, sqlHead: e.sqlHead, mongoIdentity: e.mongoIdentity, mongoMetadata: e.mongoMetadata, transaction: mongoCycleTxn{session: bytes.Clone(e.transaction.session), number: e.transaction.number}, receipts: e.receipts, boundaries: e.boundaries, originalEpochHash: e.hash}
	var err error
	a.seal, err = a.factsDigest()
	if err != nil || !a.intact() || a.binding.alive(ctx) != nil {
		return nil, ErrSourceOrigin
	}
	return a, nil
}

// Recheck consumes real new host snapshots, then repeats the complete physical
// copy, metadata and original under-upper source scans. Summary/hash equality
// alone can never create this proof. Unverified approval/fence/CAS gaps remain.
func (a *FreshRecheckAnchor) Recheck(ctx context.Context, sql *sqlevaluation.SQLHistoricalResponsibilityCycle, mgo *MongoResponsibilitySnapshot, readers []io.Reader) (*SourceOriginRecheckProof, error) {
	if !a.intact() || a.binding.alive(ctx) != nil || sql == nil || mgo == nil {
		return nil, ErrSourceOrigin
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx.Statement == nil || tx.Statement.ConnPool == a.sqlConnection || sql.Report().CycleID == a.sqlCycleID || (bytes.Equal(mgo.txn.session, a.transaction.session) && mgo.txn.number == a.transaction.number) {
		return nil, ErrSourceOriginFresh
	}
	fresh, err := PrepareSourceOriginEpoch(ctx, a.binding, sql, mgo, readers)
	if err != nil {
		return nil, err
	}
	if fresh.sqlIdentity != a.sqlIdentity || fresh.sqlHead != a.sqlHead || fresh.mongoIdentity != a.mongoIdentity || fresh.mongoMetadata != a.mongoMetadata || fresh.boundaries != a.boundaries || fresh.receipts != a.receipts || !a.intact() {
		return nil, ErrSourceOrigin
	}
	return &SourceOriginRecheckProof{firstAnchor: a, second: fresh}, nil
}

func (a *FreshRecheckAnchor) RecheckSnapshots(ctx context.Context, sql *SQLResponsibilitySnapshot, mongo *MongoResponsibilitySnapshot, readers []io.Reader) (*SourceOriginRecheckProof, error) {
	if sql == nil || sql.cycle == nil {
		return nil, ErrSourceOrigin
	}
	return a.Recheck(ctx, sql.cycle, mongo, readers)
}
