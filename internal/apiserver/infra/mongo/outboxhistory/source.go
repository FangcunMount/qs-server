// Package outboxhistory selects the original Mongo Outbox evidence for one
// bounded read. It does not create, write to, or change either collection.
package outboxhistory

import (
	"bytes"
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	Original = "rm_outbox"
	Archive  = "qs_rm_outbox_history"
)

var (
	ErrMissing  = errors.New("historical MongoDB outbox collections are unavailable")
	errIdentity = errors.New("historical MongoDB outbox collection identity changed or is invalid")
)

type source struct {
	name string
	uuid primitive.Binary
}

// Reader is owned by a single operation. The parent context must be outside
// the data snapshot: MongoDB forbids listCollections inside a transaction.
type Reader struct {
	ctx      context.Context
	db       *mongo.Database
	resolved bool
	source   source
	err      error
}

func NewReader(ctx context.Context, db *mongo.Database) *Reader {
	return &Reader{ctx: ctx, db: db}
}

// Collection resolves lazily. A v2-only path never reads legacy metadata, and
// a missing message in the selected collection never triggers another source.
func (r *Reader) Collection() (*mongo.Collection, error) {
	if !r.resolved {
		r.source, r.err = r.resolve()
		r.resolved = true
	}
	if r.err != nil {
		return nil, r.err
	}
	return r.db.Collection(r.source.name, options.Collection().SetReadPreference(readpref.Primary())), nil
}

func (r *Reader) Verify() error {
	if !r.resolved || r.err != nil {
		return nil
	}
	after, err := r.resolve()
	if err != nil {
		return err
	}
	if after.name != r.source.name || !bytes.Equal(after.uuid.Data, r.source.uuid.Data) {
		return errIdentity
	}
	return nil
}

func (r *Reader) resolve() (source, error) {
	if mongo.SessionFromContext(r.ctx) != nil {
		return source{}, errIdentity
	}
	db := r.db.Client().Database(r.db.Name(), options.Database().SetReadPreference(readpref.Primary()))
	cursor, err := db.ListCollections(r.ctx, bson.M{"name": bson.M{"$in": bson.A{Original, Archive}}}, options.ListCollections().SetNameOnly(false))
	if err != nil {
		return source{}, err
	}
	defer func() { _ = cursor.Close(r.ctx) }()
	var original, archive source
	for cursor.Next(r.ctx) {
		var meta struct {
			Name string `bson:"name"`
			Type string `bson:"type"`
			Info struct {
				UUID primitive.Binary `bson:"uuid"`
			} `bson:"info"`
		}
		if err := cursor.Decode(&meta); err != nil {
			return source{}, err
		}
		var dst *source
		switch meta.Name {
		case Original:
			dst = &original
		case Archive:
			dst = &archive
		default:
			return source{}, errIdentity
		}
		if dst.name != "" || meta.Type != "collection" || meta.Info.UUID.Subtype != 4 || len(meta.Info.UUID.Data) != 16 {
			return source{}, errIdentity
		}
		*dst = source{meta.Name, meta.Info.UUID}
	}
	if err := cursor.Err(); err != nil {
		return source{}, err
	}
	if original.name != "" {
		return original, nil
	}
	if archive.name != "" {
		return archive, nil
	}
	return source{}, ErrMissing
}
