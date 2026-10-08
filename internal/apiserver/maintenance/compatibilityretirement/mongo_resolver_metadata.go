package retirement

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	sheetmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	interpretmongo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"reflect"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type mongoOwnerMetadata struct {
	identity        string
	collections     map[string]string
	head            bson.Raw
	artifactIndexes []bson.Raw
}

func mongoOwnerHashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		var frame [9]byte
		frame[0] = 1
		binary.BigEndian.PutUint64(frame[1:], uint64(len(part)))
		_, _ = h.Write(frame[:])
		_, _ = h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Mongo metadata commands cannot run inside a transaction. Strip session values
// without losing cancellation, and use the exact borrowed client/database.
func mongoMetadataContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	stop := context.AfterFunc(parent, cancel)
	if deadline, ok := parent.Deadline(); ok {
		if delay := time.Until(deadline); delay <= 0 {
			cancel()
		} else {
			timer := time.AfterFunc(delay, cancel)
			return ctx, func() { stop(); timer.Stop(); cancel() }
		}
	}
	return ctx, func() { stop(); cancel() }
}

func observeMongoOwnerMetadata(parent context.Context, db *mongo.Database, config MongoOwnerConfig) (mongoOwnerMetadata, error) {
	var result mongoOwnerMetadata
	ctx, cancel := mongoMetadataContext(parent)
	defer cancel()
	var hello bson.Raw
	if err := db.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return result, ErrMongoOwnerRead
	}
	fields, err := exactBSONFields(hello)
	if err != nil {
		return result, ErrMongoOwnerIdentity
	}
	stable := bson.D{}
	for _, key := range []string{"setName", "hosts", "me"} {
		if raw, ok := fields[key]; ok {
			var value any
			if err := raw.Unmarshal(&value); err != nil {
				return result, ErrMongoOwnerIdentity
			}
			stable = append(stable, bson.E{Key: key, Value: value})
		}
	}
	if name, ok := fields["setName"]; !ok || name.Type != bson.TypeString || name.StringValue() == "" {
		return result, ErrMongoOwnerIdentity
	}
	encoded, err := json.Marshal(stable)
	if err != nil {
		return result, ErrMongoOwnerIdentity
	}
	result.collections = map[string]string{}
	names := []string{"schema_migrations", "answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts", "rm_outbox", "qs_rm_replay_requests"}
	for _, name := range names {
		result.collections[name] = "absent"
	}
	cursor, err := db.ListCollections(ctx, bson.D{{Key: "name", Value: bson.D{{Key: "$in", Value: names}}}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if err != nil {
		return result, ErrMongoOwnerRead
	}
	var rows []struct {
		Name string `bson:"name"`
		Type string `bson:"type"`
		Info struct {
			UUID primitive.Binary `bson:"uuid"`
		} `bson:"info"`
	}
	if err = cursor.All(ctx, &rows); err != nil {
		return result, ErrMongoOwnerRead
	}
	for _, row := range rows {
		if _, ok := result.collections[row.Name]; !ok || result.collections[row.Name] != "absent" || row.Type != "collection" || row.Info.UUID.Subtype != 4 || len(row.Info.UUID.Data) != 16 {
			return result, ErrMongoOwnerIdentity
		}
		result.collections[row.Name] = hex.EncodeToString(row.Info.UUID.Data)
	}
	if result.collections["schema_migrations"] == "absent" {
		return result, ErrMongoOwnerIdentity
	}
	result.identity = mongoOwnerHashParts("mongodb_database_identity_v1", string(encoded), db.Name(), result.collections["schema_migrations"])
	if result.identity != config.ExpectedIdentityHash {
		return result, ErrMongoOwnerIdentity
	}
	cursor, err = db.Collection("schema_migrations").Find(ctx, bson.D{}, options.Find().SetLimit(2).SetCollation(&options.Collation{Locale: "simple"}))
	if err != nil {
		return result, ErrMongoOwnerRead
	}
	var heads []bson.Raw
	if err = cursor.All(ctx, &heads); err != nil {
		return result, ErrMongoOwnerRead
	}
	if len(heads) != 1 {
		return result, ErrMongoOwnerIdentity
	}
	hf, err := exactBSONFields(heads[0])
	if err != nil {
		return result, ErrMongoOwnerIdentity
	}
	version, ok := mongoExactInteger(hf["version"])
	dirty, dirtyOK := hf["dirty"].BooleanOK()
	if !ok || version != config.ExpectedMigrationVersion || !dirtyOK || dirty {
		return result, ErrMongoOwnerIdentity
	}
	result.head = append(bson.Raw(nil), heads[0]...)
	return result, nil
}

func mongoExactInteger(v bson.RawValue) (int64, bool) {
	switch v.Type {
	case bson.TypeInt64:
		return v.Int64(), true
	case bson.TypeInt32:
		return int64(v.Int32()), true
	}
	return 0, false
}

func (r *MongoOwnerResolution) readRows(ctx context.Context, name string, filter bson.D) ([]bson.Raw, error) {
	cursor, err := r.db.Collection(name).Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(MongoOwnerRowLimit+1).SetMaxTime(15*time.Second).SetCollation(&options.Collation{Locale: "simple"}))
	if err != nil {
		return nil, ErrMongoOwnerRead
	}
	var rows []bson.Raw
	if err = cursor.All(ctx, &rows); err != nil {
		return nil, ErrMongoOwnerRead
	}
	if len(rows) > MongoOwnerRowLimit {
		return nil, ErrMongoOwnerBounds
	}
	for i, raw := range rows {
		if err := mongoUniqueBSON(raw, 0); err != nil {
			return nil, err
		}
		var typ reflect.Type
		switch name {
		case "answersheets":
			typ = reflect.TypeOf(sheetmongo.AnswerSheetPO{})
		case "report_generations":
			typ = reflect.TypeOf(interpretmongo.ReportGenerationPO{})
		case "interpretation_runs":
			typ = reflect.TypeOf(interpretmongo.InterpretationRunPO{})
		case "interpret_report_artifacts":
			typ = reflect.TypeOf(interpretmongo.InterpretReportPO{})
		}
		if typ != nil {
			if err := mongoPOShape(raw, typ); err != nil {
				return nil, err
			}
			fields, _ := exactBSONFields(raw)
			n, ok := mongoExactInteger(fields["domain_id"])
			if !ok || n <= 0 {
				return nil, ErrMongoOwnerResolution
			}
		}
		r.bytes += len(raw)
		if r.bytes > MongoOwnerByteLimit {
			return nil, ErrMongoOwnerBounds
		}
		rows[i] = append(bson.Raw(nil), raw...)
	}
	cloned, err := bson.Marshal(filter)
	if err != nil {
		return nil, ErrMongoOwnerResolution
	}
	var sealed bson.D
	if bson.Unmarshal(cloned, &sealed) != nil {
		return nil, ErrMongoOwnerResolution
	}
	r.reads = append(r.reads, mongoOwnerRead{collection: name, filter: sealed, rows: rows})
	return rows, nil
}

func (r *MongoOwnerResolution) readOne(ctx context.Context, name string, id uint64, target any) (bson.Raw, error) {
	if id == 0 || id > 1<<63-1 || r.metadata.collections[name] == "absent" {
		return nil, ErrMongoOwnerResolution
	}
	rows, err := r.readRows(ctx, name, bson.D{{Key: "domain_id", Value: int64(id)}})
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, ErrMongoOwnerResolution
	}
	f, _ := exactBSONFields(rows[0])
	n, ok := mongoExactInteger(f["domain_id"])
	if !ok || n <= 0 || uint64(n) != id {
		return nil, ErrMongoOwnerResolution
	}
	if v, exists := f["deleted_at"]; exists && v.Type != bson.TypeNull {
		return nil, ErrMongoOwnerResolution
	}
	if err = mongoPOShape(rows[0], reflect.TypeOf(target).Elem()); err != nil {
		return nil, err
	}
	if err = bson.Unmarshal(rows[0], target); err != nil {
		return nil, ErrMongoOwnerResolution
	}
	return rows[0], nil
}

// PO shape is checked before decoding: BSON coercions and unknown business
// fields must not disappear into a seemingly valid projected owner.
func mongoPOShape(raw bson.Raw, t reflect.Type) error {
	fields, err := exactBSONFields(raw)
	if err != nil {
		return ErrMongoOwnerResolution
	}
	allowed := map[string]reflect.Type{}
	var collect func(reflect.Type)
	collect = func(typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			tag := f.Tag.Get("bson")
			parts := strings.Split(tag, ",")
			if parts[0] == "-" {
				continue
			}
			if len(parts) > 1 && parts[1] == "inline" {
				collect(f.Type)
				continue
			}
			if parts[0] != "" {
				allowed[parts[0]] = f.Type
			}
		}
	}
	collect(t)
	for key, value := range fields {
		typ, ok := allowed[key]
		if !ok {
			return ErrMongoOwnerResolution
		}
		if err := mongoPOValue(value, typ); err != nil {
			return err
		}
	}
	return nil
}

func mongoPOValue(value bson.RawValue, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		if value.Type == bson.TypeNull {
			return nil
		}
		return mongoPOValue(value, t.Elem())
	}
	if t == reflect.TypeOf(time.Time{}) {
		if value.Type != bson.TypeDateTime {
			return ErrMongoOwnerResolution
		}
		return nil
	}
	if t == reflect.TypeOf(primitive.ObjectID{}) {
		if value.Type != bson.TypeObjectID {
			return ErrMongoOwnerResolution
		}
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		if value.Type != bson.TypeEmbeddedDocument {
			return ErrMongoOwnerResolution
		}
		return mongoPOShape(value.Document(), t)
	case reflect.String:
		if value.Type != bson.TypeString {
			return ErrMongoOwnerResolution
		}
	case reflect.Bool:
		if value.Type != bson.TypeBoolean {
			return ErrMongoOwnerResolution
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := mongoExactInteger(value)
		if !ok || (t.Kind() >= reflect.Uint && t.Kind() <= reflect.Uint64 && n < 0) {
			return ErrMongoOwnerResolution
		}
	case reflect.Float32, reflect.Float64:
		if value.Type != bson.TypeDouble {
			n, ok := mongoExactInteger(value)
			if !ok || n > 1<<53 || n < -(1<<53) {
				return ErrMongoOwnerResolution
			}
		}
	case reflect.Slice:
		if value.Type == bson.TypeNull {
			return nil
		}
		if t.Elem().Kind() == reflect.Uint8 {
			if value.Type != bson.TypeBinary {
				return ErrMongoOwnerResolution
			}
			return nil
		}
		if value.Type != bson.TypeArray {
			return ErrMongoOwnerResolution
		}
		items, err := value.Array().Values()
		if err != nil {
			return ErrMongoOwnerResolution
		}
		for _, item := range items {
			if err = mongoPOValue(item, t.Elem()); err != nil {
				return err
			}
		}
	case reflect.Interface, reflect.Map: // Explicit PO extension maps stay in the private full-BSON baseline.
	default:
		return ErrMongoOwnerResolution
	}
	return nil
}

func mongoUniqueBSON(raw []byte, depth int) error {
	if depth > 32 {
		return ErrMongoOwnerBounds
	}
	fields, err := exactBSONFields(raw)
	if err != nil {
		return ErrMongoOwnerResolution
	}
	for _, value := range fields {
		switch value.Type {
		case bson.TypeEmbeddedDocument, bson.TypeArray:
			if err = mongoUniqueBSON(value.Value, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
