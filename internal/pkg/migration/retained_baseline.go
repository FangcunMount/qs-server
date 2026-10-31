package migration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Content is streamed and hashed in full, never returned. An exceeded budget
// fails the complete report; a partial scan never means zero or an empty table.
type RetainedObject struct {
	Kind             string  `json:"kind"`
	DefinitionSHA256 string  `json:"definition_sha256"`
	ContentSHA256    string  `json:"content_sha256,omitempty"`
	Rows             *uint64 `json:"rows,omitempty"`
	Diagnostic       bool    `json:"diagnostic,omitempty"`
	TTL              bool    `json:"ttl,omitempty"`
}
type RetainedBaseline struct {
	ReadOnly   bool                      `json:"read_only"`
	Mutation   bool                      `json:"mutation"`
	Complete   bool                      `json:"complete"`
	Phase      string                    `json:"phase"`
	ObservedAt time.Time                 `json:"observed_at"`
	Snapshot   OfflineSnapshot           `json:"snapshot"`
	Objects    map[string]RetainedObject `json:"objects"`
}

func quotedSQLIdentifier(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

func sortedDigestRows(rows [][32]byte) string {
	sort.Slice(rows, func(i, j int) bool { return bytes.Compare(rows[i][:], rows[j][:]) < 0 })
	h := sha256.New()
	for _, row := range rows {
		_, _ = h.Write(row[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func digestSQLRow(values []sql.RawBytes) [32]byte {
	h := sha256.New()
	var length [8]byte
	for _, value := range values {
		if value == nil {
			_, _ = h.Write([]byte{0})
			continue
		}
		_, _ = h.Write([]byte{1})
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = h.Write(length[:])
		_, _ = h.Write(value)
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

func scanSQLDigests(ctx context.Context, tx *sql.Tx, query string, maxRows int) (hash string, count uint64, err error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return "", 0, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	columns, err := rows.Columns()
	if err != nil {
		return "", 0, err
	}
	values := make([]sql.RawBytes, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	digests := make([][32]byte, 0)
	for rows.Next() {
		if err = rows.Scan(pointers...); err != nil {
			return "", 0, err
		}
		if len(digests) >= maxRows {
			return "", 0, errors.New("complete retained row budget exceeded")
		}
		digests = append(digests, digestSQLRow(values))
	}
	if err = rows.Err(); err != nil {
		return "", 0, err
	}
	return sortedDigestRows(digests), uint64(len(digests)), nil
}

func captureSQLRetained(ctx context.Context, db *sql.DB, database string, maxRows int, out map[string]RetainedObject) (err error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, tx.Rollback()) }()
	if err = VerifyEmptyAIPublisherDependencies(ctx, tx, database); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT table_name,table_type FROM information_schema.tables WHERE table_schema=? ORDER BY table_name", database)
	if err != nil {
		return err
	}
	type object struct{ name, kind string }
	var catalog []object
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.name, &o.kind); err != nil {
			_ = rows.Close()
			return err
		}
		catalog = append(catalog, o)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	if len(catalog) == 0 {
		return errors.New("retained SQL catalog unavailable")
	}
	for _, o := range catalog {
		if o.kind != "BASE TABLE" && o.kind != "VIEW" {
			return errors.New("unreviewed SQL object kind")
		}
		name := quotedSQLIdentifier(database) + "." + quotedSQLIdentifier(o.name)
		definition, _, e := scanSQLDigests(ctx, tx, "SHOW CREATE TABLE "+name, 1)
		if e != nil {
			return e
		}
		r := RetainedObject{Kind: o.kind, DefinitionSHA256: definition}
		if o.kind == "BASE TABLE" {
			columnRows, e := tx.QueryContext(ctx, "SELECT column_name FROM information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position", database, o.name)
			if e != nil {
				return e
			}
			var expressions []string
			for columnRows.Next() {
				var name string
				if e = columnRows.Scan(&name); e != nil {
					_ = columnRows.Close()
					return e
				}
				expressions = append(expressions, "HEX(CAST("+quotedSQLIdentifier(name)+" AS BINARY))")
			}
			if e = errors.Join(columnRows.Err(), columnRows.Close()); e != nil {
				return e
			}
			if len(expressions) == 0 {
				return errors.New("retained columns unavailable")
			}
			// Server-side BINARY/HEX avoids lossy driver datetime/decimal coercion;
			// NULL and empty values remain distinct. Every column is included.
			hash, n, e := scanSQLDigests(ctx, tx, "SELECT "+strings.Join(expressions, ",")+" FROM "+name, maxRows)
			if e != nil {
				return e
			}
			r.ContentSHA256, r.Rows = hash, &n
		}
		out["qs.mysql."+o.name] = r
	}
	return nil
}

func captureMongoRetained(ctx context.Context, documents *mongo.Database, maxRows int, out map[string]RetainedObject) error {
	cursor, err := documents.ListCollections(ctx, bson.D{})
	if err != nil {
		return err
	}
	type object struct {
		name, kind string
		raw        []byte
	}
	var catalog []object
	for cursor.Next(ctx) {
		var o struct {
			Name string `bson:"name"`
			Type string `bson:"type"`
		}
		if err = bson.Unmarshal(cursor.Current, &o); err != nil {
			_ = cursor.Close(ctx)
			return err
		}
		catalog = append(catalog, object{o.Name, o.Type, append([]byte{}, cursor.Current...)})
	}
	if err = errors.Join(cursor.Err(), cursor.Close(ctx)); err != nil {
		return err
	}
	if len(catalog) == 0 {
		return errors.New("retained Mongo catalog unavailable")
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].name < catalog[j].name })
	for _, o := range catalog {
		if o.kind != "collection" && o.kind != "view" {
			return errors.New("unreviewed Mongo object kind")
		}
		r := RetainedObject{Kind: o.kind, Diagnostic: o.name == "system.profile"}
		definitions := map[string][]byte{"collection": o.raw}
		if o.kind == "collection" {
			indexes, e := documents.Collection(o.name).Indexes().List(ctx)
			if e != nil {
				return e
			}
			for indexes.Next(ctx) {
				var index struct {
					Name string `bson:"name"`
					TTL  any    `bson:"expireAfterSeconds"`
				}
				if e = bson.Unmarshal(indexes.Current, &index); e != nil {
					_ = indexes.Close(ctx)
					return e
				}
				if _, ok := definitions["index:"+index.Name]; ok {
					_ = indexes.Close(ctx)
					return errors.New("ambiguous retained index")
				}
				definitions["index:"+index.Name] = append([]byte{}, indexes.Current...)
				r.TTL = r.TTL || index.TTL != nil
			}
			if e = errors.Join(indexes.Err(), indexes.Close(ctx)); e != nil {
				return e
			}
		}
		keys := make([]string, 0, len(definitions))
		for key := range definitions {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		h := sha256.New()
		for _, key := range keys {
			_, _ = fmt.Fprintf(h, "%d:%s:%d:", len(key), key, len(definitions[key]))
			_, _ = h.Write(definitions[key])
		}
		r.DefinitionSHA256 = hex.EncodeToString(h.Sum(nil))
		out["qs.mongo."+o.name] = r
	}
	session, err := documents.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(context.Background())
	if err = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); err != nil {
		return err
	}
	err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		for _, o := range catalog {
			key := "qs.mongo." + o.name
			r := out[key]
			if r.Diagnostic || o.kind == "view" {
				continue
			}
			rows, e := documents.Collection(o.name).Find(sc, bson.D{}, options.Find().SetBatchSize(128))
			if e != nil {
				return e
			}
			digests := make([][32]byte, 0)
			for rows.Next(sc) {
				if len(digests) >= maxRows {
					_ = rows.Close(sc)
					return errors.New("complete retained document budget exceeded")
				}
				digests = append(digests, sha256.Sum256(rows.Current))
			}
			if e = errors.Join(rows.Err(), rows.Close(sc)); e != nil {
				return e
			}
			n := uint64(len(digests))
			r.ContentSHA256, r.Rows = sortedDigestRows(digests), &n
			out[key] = r
			if r.TTL && n != 0 {
				return errors.New("nonempty business TTL collection requires separate expiry qualification")
			}
		}
		return nil
	})
	cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return errors.Join(err, session.AbortTransaction(cleanup))
}

// CaptureRetainedBaseline is a complete bounded primary read, not a backup or
// writer exclusion certificate. Two independent stores are not atomic together.
func CaptureRetainedBaseline(ctx context.Context, db *sql.DB, client *mongo.Client, cfg PairConfig, maxRows int) (out RetainedBaseline, err error) {
	out = RetainedBaseline{ReadOnly: true, ObservedAt: time.Now().UTC(), Phase: "input", Objects: map[string]RetainedObject{}}
	if maxRows < 1 || maxRows > 1000000 {
		return out, errors.New("bounded complete retained budget required")
	}
	out.Phase = "pair"
	out.Snapshot, err = InspectOffline(ctx, db, client, cfg)
	if err != nil {
		return out, err
	}
	out.Phase = "mysql"
	if err = captureSQLRetained(ctx, db, cfg.MySQLDatabase, maxRows, out.Objects); err != nil {
		return out, err
	}
	out.Phase = "mongo"
	if err = captureMongoRetained(ctx, client.Database(cfg.MongoDatabase), maxRows, out.Objects); err != nil {
		return out, err
	}
	out.Phase = "complete"
	out.Complete = true
	return out, nil
}
