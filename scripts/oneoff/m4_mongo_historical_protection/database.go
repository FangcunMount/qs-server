package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const queryLimit = 5 * time.Second

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type sqlReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
type database struct {
	conn                     *sql.Conn
	mongo                    *mongo.Client
	sqlName, mongoName       string
	cursorCount              int
	cursorReleaseUnproven    bool
	sqlRollbackUnproven      bool
	sessionEndCalled         bool
	ledgerCommitIssued       bool
	ledgerCommitAcknowledged bool
	ledgerCommitUnknown      bool
	ledgerAbortCalled        bool
}

func (d *database) sqlState(ctx context.Context, r sqlReader) (string, uint64, bool, error) {
	q, c := context.WithTimeout(ctx, queryLimit)
	defer c()
	var uuid, name, v string
	if r.QueryRowContext(q, "SELECT @@server_uuid,DATABASE(),VERSION()").Scan(&uuid, &name, &v) != nil || !uuidRE.MatchString(uuid) || name != d.sqlName || v != "8.0.36" {
		return "", 0, false, errors.New("mysql_identity_rejected")
	}
	raw, _ := json.Marshal([]any{"mysql_server_selected_v1", uuid, name})
	hash := fingerprint(raw)
	rows, e := r.QueryContext(q, "SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2")
	if e != nil {
		return "", 0, false, errors.New("sql_ledger_read_failed")
	}
	var version uint64
	var dirty bool
	count := 0
	for rows.Next() {
		if rows.Scan(&version, &dirty) != nil {
			_ = rows.Close()
			return "", 0, false, errors.New("sql_ledger_invalid")
		}
		count++
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || count != 1 {
		return "", 0, false, errors.New("sql_ledger_or_release_invalid")
	}
	if version != 101 || dirty {
		return "", 0, false, errors.New("selected_clean101_required")
	}
	return hash, version, dirty, nil
}

func (d *database) mongoIdentity(ctx context.Context) (string, error) {
	q, c := context.WithTimeout(ctx, queryLimit)
	defer c()
	var hello struct {
		Set     string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
		Primary bool     `bson:"isWritablePrimary"`
		Message string   `bson:"msg"`
	}
	if d.mongo.Database("admin").RunCommand(q, bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || hello.Set == "" || !hello.Primary || hello.Message != "" || len(hello.Hosts) == 0 {
		return "", errors.New("mongo_primary_replica_identity_unknown")
	}
	sort.Strings(hello.Hosts)
	for i, h := range hello.Hosts {
		if h == "" || i > 0 && h == hello.Hosts[i-1] {
			return "", errors.New("mongo_replica_identity_invalid")
		}
	}
	q2, c2 := context.WithTimeout(ctx, queryLimit)
	defer c2()
	var build struct {
		Version string `bson:"version"`
	}
	if d.mongo.Database("admin").RunCommand(q2, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil || build.Version != "7.0.31" {
		return "", errors.New("mongo_same_patch_required")
	}
	raw, _ := json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: hello.Set}, {Key: "hosts", Value: hello.Hosts}}, d.mongoName})
	return fingerprint(raw), nil
}

// Full EOF with ID zero is the normal release proof. After an interrupted
// cursor we attempt bounded cleanup, but Close's nil return alone does not
// prove every server-side cursor was killed.
func (d *database) finishCursor(cur *mongo.Cursor, readErr error) error {
	d.cursorCount++
	live := cur.ID() != 0
	q, c := context.WithTimeout(context.Background(), queryLimit)
	defer c()
	ce := cur.Close(q)
	if ce != nil || live {
		d.cursorReleaseUnproven = true
		return errors.New("mongo_cursor_release_unproven")
	}
	if readErr != nil {
		return errors.New("mongo_cursor_read_failed")
	}
	return nil
}
func (d *database) catalog(ctx context.Context) ([3]namespaceProof, error) {
	var result [3]namespaceProof
	names := []string{"answersheets", "report_generations", "schema_migrations"}
	q, c := context.WithTimeout(ctx, queryLimit)
	cur, e := d.mongo.Database(d.mongoName).ListCollections(q, bson.D{{Key: "name", Value: bson.D{{Key: "$in", Value: names}}}}, options.ListCollections().SetBatchSize(4))
	if e != nil {
		c()
		return result, errors.New("namespace_catalog_read_failed")
	}
	seen := map[string]bool{}
	invalid := false
	for cur.Next(q) {
		r := cur.Current
		name, ok := r.Lookup("name").StringValueOK()
		kind, kok := r.Lookup("type").StringValueOK()
		uuidKind, uuid, uok := r.Lookup("info", "uuid").BinaryOK()
		opts, ook := r.Lookup("options").DocumentOK()
		position := -1
		for i, n := range names {
			if n == name {
				position = i
			}
		}
		if !ok || !kok || kind != "collection" || !uok || uuidKind != 4 || len(uuid) != 16 || !ook || position < 0 || seen[name] {
			invalid = true
			continue
		}
		seen[name] = true
		result[position] = namespaceProof{Name: name, UUID: append([]byte(nil), uuid...), Options: append([]byte(nil), opts...)}
	}
	re := cur.Err()
	ce := d.finishCursor(cur, re)
	c()
	if ce != nil {
		return result, ce
	}
	if invalid || len(seen) != 3 {
		return result, errors.New("fixed_namespace_catalog_invalid")
	}
	for i, name := range names {
		q, c = context.WithTimeout(ctx, queryLimit)
		cur, e = d.mongo.Database(d.mongoName).Collection(name).Indexes().List(q, options.ListIndexes().SetBatchSize(256).SetMaxTime(queryLimit))
		if e != nil {
			c()
			return result, errors.New("namespace_index_read_failed")
		}
		for cur.Next(q) {
			result[i].Indexes = append(result[i].Indexes, append([]byte(nil), cur.Current...))
			if len(result[i].Indexes) > 256 {
				invalid = true
				break
			}
		}
		re = cur.Err()
		ce = d.finishCursor(cur, re)
		c()
		if ce != nil {
			return result, ce
		}
		if invalid {
			return result, errors.New("namespace_index_inventory_too_large")
		}
		sort.Slice(result[i].Indexes, func(a, b int) bool {
			an, _ := indexName(result[i].Indexes[a])
			bn, _ := indexName(result[i].Indexes[b])
			return an < bn
		})
	}
	return result, nil
}
func (d *database) ledger(ctx context.Context) ([]byte, error) {
	q, c := context.WithTimeout(ctx, queryLimit)
	defer c()
	cur, e := d.mongo.Database(d.mongoName).Collection("schema_migrations").Find(q, bson.D{}, options.Find().SetLimit(2).SetBatchSize(2).SetMaxTime(queryLimit))
	if e != nil {
		return nil, errors.New("mongo_ledger_read_failed")
	}
	var raw []byte
	count := 0
	for cur.Next(q) {
		raw = append([]byte(nil), cur.Current...)
		count++
	}
	re := cur.Err()
	ce := d.finishCursor(cur, re)
	if ce != nil {
		return nil, ce
	}
	if count != 1 {
		return nil, errors.New("mongo_ledger_exact_single_document_required")
	}
	if _, _, e := ledgerVersion(raw); e != nil {
		return nil, e
	}
	return raw, nil
}
func (d *database) exactCount(ctx context.Context, name string) (int64, error) {
	q, c := context.WithTimeout(ctx, queryLimit)
	defer c()
	// No document body is selected or exported. An empty count result is a
	// verified zero; an absent/failed namespace is never converted into zero.
	cur, e := d.mongo.Database(d.mongoName).Collection(name).Aggregate(q, mongo.Pipeline{bson.D{{Key: "$count", Value: "count"}}}, options.Aggregate().SetBatchSize(1).SetMaxTime(queryLimit))
	if e != nil {
		return 0, errors.New("exact_namespace_count_failed")
	}
	var n int64
	seen := 0
	invalid := false
	for cur.Next(q) {
		fields, e := document(cur.Current)
		seen++
		if e != nil || len(fields) != 1 {
			invalid = true
			continue
		}
		if v, ok := fields["count"].Int64OK(); ok {
			n = v
		} else if v, ok := fields["count"].Int32OK(); ok {
			n = int64(v)
		} else {
			invalid = true
		}
	}
	re := cur.Err()
	ce := d.finishCursor(cur, re)
	if ce != nil {
		return 0, ce
	}
	if invalid || seen > 1 || n < 0 {
		return 0, errors.New("exact_namespace_count_invalid")
	}
	return n, nil
}
func catalogEqual(a, b [3]namespaceProof) bool {
	for i := range a {
		a[i].Count = nil
		b[i].Count = nil
	}
	return reflect.DeepEqual(a, b)
}

func (d *database) snapshot(ctx context.Context) (out observation, err error) {
	tx, e := d.conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return out, errors.New("sql_read_only_snapshot_failed")
	}
	rollbackDone := false
	defer func() {
		if !rollbackDone {
			if tx.Rollback() != nil {
				d.sqlRollbackUnproven = true
				err = errors.New("sql_snapshot_rollback_unproven")
			}
		}
	}()
	out.SQLIdentity, out.SQLVersion, out.SQLDirty, err = d.sqlState(ctx, tx)
	if err != nil {
		return out, err
	}
	out.MongoIdentity, err = d.mongoIdentity(ctx)
	if err != nil {
		return out, err
	}
	before, e := d.catalog(ctx)
	if e != nil {
		return out, e
	}
	session, e := d.mongo.StartSession(options.Session().SetSnapshot(true).SetCausalConsistency(false))
	if e != nil {
		return out, errors.New("mongo_snapshot_session_failed")
	}
	func() {
		defer func() {
			q, c := context.WithTimeout(context.Background(), queryLimit)
			session.EndSession(q)
			c()
			d.sessionEndCalled = true
		}()
		sc := mongo.NewSessionContext(ctx, session)
		out.Ledger.Document, err = d.ledger(sc)
		if err != nil {
			return
		}
		for i, name := range []string{"answersheets", "report_generations"} {
			var n int64
			n, err = d.exactCount(sc, name)
			if err != nil {
				return
			}
			before[i].Count = &n
		}
	}()
	if err != nil {
		return out, err
	}
	one := int64(1)
	before[2].Count = &one
	after, e := d.catalog(ctx)
	if e != nil {
		return out, e
	}
	if !catalogEqual(before, after) {
		return out, errors.New("namespace_metadata_changed_during_capture")
	}
	currentHead, e := d.ledger(ctx)
	if e != nil {
		return out, e
	}
	if !bytes.Equal(currentHead, out.Ledger.Document) {
		return out, errors.New("mongo_ledger_changed_during_capture")
	}
	physical, e := d.mongoIdentity(ctx)
	if e != nil || physical != out.MongoIdentity {
		return out, errors.New("mongo_identity_changed_during_capture")
	}
	rollbackDone = true
	if tx.Rollback() != nil {
		d.sqlRollbackUnproven = true
		return out, errors.New("sql_snapshot_rollback_unproven")
	}
	physical, v, dirty, e := d.sqlState(ctx, d.conn)
	if e != nil || physical != out.SQLIdentity || v != out.SQLVersion || dirty != out.SQLDirty {
		return out, errors.New("sql_state_changed_during_capture")
	}
	out.Collections = [2]namespaceProof{before[0], before[1]}
	out.Ledger.Namespace = before[2]
	return out, nil
}
func makeMaterial(o observation, tool, sqlName, mongoName string, at time.Time) material {
	return material{Format: 1, Kind: "m4_b_mongo_index_object_protection", ToolSource: tool, BSource: bSource, BResources: bResources, BResourceFiles: bResourceFiles, CapturedAt: at.UTC().Format(time.RFC3339Nano), SQLName: sqlName, MongoName: mongoName, SQLIdentity: o.SQLIdentity, MongoIdentity: o.MongoIdentity, SQLVersion: o.SQLVersion, Collections: o.Collections, Ledger: o.Ledger}
}
