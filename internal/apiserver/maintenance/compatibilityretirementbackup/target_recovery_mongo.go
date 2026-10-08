package compatibilityretirementbackup

import (
	"context"
	"encoding/hex"
	"go.mongodb.org/mongo-driver/bson"
)

func (p *TargetRecoveryPlan) recoverMongo(ctx context.Context) error {
	if e := p.checkBases(ctx, true); e != nil {
		return e
	}
	source := bson.Raw(p.archive.data.Mongo.Collection)
	opts, ok := source.Lookup("options").DocumentOK()
	if !ok {
		return ErrStructure
	}
	fields, e := opts.Elements()
	if e != nil {
		return ErrStructure
	}
	create := bson.D{{Key: "create", Value: targetNames[3]}}
	for _, field := range fields {
		switch field.Key() {
		case "uuid", "viewOn", "pipeline", "create", "$db":
			return ErrStructure
		}
		create = append(create, bson.E{Key: field.Key(), Value: field.Value()})
	}
	raw, e := bson.Marshal(create)
	if e != nil {
		return ErrStructure
	}
	statement := string(raw)
	if _, e = p.journal.write("3-create-intent", targetRecord(p, 3, "create", statement, "intent", 0)); e != nil {
		return e
	}
	if p.borrowed.Mongo.RunCommand(ctx, create).Err() != nil {
		_, _ = p.journal.write("3-create-result", targetRecord(p, 3, "create", statement, "unknown", 0))
		return ErrRecoveryUnknown
	}
	if _, e = p.journal.write("3-create-result", targetRecord(p, 3, "create", statement, "native_success", 0)); e != nil {
		return e
	}
	if _, e = p.journal.write("3-load-intent", targetRecord(p, 3, "load", "ordered original BSON InsertMany", "intent", 0)); e != nil {
		return e
	}
	metrics, e := loadMongoRows(ctx, p.borrowed.Mongo, p.archive)
	if e != nil {
		_, _ = p.journal.write("3-load-result", targetRecord(p, 3, "load", "ordered original BSON InsertMany", "unknown", metrics.RestoredRecords))
		return ErrRecoveryUnknown
	}
	if _, e = p.journal.write("3-load-result", targetRecord(p, 3, "load", "ordered original BSON InsertMany", "native_success", metrics.RestoredRecords)); e != nil {
		return e
	}
	indexes := bson.A{}
	for _, raw := range p.archive.data.Mongo.Indexes {
		index := bson.Raw(raw)
		name, ok := index.Lookup("name").StringValueOK()
		if !ok {
			return ErrStructure
		}
		if name != "_id_" {
			indexes = append(indexes, index)
		}
	}
	if len(indexes) > 0 {
		command := bson.D{{Key: "createIndexes", Value: targetNames[3]}, {Key: "indexes", Value: indexes}}
		wire, e := bson.Marshal(command)
		if e != nil {
			return ErrStructure
		}
		if _, e = p.journal.write("3-index-intent", targetRecord(p, 3, "index", string(wire), "intent", 0)); e != nil {
			return e
		}
		if p.borrowed.Mongo.RunCommand(ctx, command).Err() != nil {
			_, _ = p.journal.write("3-index-result", targetRecord(p, 3, "index", string(wire), "unknown", 0))
			return ErrRecoveryUnknown
		}
		if _, e = p.journal.write("3-index-result", targetRecord(p, 3, "index", string(wire), "native_success", 0)); e != nil {
			return e
		}
	}
	restored, e := ReadOrderedMongoSchema(ctx, p.borrowed.Mongo)
	if e != nil || !schemaEqual(restored.data, p.archive.data.Mongo) {
		return ErrStructure
	}
	kind, id, ok := bson.Raw(restored.data.Collection).Lookup("info", "uuid").BinaryOK()
	oldKind, old, valid := source.Lookup("info", "uuid").BinaryOK()
	if !ok || !valid || kind != 4 || oldKind != 4 || len(id) != 16 || len(old) != 16 || hex.EncodeToString(id) == hex.EncodeToString(old) {
		return ErrIdentity
	}
	p.mongoTargetUUID = hex.EncodeToString(id)
	present, e := p.checkTarget(ctx, 3)
	if e != nil || !present {
		return ErrContent
	}
	if _, e = p.journal.write("3-verify-result", targetRecord(p, 3, "verify", "full original ordered schema and BSON readback", "equal", metrics.RestoredRecords)); e != nil {
		return e
	}
	p.observations[3].State = "restored_exact"
	return nil
}
