package messagingruntime

import (
	"bytes"
	"context"
	"errors"
	"time"

	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
)

type MongoNativeLimits struct {
	db        *mongo.Database
	authority *txmongo.Store
	binding   transaction.Binding
	revision  string
}

func NewMongoNativeLimits(db *mongo.Database, b transaction.Binding, revision string) (*MongoNativeLimits, error) {
	if db == nil || db.Client() == nil || db.Name() == "" || b.Owner != "qs.mongo" || !nativeNoticeBinding(b, revision) {
		return nil, transaction.ErrBinding
	}
	authority, err := txmongo.New(db, b)
	if err != nil {
		return nil, err
	}
	primary := db.Client().Database(db.Name(), options.Database().SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority()).SetWriteConcern(writeconcern.Majority()))
	return &MongoNativeLimits{db: primary, authority: authority, binding: b, revision: revision}, nil
}
func (s *MongoNativeLimits) Binding() transaction.Binding {
	if s == nil {
		return transaction.Binding{}
	}
	return s.binding
}

type nativeNoticeDocument struct {
	Key        []byte     `bson:"_id"`
	Owner      string     `bson:"owner"`
	StoreID    string     `bson:"store_id"`
	Epoch      string     `bson:"writer_epoch"`
	Revision   string     `bson:"route_revision"`
	Index      []byte     `bson:"native_index_sha256"`
	Evidence   []byte     `bson:"evidence_sha256"`
	Status     string     `bson:"status"`
	Schema     int        `bson:"schema_version"`
	ResolvedAt *time.Time `bson:"resolved_at"`
	Receipt    []byte     `bson:"resolution_receipt_sha256"`
}

func (s *MongoNativeLimits) validate(row nativeNoticeDocument) ([32]byte, error) {
	if row.Schema != 1 {
		return [32]byte{}, transaction.ErrSchema
	}
	if row.Owner != s.binding.Owner || row.StoreID != s.binding.StoreID || row.Epoch != s.binding.WriterEpoch || row.Revision != s.revision || len(row.Key) != 32 || len(row.Index) != 32 || len(row.Evidence) != 32 {
		return [32]byte{}, transaction.ErrBinding
	}
	var evidence [32]byte
	copy(evidence[:], row.Evidence)
	var index [32]byte
	copy(index[:], row.Index)
	key := nativeNoticeKey(s.binding, s.revision, index)
	if index == [32]byte{} || evidence == [32]byte{} || !bytes.Equal(row.Key, key[:]) || !nativeNoticeResolutionValid(row.Status, row.ResolvedAt, row.Receipt) {
		return [32]byte{}, transaction.ErrBinding
	}
	return evidence, nil
}
func (s *MongoNativeLimits) RecordNativeLimit(ctx context.Context, e NativeLimitEvidence) error {
	if s == nil || s.db == nil || s.authority == nil {
		return transaction.ErrBinding
	}
	if !e.valid() {
		return ErrNativeLimitEvidence
	}
	if mongo.SessionFromContext(ctx) != nil {
		return transaction.ErrTransactionRequired
	}
	recordedAt := time.Now().UTC()
	ctx, cancel := nativeNoticeBudget(ctx)
	defer cancel()
	if err := s.validateSchema(ctx); err != nil {
		return err
	}
	collection := s.db.Collection("messaging_native_limit_notices")
	models := make([]mongo.WriteModel, 0, len(e.indices))
	keys := make(bson.A, 0, len(e.indices))
	expected := map[[32]byte]bool{}
	for _, index := range e.indices {
		key := nativeNoticeKey(s.binding, s.revision, index)
		keys = append(keys, key[:])
		expected[key] = true
		document := bson.M{"_id": key[:], "owner": s.binding.Owner, "store_id": s.binding.StoreID, "writer_epoch": s.binding.WriterEpoch, "route_revision": s.revision, "native_index_sha256": index[:], "evidence_sha256": e.digest[:], "status": "blocked", "schema_version": 1, "recorded_at": recordedAt}
		models = append(models, mongo.NewUpdateOneModel().SetFilter(bson.M{"_id": key[:]}).SetUpdate(bson.M{"$setOnInsert": document}).SetUpsert(true))
	}
	if _, err := collection.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(true)); err != nil {
		return err
	}
	cursor, err := collection.Find(ctx, bson.M{"_id": bson.M{"$in": keys}}, options.Find().SetLimit(int64(len(keys))+1))
	if err != nil {
		return err
	}
	verify := func() error {
		for cursor.Next(ctx) {
			var row nativeNoticeDocument
			if err := cursor.Decode(&row); err != nil {
				return err
			}
			if _, err := s.validate(row); err != nil {
				return err
			}
			var key [32]byte
			copy(key[:], row.Key)
			if !expected[key] {
				return transaction.ErrBinding
			}
			delete(expected, key)
		}
		if err := cursor.Err(); err != nil {
			return err
		}
		if len(expected) != 0 {
			return transaction.ErrBinding
		}
		return nil
	}
	return errors.Join(verify(), cursor.Close(ctx))
}

func (s *MongoNativeLimits) ReadNativeLimit(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil || s.authority == nil {
		return false, transaction.ErrBinding
	}
	if mongo.SessionFromContext(ctx) != nil {
		return false, transaction.ErrTransactionRequired
	}
	ctx, cancel := nativeNoticeBudget(ctx)
	defer cancel()
	if err := s.validateSchema(ctx); err != nil {
		return false, err
	}
	query := bson.M{"owner": s.binding.Owner, "status": bson.M{"$ne": "resolved"}}
	var actual nativeNoticeDocument
	err := s.db.Collection("messaging_native_limit_notices").FindOne(ctx, query).Decode(&actual)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err = s.validate(actual); err != nil {
		return false, err
	}
	if actual.Status != "blocked" {
		return false, transaction.ErrBinding
	}
	return true, nil
}

func (s *MongoNativeLimits) validateSchema(ctx context.Context) (result error) {
	if err := s.authority.Validate(ctx); err != nil {
		return err
	}
	var schema struct {
		Version int `bson:"version"`
	}
	if err := s.db.Collection("messaging_native_limit_schema").FindOne(ctx, bson.M{"_id": "v1"}).Decode(&schema); err != nil {
		return errors.Join(transaction.ErrSchema, err)
	}
	if schema.Version != 1 {
		return transaction.ErrSchema
	}
	cursor, err := s.db.ListCollections(ctx, bson.M{"name": "messaging_native_limit_notices"})
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, cursor.Close(ctx)) }()
	if !cursor.Next(ctx) {
		if err = cursor.Err(); err != nil {
			return err
		}
		return transaction.ErrSchema
	}
	var info struct {
		Options struct {
			Collation struct {
				Locale string `bson:"locale"`
			} `bson:"collation"`
		} `bson:"options"`
	}
	if err = cursor.Decode(&info); err != nil {
		return err
	}
	if info.Options.Collation.Locale != "" && info.Options.Collation.Locale != "simple" {
		return transaction.ErrSchema
	}
	return cursor.Err()
}
