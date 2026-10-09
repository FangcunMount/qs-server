package compatibilityretirementbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ErrNonTargetData Error = "complete_non_target_data_baseline_missing_changed_or_unsupported"

// This binds the fixed host to its already constructed native plan. It does
// not create/reopen a journal, perform DDL or grant acceptance from a DTO.
func VerifyHostAcceptancePlan(ctx context.Context, p *TargetRecoveryPlan, a *Archive, b TargetRecoveryBorrowed, r TargetRecoveryRequest, w *fence.MaintenanceWindow) error {
	if ctx == nil || ctx.Err() != nil || p == nil || p.self != p {
		return ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.archive != a || p.borrowed != b || p.request != r || p.window != w || p.blocked || p.journal == nil || p.journal.validate() != nil {
		return ErrRecoveryBinding
	}
	_, e := targetWindowMatches(ctx, w, r, "")
	return e
}

type nonTargetDataObject struct {
	Database, Name, ColumnsSHA256, DataSHA256 string
	Rows, Bytes, Pages                        uint64
}

// NonTargetDataBaseline covers every stored object in the Archive's approved
// complete namespace catalogs. It is produced after the real historical
// readback in its same live paired RO epoch, not from the 8/11/12 responsibility
// subsets. It includes newly persisted evidence/retired IDs/current MQ facts.
// No original bodies are kept and this object is never writer/DROP authority.
type NonTargetDataBaseline struct {
	self          *NonTargetDataBaseline
	archive       *Archive
	mongo         *mongo.Database
	sqlConnection string
	sqlEvent      uint64
	mongoSession  bson.Raw
	mongoNumber   int64
	objects       []nonTargetDataObject
	seal          string
}

func (*NonTargetDataBaseline) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*NonTargetDataBaseline) UnmarshalJSON([]byte) error   { return ErrSerialization }
func (*NonTargetDataBaseline) String() string {
	return "opaque complete approved non-target data baseline; no writer or DROP authority"
}

type nonTargetDataEpoch struct {
	connection string
	event      uint64
	session    bson.Raw
	number     int64
}

// The host has already opened both snapshots. Observe actual instrumentation
// and driver state; a session default/JSON read-only flag cannot establish one.
func nonTargetEpoch(ctx context.Context, b BorrowedSources) (nonTargetDataEpoch, error) {
	if ctx == nil || ctx.Err() != nil || b.SQL == nil || b.Mongo == nil || retirementevidence.RequireTransaction(ctx, b.Mongo.Collection("rm_outbox")) != nil {
		return nonTargetDataEpoch{}, ErrNonTargetData
	}
	g, ok := hostmysql.TxFromContext(ctx)
	if !ok || g == nil || g.Statement == nil || g.Statement.ConnPool != b.SQL {
		return nonTargetDataEpoch{}, ErrNonTargetData
	}
	r, e := readSQL(ctx, b.SQL, "SELECT t.PROCESSLIST_ID,e.EVENT_ID,e.STATE,e.END_EVENT_ID,e.ACCESS_MODE,e.ISOLATION_LEVEL,e.AUTOCOMMIT FROM performance_schema.events_transactions_current e JOIN performance_schema.threads t ON t.THREAD_ID=e.THREAD_ID WHERE t.PROCESSLIST_ID=CONNECTION_ID()")
	if e != nil || len(r) != 1 || len(r[0]) != 7 || r[0][3] != nil || cell(r[0], 2) != "ACTIVE" || cell(r[0], 4) != "READ ONLY" || cell(r[0], 5) != "REPEATABLE READ" || cell(r[0], 6) != "NO" {
		return nonTargetDataEpoch{}, ErrNonTargetData
	}
	connection, e := strconv.ParseUint(cell(r[0], 0), 10, 64)
	event, ee := strconv.ParseUint(cell(r[0], 1), 10, 64)
	s := mongo.SessionFromContext(ctx)
	x, valid := s.(mongo.XSession) //nolint:staticcheck // pinned native driver snapshot accessor
	if e != nil || ee != nil || connection == 0 || event == 0 || !valid || x.ClientSession() == nil || !x.ClientSession().TransactionRunning() || x.ClientSession().CurrentRc == nil || x.ClientSession().CurrentRc.Level != "snapshot" {
		return nonTargetDataEpoch{}, ErrNonTargetData
	}
	return nonTargetDataEpoch{cell(r[0], 0), event, append(bson.Raw(nil), s.ID()...), x.ClientSession().TxnNumber}, nil
}

func nonTargetEpochSame(a, b nonTargetDataEpoch) bool {
	return a.connection == b.connection && a.event == b.event && a.number == b.number && bytes.Equal(a.session, b.session)
}

// CaptureCompleteNonTargetData borrows actual original handles only. The
// caller ends both RO scopes before DDL. Complete metadata equality fixes the
// approved range: no missing/foreign/unsupported store is silently excluded.
func CaptureCompleteNonTargetData(ctx context.Context, a *Archive, b BorrowedSources, final *retirement.FinalHistoricalObservation) (*NonTargetDataBaseline, error) {
	if ctx == nil || ctx.Err() != nil || a == nil || final == nil || final.ValidateBorrowedSnapshot(ctx) != nil || b.SQL == nil || b.Mongo == nil || a.data.Inventory.Bindings["mysql"].Version != 99 || a.data.Inventory.Bindings["mongodb"].Version != 38 {
		return nil, ErrNonTargetData
	}
	facts := final.Summary()
	if facts.SourceSHA != a.data.Approval.SourceSHA || facts.OperationID != a.data.Approval.OperationID {
		return nil, ErrNonTargetData
	}
	if _, e := sqlState(ctx, b.SQL, a.data.Inventory.Bindings["mysql"]); e != nil {
		return nil, e
	}
	epoch, e := nonTargetEpoch(ctx, b)
	if e != nil {
		return nil, e
	}
	defs, e := readSQLCatalog(ctx, b.SQL)
	if e != nil || jsonSHA(defs) != a.data.Inventory.Bindings["mysql"].CatalogHash {
		return nil, ErrNonTargetData
	}
	cols, mdefs, e := mongoCatalog(ctx, b.Mongo)
	if e != nil || jsonSHA(mdefs) != a.data.Inventory.Bindings["mongodb"].CatalogHash {
		return nil, ErrNonTargetData
	}
	if _, e := mongoState(ctx, b.Mongo, a.data.Inventory.Bindings["mongodb"], cols); e != nil {
		return nil, e
	}
	objects, e := nonTargetDataScan(ctx, b, defs, cols, 99, 38)
	if e != nil {
		return nil, e
	}
	end, e := nonTargetEpoch(ctx, b)
	if e != nil || !nonTargetEpochSame(epoch, end) || final.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrNonTargetData
	}
	// Metadata commands are reread after every body reached real EOF. They
	// are not a substitute for a full writer fence held by the fixed host.
	afterSQL, e := readSQLCatalog(ctx, b.SQL)
	if e != nil || jsonSHA(afterSQL) != jsonSHA(defs) {
		return nil, ErrNonTargetData
	}
	_, afterMongo, e := mongoCatalog(ctx, b.Mongo)
	if e != nil || jsonSHA(afterMongo) != jsonSHA(mdefs) {
		return nil, ErrNonTargetData
	}
	v := &NonTargetDataBaseline{archive: a, mongo: b.Mongo, sqlConnection: epoch.connection, sqlEvent: epoch.event, mongoSession: epoch.session, mongoNumber: epoch.number, objects: objects}
	v.self, v.seal = v, jsonSHA(objects)
	return v, nil
}

// VerifyCompleteNonTargetData uses the same actual DROP plan/native pair
// transition and original handles. The host opens a NEW RR-RO/Mongo snapshot
// only after the migration proof was verified outside a SQL transaction.
// It must finish this comparison before resuming any current internal writer.
func VerifyCompleteNonTargetData(ctx context.Context, v *NonTargetDataBaseline, p *TargetRecoveryPlan, proof *migration.CompatibilityPairMigrationProof, b BorrowedSources) error {
	if ctx == nil || ctx.Err() != nil || b.SQL == nil || b.Mongo == nil || v == nil || v.self != v || v.seal != jsonSHA(v.objects) || p == nil || p.self != p || proof == nil || b.Mongo != v.mongo {
		return ErrNonTargetData
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.archive != v.archive || p.bMigration == nil || p.bMigration.proof != proof || p.blocked || p.borrowed.Mongo != b.Mongo || p.window == nil {
		return ErrNonTargetData
	}
	for i := 0; i < 4; i++ {
		if !p.validDrop(i) {
			return ErrNonTargetData
		}
	}
	if _, e := targetWindowMatches(ctx, p.window, p.request, ""); e != nil {
		return e
	}
	if _, e := sqlState(ctx, b.SQL, p.bMigration.sqlBinding); e != nil {
		return e
	}
	epoch, e := nonTargetEpoch(ctx, b)
	if e != nil || epoch.connection != v.sqlConnection || epoch.event <= v.sqlEvent || (bytes.Equal(epoch.session, v.mongoSession) && epoch.number <= v.mongoNumber) {
		return ErrNonTargetData
	}
	defs, e := readSQLCatalog(ctx, b.SQL)
	if e != nil || jsonSHA(defs) != p.bMigration.sqlBinding.CatalogHash {
		return ErrNonTargetData
	}
	cols, mdefs, e := mongoCatalog(ctx, b.Mongo)
	if e != nil || jsonSHA(mdefs) != p.bMigration.mongoBinding.CatalogHash {
		return ErrNonTargetData
	}
	if _, e = mongoState(ctx, b.Mongo, p.bMigration.mongoBinding, cols); e != nil {
		return e
	}
	objects, e := nonTargetDataScan(ctx, b, defs, cols, 100, 39)
	if e != nil || !reflect.DeepEqual(v.objects, objects) {
		return ErrNonTargetData
	}
	end, e := nonTargetEpoch(ctx, b)
	if e != nil || !nonTargetEpochSame(epoch, end) {
		return ErrNonTargetData
	}
	afterSQL, e := readSQLCatalog(ctx, b.SQL)
	if e != nil || jsonSHA(afterSQL) != jsonSHA(defs) {
		return ErrNonTargetData
	}
	_, afterMongo, e := mongoCatalog(ctx, b.Mongo)
	if e != nil || jsonSHA(afterMongo) != jsonSHA(mdefs) {
		return ErrNonTargetData
	}
	return ctx.Err()
}

func nonTargetDataScan(ctx context.Context, b BorrowedSources, defs map[string]any, cols map[string]bson.Raw, sqlHead, mongoHead uint64) ([]nonTargetDataObject, error) {
	tables, e := readSQL(ctx, b.SQL, "SELECT TABLE_NAME,TABLE_TYPE,ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() ORDER BY TABLE_NAME")
	if e != nil || len(tables) > 500 {
		return nil, ErrNonTargetData
	}
	var out []nonTargetDataObject
	for _, row := range tables {
		name := cell(row, 0)
		if len(row) != 3 || name == "" || defs["table:"+name] == nil {
			return nil, ErrNonTargetData
		}
		if targetSQLName(name) {
			continue
		}
		if cell(row, 1) == "VIEW" {
			out = append(out, nonTargetDataObject{Database: "mysql", Name: name, ColumnsSHA256: jsonSHA(defs["table:"+name]), DataSHA256: sha([]byte("view-no-independent-storage/v1"))})
			continue // No stored rows are exempted; full view DDL remains equal.
		}
		if cell(row, 1) != "BASE TABLE" || cell(row, 2) != "InnoDB" {
			return nil, ErrNonTargetData
		}
		v, e := nonTargetSQLData(ctx, b.SQL, name, sqlHead)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	names := make([]string, 0, len(cols))
	for name := range cols {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == targetNames[3] {
			continue
		}
		typ, ok := cols[name].Lookup("type").StringValueOK()
		if !ok {
			return nil, ErrNonTargetData
		}
		if typ == "view" {
			out = append(out, nonTargetDataObject{Database: "mongodb", Name: name, ColumnsSHA256: sha(cols[name]), DataSHA256: sha([]byte("view-no-independent-storage/v1"))})
			continue // All backing stores are still part of the complete range.
		}
		if typ != "collection" {
			return nil, ErrNonTargetData
		}
		v, e := nonTargetMongoData(ctx, b.Mongo, name, mongoHead)
		if e != nil {
			return nil, e // Capped/system collections are not guessed away.
		}
		v.ColumnsSHA256, e = nonTargetOrderedMongo(ctx, b.Mongo, name, cols[name])
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, ErrNonTargetData
	}
	return out, ctx.Err()
}

// Preserve actual ordered BSON compound-index keys and collection options.
// ExtJSON maps alone cannot show an index-order change. Only the paired
// migration's own schema_migrations UUID is normalized; all other UUIDs stay.
func nonTargetOrderedMongo(ctx context.Context, db *mongo.Database, name string, raw bson.Raw) (string, error) {
	q, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cur, e := db.Collection(name).Indexes().List(q)
	if e != nil {
		return "", ErrRead
	}
	s := MongoStructure{Collection: append([]byte(nil), raw...)}
	for cur.Next(q) {
		if len(s.Indexes) >= 1000 {
			_ = cur.Close(q)
			return "", ErrNonTargetData
		}
		s.Indexes = append(s.Indexes, append([]byte(nil), cur.Current...))
	}
	err, closeErr := cur.Err(), cur.Close(q)
	if err != nil || closeErr != nil || len(s.Indexes) == 0 {
		return "", ErrRead
	}
	sort.Slice(s.Indexes, func(i, j int) bool {
		return bson.Raw(s.Indexes[i]).Lookup("name").StringValue() < bson.Raw(s.Indexes[j]).Lookup("name").StringValue()
	})
	if name == "schema_migrations" {
		s, e = normalizedMongo(s)
		if e != nil {
			return "", e
		}
	}
	return jsonSHA(s), nil
}

// Lexicographic predicates use the actual ordered PRIMARY index. Values are
// raw driver bytes rebound to their native columns; no ID is invented/coerced
// into a numeric paging cursor. Missing PK or nullable PK is an explicit gap.
func nonTargetSQLPredicate(keys []string, values []any, upper bool) (string, []any) {
	var terms []string
	var args []any
	for i, key := range keys {
		var conjunction []string
		for j := 0; j < i; j++ {
			conjunction = append(conjunction, quote(keys[j])+" = ?")
			args = append(args, values[j])
		}
		op := " > ?"
		if upper {
			op = " < ?"
			if i == len(keys)-1 {
				op = " <= ?"
			}
		}
		conjunction = append(conjunction, quote(key)+op)
		args = append(args, values[i])
		terms = append(terms, "("+strings.Join(conjunction, " AND ")+")")
	}
	return "(" + strings.Join(terms, " OR ") + ")", args
}

func nonTargetSQLData(ctx context.Context, db sqlReader, name string, head uint64) (nonTargetDataObject, error) {
	v := nonTargetDataObject{Database: "mysql", Name: name}
	columns, e := readSQL(ctx, db, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? ORDER BY ORDINAL_POSITION", name)
	if e != nil || len(columns) == 0 || len(columns) > 512 {
		return v, ErrNonTargetData
	}
	keys, e := readSQL(ctx, db, "SELECT COLUMN_NAME FROM information_schema.key_column_usage WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION", name)
	if e != nil || len(keys) == 0 || len(keys) > len(columns) {
		return v, ErrNonTargetData
	}
	var projections, order, keyNames, keyProjection []string
	keyIndices := make([]int, len(keys))
	for i, row := range columns {
		if len(row) != 6 || cell(row, 0) == "" {
			return v, ErrNonTargetData
		}
		projections = append(projections, "CAST("+quote(cell(row, 0))+" AS BINARY)")
		for j, key := range keys {
			if len(key) == 1 && cell(key, 0) == cell(row, 0) && cell(row, 2) == "NO" {
				keyIndices[j] = i + 1
			}
		}
	}
	for i, row := range keys {
		if len(row) != 1 || keyIndices[i] == 0 {
			return v, ErrNonTargetData
		}
		keyNames = append(keyNames, cell(row, 0))
		keyProjection = append(keyProjection, "CAST("+quote(cell(row, 0))+" AS BINARY)")
		order = append(order, quote(cell(row, 0)))
	}
	v.ColumnsSHA256 = jsonSHA([]any{columns, keyNames})
	upperRows, e := readSQL(ctx, db, "SELECT "+strings.Join(keyProjection, ",")+" FROM "+quote(name)+" FORCE INDEX (PRIMARY) ORDER BY "+strings.Join(order, " DESC,")+" DESC LIMIT 1")
	if e != nil || len(upperRows) > 1 {
		return v, ErrNonTargetData
	}
	var upper []any
	if len(upperRows) == 1 {
		if len(upperRows[0]) != len(keys) {
			return v, ErrNonTargetData
		}
		for _, value := range upperRows[0] {
			if value == nil {
				return v, ErrNonTargetData
			}
			upper = append(upper, []byte(*value))
		}
	}
	h := sha256.New()
	frame(h, []byte(v.ColumnsSHA256), false)
	var last []any
	for {
		q, cancel := context.WithTimeout(ctx, 30*time.Second)
		query := "SELECT " + strings.Join(projections, ",") + " FROM " + quote(name) + " FORCE INDEX (PRIMARY)"
		var predicates []string
		var args []any
		if len(upper) != 0 {
			predicate, values := nonTargetSQLPredicate(keyNames, upper, true)
			predicates, args = append(predicates, predicate), append(args, values...)
		}
		if len(last) != 0 {
			predicate, values := nonTargetSQLPredicate(keyNames, last, false)
			predicates, args = append(predicates, predicate), append(args, values...)
		}
		if len(predicates) != 0 {
			query += " WHERE " + strings.Join(predicates, " AND ")
		}
		query += " ORDER BY " + strings.Join(order, ",") + " LIMIT 1000"
		rows, e := db.QueryContext(q, query, args...)
		if e != nil {
			cancel()
			return v, ErrRead
		}
		page := 0
		for rows.Next() {
			raw := make([]sql.RawBytes, len(columns))
			scan := make([]any, len(raw))
			for i := range raw {
				scan[i] = &raw[i]
			}
			if rows.Scan(scan...) != nil {
				_ = rows.Close()
				cancel()
				return v, ErrRead
			}
			var rowBytes uint64
			for _, value := range raw {
				rowBytes += uint64(len(value))
			}
			if len(upper) == 0 || rowBytes > retirement.MaxSourceRowBytes || v.Rows >= retirement.MaxSourceRecords || v.Bytes > retirement.MaxSourceBytes || rowBytes > retirement.MaxSourceBytes-v.Bytes {
				_ = rows.Close()
				cancel()
				return v, ErrNonTargetData
			}
			if name == "schema_migrations" {
				if len(columns) != 2 || cell(columns[0], 0) != "version" || cell(columns[1], 0) != "dirty" || string(raw[0]) != strconv.FormatUint(head, 10) || string(raw[1]) != "0" || v.Rows != 0 {
					_ = rows.Close()
					cancel()
					return v, ErrNonTargetData
				}
				// Only this real native pair's exact clean head transition is
				// allowed; all other stores keep their complete byte digest.
				frame(h, []byte("native-pair-clean-head/v1"), false)
				rowBytes = 0
			} else {
				for _, value := range raw {
					frame(h, value, value == nil)
				}
			}
			last = make([]any, len(keys))
			for i, column := range keyIndices {
				if raw[column-1] == nil {
					_ = rows.Close()
					cancel()
					return v, ErrNonTargetData
				}
				last[i] = append(make([]byte, 0, len(raw[column-1])), raw[column-1]...)
			}
			v.Rows++
			v.Bytes += rowBytes
			page++
		}
		err, closeErr := rows.Err(), rows.Close()
		cancel()
		if err != nil || closeErr != nil || ctx.Err() != nil {
			return v, ErrRead
		}
		v.Pages++
		if page < 1000 {
			break
		}
	}
	if name == "schema_migrations" && v.Rows != 1 {
		return v, ErrNonTargetData
	}
	v.DataSHA256 = hex.EncodeToString(h.Sum(nil))
	return v, nil
}

func nonTargetMongoData(ctx context.Context, db *mongo.Database, name string, head uint64) (nonTargetDataObject, error) {
	v := nonTargetDataObject{Database: "mongodb", Name: name, ColumnsSHA256: sha([]byte("original-ordered-server-bson/v1"))}
	kind, expectedRows, e := nonTargetMongoTypeCount(ctx, db, name)
	if e != nil {
		return v, e
	}
	q, cancel := context.WithTimeout(ctx, 30*time.Second)
	cur, e := db.Collection(name).Find(q, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetCollation(&options.Collation{Locale: "simple"}).SetLimit(1))
	if e != nil {
		cancel()
		return v, ErrRead
	}
	var upper bson.RawValue
	if cur.Next(q) {
		id := cur.Current.Lookup("_id")
		if nonTargetMongoPKKind(name, id) == "" || nonTargetMongoPKKind(name, id) != kind || expectedRows == 0 {
			_ = cur.Close(q)
			cancel()
			return v, ErrNonTargetData
		}
		upper = bson.RawValue{Type: id.Type, Value: append([]byte(nil), id.Value...)}
	}
	err, ce := cur.Err(), cur.Close(q)
	cancel()
	if err != nil || ce != nil {
		return v, ErrRead
	}
	if (upper.Type == 0) != (expectedRows == 0) {
		return v, ErrNonTargetData
	}
	h := sha256.New()
	var last bson.RawValue
	for {
		q, cancel := context.WithTimeout(ctx, 30*time.Second)
		filter := bson.D{}
		if upper.Type != 0 {
			bounds := bson.D{{Key: "$lte", Value: upper}}
			if last.Type != 0 {
				bounds = append(bounds, bson.E{Key: "$gt", Value: last})
			}
			filter = bson.D{{Key: "_id", Value: bounds}}
		}
		cur, e := db.Collection(name).Find(q, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}).SetLimit(1000).SetBatchSize(1000))
		if e != nil {
			cancel()
			return v, ErrRead
		}
		page := 0
		for cur.Next(q) {
			raw := append(bson.Raw(nil), cur.Current...)
			id := raw.Lookup("_id")
			valid := upper.Type != 0 && raw.Validate() == nil && nonTargetMongoPKKind(name, id) != "" && nonTargetMongoPKKind(name, id) == nonTargetMongoPKKind(name, upper)
			cmp, compareErr := nonTargetMongoPKCompare(name, id, upper)
			valid = valid && compareErr == nil && cmp <= 0
			if last.Type != 0 {
				cmp, compareErr = nonTargetMongoPKCompare(name, last, id)
				valid = valid && compareErr == nil && cmp < 0
			}
			if !valid || len(raw) > retirement.MaxSourceRowBytes || v.Rows >= retirement.MaxSourceRecords || v.Bytes > retirement.MaxSourceBytes || uint64(len(raw)) > retirement.MaxSourceBytes-v.Bytes {
				_ = cur.Close(q)
				cancel()
				return v, ErrNonTargetData
			}
			last = bson.RawValue{Type: id.Type, Value: append([]byte(nil), id.Value...)}
			if name == "schema_migrations" {
				if nonTargetMigrationDocument(raw, head) != nil || v.Rows != 0 {
					_ = cur.Close(q)
					cancel()
					return v, ErrNonTargetData
				}
				frame(h, []byte("native-pair-clean-head/v1"), false)
			} else {
				frame(h, raw, false)
				v.Bytes += uint64(len(raw))
			}
			v.Rows++
			page++
		}
		err, ce := cur.Err(), cur.Close(q)
		cancel()
		if err != nil || ce != nil || ctx.Err() != nil {
			return v, ErrRead
		}
		v.Pages++
		if page < 1000 {
			break
		}
	}
	if v.Rows != expectedRows || name == "schema_migrations" && v.Rows != 1 {
		return v, ErrNonTargetData
	}
	if e := verifyMongoIDTypes(ctx, db, SourceSnapshot{Name: name, Records: expectedRows, Boundary: retirement.SourceBoundary{PKType: kind}}); e != nil {
		return v, e
	}
	v.DataSHA256 = hex.EncodeToString(h.Sum(nil))
	return v, nil
}

type nonTargetMongoTypeGroup struct {
	Kind string `bson:"_id"`
	N    int64  `bson:"n"`
}

func nonTargetMongoTypeCount(ctx context.Context, db *mongo.Database, name string) (string, uint64, error) {
	q, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pipeline := mongo.Pipeline{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: "$_id"}}}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}}
	cur, e := db.Collection(name).Aggregate(q, pipeline, options.Aggregate().SetCollation(&options.Collation{Locale: "simple"}))
	if e != nil {
		return "", 0, ErrRead
	}
	var groups []nonTargetMongoTypeGroup
	e, closeErr := cur.All(q, &groups), cur.Close(q)
	if e != nil || closeErr != nil {
		return "", 0, ErrRead
	}
	return nonTargetMongoTypes(name, groups)
}

func nonTargetMongoTypes(name string, groups []nonTargetMongoTypeGroup) (string, uint64, error) {
	if len(groups) == 0 {
		return "", 0, nil
	}
	// Mongo range predicates use type bracketing. Without a whole-collection
	// type/count observation, mixed IDs below a typed upper could be hidden
	// while a paged query misleadingly reaches EOF. Unsupported means fail.
	if len(groups) != 1 {
		return "", 0, ErrNonTargetData
	}
	kind := groups[0].Kind
	supported := kind == "objectId" || kind == "string" || kind == "int" || kind == "long" || name == "rm_outbox" && kind == "object"
	if !supported || groups[0].N <= 0 || uint64(groups[0].N) > retirement.MaxSourceRecords {
		return "", 0, ErrNonTargetData
	}
	return groups[0].Kind, uint64(groups[0].N), nil
}

// The pinned Mongo RM producer uses this exact ordered document identity.
// Preserve its original BSON token and simple-collation string tuple order;
// an arbitrary/reordered/mixed document ID is unsupported, never flattened.
func nonTargetMongoPKKind(name string, id bson.RawValue) string {
	if id.Type != bson.TypeEmbeddedDocument {
		return pkKind(id)
	}
	if name != "rm_outbox" {
		return ""
	}
	elements, e := id.Document().Elements()
	keys := []string{"producer", "message_id", "destination"}
	if e != nil || len(elements) != len(keys) {
		return ""
	}
	for i, key := range keys {
		value, ok := elements[i].Value().StringValueOK()
		if elements[i].Key() != key || !ok || value == "" || !utf8.ValidString(value) {
			return ""
		}
	}
	return "object"
}

func nonTargetMongoPKCompare(name string, a, b bson.RawValue) (int, error) {
	if a.Type != bson.TypeEmbeddedDocument || b.Type != bson.TypeEmbeddedDocument {
		return comparePK(a, b)
	}
	if nonTargetMongoPKKind(name, a) != "object" || nonTargetMongoPKKind(name, b) != "object" {
		return 0, ErrNonTargetData
	}
	for _, key := range []string{"producer", "message_id", "destination"} {
		cmp := strings.Compare(a.Document().Lookup(key).StringValue(), b.Document().Lookup(key).StringValue())
		if cmp != 0 {
			return cmp, nil
		}
	}
	return 0, nil
}

func nonTargetMigrationDocument(raw bson.Raw, head uint64) error {
	elements, e := raw.Elements()
	if e != nil || len(elements) != 3 {
		return ErrNonTargetData
	}
	seen := map[string]bool{}
	for _, element := range elements {
		key := element.Key()
		if seen[key] || (key != "_id" && key != "version" && key != "dirty") {
			return ErrNonTargetData
		}
		seen[key] = true
	}
	if _, ok := raw.Lookup("_id").ObjectIDOK(); !ok {
		return ErrNonTargetData
	}
	dirty, ok := raw.Lookup("dirty").BooleanOK()
	version := raw.Lookup("version")
	var actual int64
	switch version.Type {
	case bson.TypeInt32:
		actual = int64(version.Int32())
	case bson.TypeInt64:
		actual = version.Int64()
	default:
		return ErrNonTargetData
	}
	if !ok || dirty || actual < 0 || uint64(actual) != head {
		return ErrNonTargetData
	}
	return nil
}
