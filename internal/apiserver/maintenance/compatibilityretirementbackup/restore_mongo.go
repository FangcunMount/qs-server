package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"io"
	"time"
)

// RestoreMongo only uses the host's isolated database/connection. It preserves
// original server BSON (including true _id and numeric/date/binary types), all
// collection options and ordered index keys. It creates no migration ledger,
// MQ objects, business references or writer, and never closes the borrowed client.
func restoreMongo(ctx context.Context, db *mongo.Database, a *Archive) (v Verification, result error) {
	v = Verification{Database: "mongodb", StartedAt: time.Now().UTC(), SourceOriginAuthentication: "host_binding_required", ForeignKeyBusinessClosure: "not_proven", Isolation: "host_runtime_inspection_required"}
	defer func() {
		v.FinishedAt = time.Now().UTC()
		v.ElapsedMillis = v.FinishedAt.Sub(v.StartedAt).Milliseconds()
		if v.ElapsedMillis > MaxRestoreSeconds*1000 && result == nil {
			result = ErrBudget
			v.ContentEqual = false
			v.SchemaEqual = false
		}
	}()
	if db == nil || a == nil {
		return v, ErrRestore
	}
	q, c, e := boundedRestore(ctx)
	if e != nil {
		return v, e
	}
	defer c()
	if e = a.verifyAssets(q); e != nil {
		return v, e
	}
	names, e := db.ListCollectionNames(q, bson.D{})
	if e != nil || len(names) != 0 {
		return v, ErrIsolation
	}
	var hello, build bson.Raw
	if db.Client().Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || db.Client().Database("admin").RunCommand(q, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil {
		return v, ErrIdentity
	}
	version, ok := build.Lookup("version").StringValueOK()
	if !ok || len(version) < 2 || version[:2] != "7." {
		return v, ErrIdentity
	}
	processBefore, processOK := hello.Lookup("topologyVersion", "processId").ObjectIDOK()
	if !processOK || processBefore.IsZero() || processBefore.Hex() == a.data.SourceMongoProcessID || db.Name() == a.data.SourceMongoNamespace {
		return v, ErrIsolation
	}
	source := bson.Raw(a.data.Mongo.Collection)
	opts, ok := source.Lookup("options").DocumentOK()
	if !ok {
		return v, ErrStructure
	}
	create := bson.D{{Key: "create", Value: targetNames[3]}}
	elements, e := opts.Elements()
	if e != nil {
		return v, ErrStructure
	}
	for _, field := range elements {
		if field.Key() == "uuid" || field.Key() == "viewOn" || field.Key() == "pipeline" || field.Key() == "create" || field.Key() == "$db" {
			return v, ErrStructure
		}
		create = append(create, bson.E{Key: field.Key(), Value: field.Value()})
	}
	if db.RunCommand(q, create).Err() != nil {
		return v, ErrRestore
	}
	metrics, e := loadMongoRows(q, db, a)
	if e != nil {
		return v, e
	}
	v.Targets = append(v.Targets, metrics)
	indexes := bson.A{}
	for _, raw := range a.data.Mongo.Indexes {
		index := bson.Raw(raw)
		name, ok := index.Lookup("name").StringValueOK()
		if !ok {
			return v, ErrStructure
		}
		if name == "_id_" {
			continue
		}
		indexes = append(indexes, index)
	}
	if len(indexes) > 0 && db.RunCommand(q, bson.D{{Key: "createIndexes", Value: targetNames[3]}, {Key: "indexes", Value: indexes}}).Err() != nil {
		return v, ErrRestore
	}
	restored, e := ReadOrderedMongoSchema(q, db)
	if e != nil || !schemaEqual(a.data.Mongo, restored.data) {
		return v, ErrStructure
	}
	uuidKind, uuid, ok := bson.Raw(restored.data.Collection).Lookup("info", "uuid").BinaryOK()
	sourceKind, sourceUUID, sourceOK := source.Lookup("info", "uuid").BinaryOK()
	if !ok || !sourceOK || uuidKind != 4 || sourceKind != 4 || len(uuid) != 16 || len(sourceUUID) != 16 || hex.EncodeToString(uuid) == hex.EncodeToString(sourceUUID) {
		return v, ErrIsolation
	}
	process, ok := hello.Lookup("topologyVersion", "processId").ObjectIDOK()
	if !ok || process.IsZero() {
		return v, ErrIdentity
	}
	v.RestoreIdentityHash = parts("mongodb_isolated_restore_v1", process.Hex(), db.Name(), hex.EncodeToString(uuid))
	if e = verifyMongoContent(q, db, a.data.Inventory.Targets[3]); e != nil {
		return v, e
	}
	v.Targets[0].RestoredRawBytes = v.Targets[0].SourceRawBytes
	names, e = db.ListCollectionNames(q, bson.D{})
	if e != nil || len(names) != 1 || names[0] != targetNames[3] {
		return v, ErrIsolation
	}
	again, e := ReadOrderedMongoSchema(q, db)
	if e != nil || again.digest != restored.digest {
		return v, ErrStructure
	}
	if q.Err() != nil {
		return v, ErrBudget
	}
	v.ArchiveSHA256 = a.digest
	v.TargetCount = 1
	v.ContentEqual = true
	v.SchemaEqual = true
	return v, nil
}
func loadMongoRows(ctx context.Context, db *mongo.Database, a *Archive) (metrics TargetLoadMetrics, result error) {
	metrics.Database = "mongodb"
	metrics.Name = targetNames[3]
	metrics.SourceFileBytes = a.data.Assets[3].Bytes
	f, r, e := a.openSource(ctx, 3)
	if e != nil {
		return metrics, e
	}
	defer func() {
		if f.Close() != nil && result == nil {
			result = ErrPrivate
		}
	}()
	for {
		var docs []any
		size := 0
		for len(docs) < 250 && size < 8<<20 {
			row, e := r.next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return metrics, e
			}
			docs = append(docs, row.bson)
			size += len(row.bson)
		}
		if len(docs) == 0 {
			break
		}
		actual, e := db.Collection(targetNames[3]).InsertMany(ctx, docs, options.InsertMany().SetOrdered(true))
		if e != nil || actual == nil || len(actual.InsertedIDs) != len(docs) {
			return metrics, ErrRestore
		}
		metrics.RestoredRecords += uint64(len(actual.InsertedIDs))
		metrics.InsertStatements++
		if uint64(len(docs)) > metrics.MaxBatchRecords {
			metrics.MaxBatchRecords = uint64(len(docs))
		}
		if uint64(size) > metrics.MaxBatchRawBytes {
			metrics.MaxBatchRawBytes = uint64(size)
		}
		if r.done {
			break
		}
	}
	if !r.done {
		return metrics, ErrSource
	}
	metrics.SourceRecords = r.records
	metrics.SourceRawBytes = r.size
	if metrics.RestoredRecords != r.records {
		return metrics, ErrContent
	}
	return metrics, nil
}
