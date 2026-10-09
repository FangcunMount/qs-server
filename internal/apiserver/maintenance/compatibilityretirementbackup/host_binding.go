package compatibilityretirementbackup

import (
	"context"
	"reflect"
)

// HostObjectBinding carries only the original object's identity/count/digests.
// It is expected material, never a backup or retirement completion assertion.
type HostObjectBinding struct {
	Database, Name, Kind, IdentityHash, SchemaHash, DataHash string
	Records                                                  uint64
}

// VerifyHostArchiveBinding compares the fixed host's independently approved
// input with the actual opaque archive and rereads every registered asset to
// EOF. It neither exports original bodies nor grants a DROP capability.
func VerifyHostArchiveBinding(ctx context.Context, archive *Archive, expected Approval) error {
	if ctx == nil || ctx.Err() != nil || archive == nil || !approvalValid(expected) {
		return ErrApproval
	}
	if archive.data.Approval != expected || archive.data.Inventory.SourceSHA != expected.SourceSHA ||
		archive.data.Inventory.OperationID != expected.OperationID || archive.data.Inventory.RunID != expected.RunID ||
		archive.data.Inventory.RequestHash != expected.RequestHash ||
		archive.data.SQLMetadataHash != expected.SQLMetadataSHA256 ||
		archive.data.MongoMetadataHash != expected.MongoMetadataSHA256 ||
		archive.data.OrderedMongoSchemaHash != expected.OrderedMongoSchemaSHA256 ||
		sha(archive.data.InventoryRaw) != expected.InventorySHA256 {
		return ErrApproval
	}
	return archive.verifyAssets(ctx)
}

// VerifyHostArchiveObjects binds the frozen retirement manifest to the archive
// inventory. Object identity hashes differ from database identity hashes.
func VerifyHostArchiveObjects(ctx context.Context, archive *Archive, expected [4]HostObjectBinding, sqlHead, mongoHead uint64, sqlInventoryNonTarget, sqlRecoveryNonTarget, mongoNonTarget string) error {
	if ctx == nil || ctx.Err() != nil || archive == nil || len(archive.data.Inventory.Targets) != 4 || len(archive.data.Inventory.Bindings) != 2 || !hashPattern.MatchString(sqlInventoryNonTarget) || !hashPattern.MatchString(sqlRecoveryNonTarget) || !hashPattern.MatchString(mongoNonTarget) {
		return ErrApproval
	}
	if archive.data.Inventory.Bindings["mysql"].Version != sqlHead || archive.data.Inventory.Bindings["mongodb"].Version != mongoHead ||
		archive.data.Inventory.Bindings["mysql"].NonTargetHash != sqlInventoryNonTarget || archive.data.Inventory.Bindings["mongodb"].NonTargetHash != mongoNonTarget ||
		archive.data.SQLRecoveryNonTargetHash != sqlRecoveryNonTarget {
		return ErrApproval
	}
	for i, want := range expected {
		actual := archive.data.Inventory.Targets[i]
		if want.Database != actual.Database || want.Name != actual.Name || want.Kind != actual.Kind || want.IdentityHash != actual.IdentityHash ||
			want.SchemaHash != actual.SchemaHash || want.DataHash != actual.DataHash || want.Records != actual.Records {
			return ErrApproval
		}
	}
	return nil // The host then calls VerifyHostArchiveBinding for one joint EOF pass.
}

// HostArchiveRecoveryBaseline returns the actual Capture-derived projection,
// for preparation output only. This hash conveys neither execution authority
// nor evidence that source, writers or restored databases are quiescent.
func HostArchiveRecoveryBaseline(a *Archive) (string, error) {
	if a == nil || !hashPattern.MatchString(a.data.SQLRecoveryNonTargetHash) {
		return "", ErrApproval
	}
	return a.data.SQLRecoveryNonTargetHash, nil
}

// VerifyHostOriginalSources observes actual current originals with host-owned
// read handles. It does not close the SQL epoch or manufacture a writer fence.
func VerifyHostOriginalSources(ctx context.Context, a *Archive, borrowed BorrowedSources) error {
	if ctx == nil || ctx.Err() != nil || a == nil || len(a.data.Inventory.Targets) != 4 || len(a.data.Inventory.Bindings) != 2 || borrowed.SQL == nil || borrowed.Mongo == nil {
		return ErrApproval
	}
	if e := a.verifyAssets(ctx); e != nil {
		return e
	}
	for pass := 0; pass < 2; pass++ {
		defs, e := readSQLCatalog(ctx, borrowed.SQL)
		if e != nil || jsonSHA(defs) != a.data.Inventory.Bindings["mysql"].CatalogHash {
			return ErrStructure
		}
		if _, e = sqlState(ctx, borrowed.SQL, a.data.Inventory.Bindings["mysql"]); e != nil {
			return e
		}
		for i := 0; i < 3; i++ {
			structure, e := sqlStructure(ctx, borrowed.SQL, i)
			if e != nil || !reflect.DeepEqual(structure, a.data.SQL[i]) {
				return ErrStructure
			}
			if e = verifySQLContent(ctx, borrowed.SQL, a.data.SQL[i], a.data.Inventory.Targets[i], i); e != nil {
				return e
			}
		}
		collections, mdefs, e := mongoCatalog(ctx, borrowed.Mongo)
		if e != nil || jsonSHA(mdefs) != a.data.Inventory.Bindings["mongodb"].CatalogHash {
			return ErrStructure
		}
		if _, e = mongoState(ctx, borrowed.Mongo, a.data.Inventory.Bindings["mongodb"], collections); e != nil {
			return e
		}
		ordered, e := ReadOrderedMongoSchema(ctx, borrowed.Mongo)
		if e != nil || ordered.digest != a.data.OrderedMongoSchemaHash {
			return ErrStructure
		}
		if e = verifyMongoContent(ctx, borrowed.Mongo, a.data.Inventory.Targets[3]); e != nil {
			return e
		}
	}
	return nil
}
