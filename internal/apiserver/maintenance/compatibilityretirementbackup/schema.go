package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type sqlReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readSQL(ctx context.Context, db sqlReader, query string, args ...any) (out [][]*string, result error) {
	q, c := context.WithTimeout(ctx, 30*time.Second)
	defer c()
	out = make([][]*string, 0)
	rows, e := db.QueryContext(q, query, args...)
	if e != nil {
		return nil, ErrRead
	}
	defer func() {
		if rows.Close() != nil && result == nil {
			result = ErrRead
		}
	}()
	cols, e := rows.Columns()
	if e != nil {
		return nil, ErrRead
	}
	var size int
	for rows.Next() {
		if len(out) >= 10000 {
			return nil, ErrStructure
		}
		raw := make([]sql.RawBytes, len(cols))
		scan := make([]any, len(cols))
		for i := range raw {
			scan[i] = &raw[i]
		}
		if rows.Scan(scan...) != nil {
			return nil, ErrRead
		}
		row := make([]*string, len(raw))
		for i, b := range raw {
			size += len(b)
			if size > metadataBudget {
				return nil, ErrStructure
			}
			if b != nil {
				v := string(b)
				row[i] = &v
			}
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, ErrRead
	}
	return out, nil
}
func cell(row []*string, i int) string {
	if i >= len(row) || row[i] == nil {
		return ""
	}
	return *row[i]
}

var increment = regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)

func readSQLCatalog(ctx context.Context, db sqlReader) (map[string]any, error) {
	tables, e := readSQL(ctx, db, "SELECT TABLE_NAME,TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE() ORDER BY TABLE_NAME")
	if e != nil || len(tables) > 500 {
		return nil, ErrRead
	}
	defs := map[string]any{}
	for _, row := range tables {
		name := cell(row, 0)
		ddl, e := readSQL(ctx, db, "SHOW CREATE TABLE "+quote(name))
		if e != nil || len(ddl) != 1 || len(ddl[0]) != 2 {
			return nil, ErrStructure
		}
		v := increment.ReplaceAllString(cell(ddl[0], 1), "")
		ddl[0][1] = &v
		defs["table:"+name] = ddl
	}
	queries := [][2]string{{"triggers", "SELECT TRIGGER_NAME,EVENT_OBJECT_TABLE,ACTION_STATEMENT FROM information_schema.triggers WHERE trigger_schema=DATABASE() ORDER BY TRIGGER_NAME"}, {"routines", "SELECT ROUTINE_NAME,ROUTINE_TYPE,ROUTINE_DEFINITION FROM information_schema.routines WHERE routine_schema=DATABASE() ORDER BY ROUTINE_NAME"}, {"events", "SELECT EVENT_NAME,EVENT_DEFINITION,STATUS FROM information_schema.events WHERE event_schema=DATABASE() ORDER BY EVENT_NAME"}, {"constraints", "SELECT TABLE_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_SCHEMA,REFERENCED_TABLE_NAME FROM information_schema.key_column_usage WHERE table_schema=DATABASE() AND REFERENCED_TABLE_NAME IS NOT NULL ORDER BY TABLE_NAME,CONSTRAINT_NAME,ORDINAL_POSITION"}, {"inbound_constraints", "SELECT TABLE_SCHEMA,TABLE_NAME,CONSTRAINT_NAME,REFERENCED_TABLE_SCHEMA,REFERENCED_TABLE_NAME,COLUMN_NAME,REFERENCED_COLUMN_NAME FROM information_schema.key_column_usage WHERE REFERENCED_TABLE_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME IN ('domain_event_outbox','ai_bridge_commands','ai_messaging_legacy_commands') ORDER BY TABLE_SCHEMA,TABLE_NAME,CONSTRAINT_NAME,ORDINAL_POSITION"}}
	for _, item := range queries {
		rows, e := readSQL(ctx, db, item[1])
		if e != nil {
			return nil, e
		}
		defs[item[0]] = rows
	}
	if lenJSON(defs) > metadataBudget {
		return nil, ErrStructure
	}
	return defs, nil
}
func lenJSON(v any) int {
	b, e := json.Marshal(v)
	if e != nil {
		return metadataBudget + 1
	}
	return len(b)
}
func sqlState(ctx context.Context, db sqlReader, b Binding) (string, error) {
	ids, e := readSQL(ctx, db, "SELECT @@server_uuid,DATABASE(),VERSION()")
	if e != nil || len(ids) != 1 || len(ids[0]) != 3 || !strings.HasPrefix(cell(ids[0], 2), "8.") {
		return "", ErrIdentity
	}
	identity := parts("mysql_database_identity_v1", cell(ids[0], 0), cell(ids[0], 1))
	if identity != b.IdentityHash || b.AnchorHash != identity || b.GenerationHash != "" {
		return "", ErrIdentity
	}
	heads, e := readSQL(ctx, db, "SELECT version,dirty FROM schema_migrations")
	if e != nil || len(heads) != 1 || cell(heads[0], 0) != strconv.FormatUint(b.Version, 10) || cell(heads[0], 1) != "0" {
		return "", ErrIdentity
	}
	return identity, nil
}
func sqlStructure(ctx context.Context, db sqlReader, index int) (SQLStructure, error) {
	name := targetNames[index]
	types, e := readSQL(ctx, db, "SELECT TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", name)
	if e != nil || len(types) != 1 || cell(types[0], 0) != "BASE TABLE" {
		return SQLStructure{}, ErrStructure
	}
	ddl, e := readSQL(ctx, db, "SHOW CREATE TABLE "+quote(name))
	if e != nil || len(ddl) != 1 || len(ddl[0]) != 2 || cell(ddl[0], 0) != name {
		return SQLStructure{}, ErrStructure
	}
	columns, e := readSQL(ctx, db, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION", name)
	if e != nil || !supportedColumns(index, retirement.SQLColumns(columns)) {
		return SQLStructure{}, ErrStructure
	}
	charsets, e := readSQL(ctx, db, "SELECT COLUMN_NAME,CHARACTER_SET_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION", name)
	if e != nil || len(charsets) != len(columns) {
		return SQLStructure{}, ErrStructure
	}
	environment, e := readSQL(ctx, db, "SELECT VERSION(),@@session.sql_quote_show_create,@@session.sql_mode,@@session.character_set_results,@@session.collation_connection")
	if e != nil || len(environment) != 1 || len(environment[0]) != 5 || cell(environment[0], 1) != "1" {
		return SQLStructure{}, ErrStructure
	}
	return SQLStructure{DDL: cell(ddl[0], 1), Columns: retirement.SQLColumns(columns), CharacterSets: charsets, ShowCreateEnvironment: environment}, nil
}
func canonicalMongo(raw bson.Raw) (any, error) {
	b, e := bson.MarshalExtJSON(raw, true, false)
	if e != nil {
		return nil, ErrStructure
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return nil, ErrStructure
	}
	return v, nil
}
func mongoCatalog(ctx context.Context, db *mongo.Database) (map[string]bson.Raw, map[string]any, error) {
	q, c := context.WithTimeout(mongoMetadataContext(ctx), 30*time.Second)
	defer c()
	cur, e := db.ListCollections(q, bson.D{}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		return nil, nil, ErrRead
	}
	raw := map[string]bson.Raw{}
	defs := map[string]any{}
	for cur.Next(q) {
		b := append(bson.Raw(nil), cur.Current...)
		name, ok := b.Lookup("name").StringValueOK()
		if !ok || len(raw) >= 500 {
			_ = cur.Close(q)
			return nil, nil, ErrStructure
		}
		raw[name] = b
		v, e := canonicalMongo(b)
		if e != nil {
			_ = cur.Close(q)
			return nil, nil, e
		}
		defs["collection:"+name] = v
	}
	readErr, closeErr := cur.Err(), cur.Close(q)
	if readErr != nil || closeErr != nil {
		return nil, nil, ErrRead
	}
	for name, col := range raw {
		if col.Lookup("type").StringValue() != "collection" {
			continue
		}
		indexes, e := db.Collection(name).Indexes().List(q)
		if e != nil {
			return nil, nil, ErrRead
		}
		var values []any
		for indexes.Next(q) {
			v, e := canonicalMongo(indexes.Current)
			if e != nil {
				_ = indexes.Close(q)
				return nil, nil, e
			}
			values = append(values, v)
		}
		readErr, closeErr = indexes.Err(), indexes.Close(q)
		if readErr != nil || closeErr != nil {
			return nil, nil, ErrRead
		}
		sort.Slice(values, func(i, j int) bool { return jsonSHA(values[i]) < jsonSHA(values[j]) })
		defs["indexes:"+name] = values
	}
	if lenJSON(defs) > metadataBudget {
		return nil, nil, ErrStructure
	}
	return raw, defs, nil
}
func mongoState(ctx context.Context, db *mongo.Database, b Binding, collections map[string]bson.Raw) (string, error) {
	var hello, config bson.Raw
	if db.Client().Database("admin").RunCommand(mongoMetadataContext(ctx), bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		return "", ErrIdentity
	}
	migration, ok := collections["schema_migrations"]
	if !ok {
		return "", ErrIdentity
	}
	subtype, uuid, ok := migration.Lookup("info", "uuid").BinaryOK()
	if !ok || subtype != 4 || len(uuid) != 16 {
		return "", ErrIdentity
	}
	stable := bson.D{}
	for _, name := range []string{"setName", "hosts", "me"} {
		v := hello.Lookup(name)
		if v.Type != 0 {
			var a any
			if v.Unmarshal(&a) != nil {
				return "", ErrIdentity
			}
			stable = append(stable, bson.E{Key: name, Value: a})
		}
	}
	stableJSON, e := json.Marshal(stable)
	if e != nil {
		return "", ErrIdentity
	}
	identity := parts("mongodb_database_identity_v1", string(stableJSON), db.Name(), hex.EncodeToString(uuid))
	if identity != b.IdentityHash || parts("mongodb_migration_generation_v1", hex.EncodeToString(uuid)) != b.GenerationHash {
		return "", ErrIdentity
	}
	if b.NamespaceAnchor != nil {
		// hello and collections above came from the real borrowed client and
		// full-visible catalog. The approved seed endpoint stays immutable;
		// the host connection owner is responsible for that selected route.
		if matchMongoNamespaceMetadata(hello, collections, db.Name(), b) != nil {
			return "", ErrIdentity
		}
	} else {
		// Never retry this legacy profile as namespace metadata after code 13
		// or any other replSetGetConfig failure.
		if db.Client().Database("admin").RunCommand(mongoMetadataContext(ctx), bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&config) != nil {
			return "", ErrIdentity
		}
		id, ok := config.Lookup("config", "settings", "replicaSetId").ObjectIDOK()
		set, okSet := hello.Lookup("setName").StringValueOK()
		if !ok || !okSet || id.IsZero() || parts("mongodb_database_anchor_v1", id.Hex(), set, db.Name()) != b.AnchorHash {
			return "", ErrIdentity
		}
	}
	cur, e := db.Collection("schema_migrations").Find(ctx, bson.D{}, options.Find().SetLimit(2))
	if e != nil {
		return "", ErrIdentity
	}
	var heads []bson.Raw
	e = cur.All(ctx, &heads)
	closeErr := cur.Close(ctx)
	if e != nil || closeErr != nil || len(heads) != 1 {
		return "", ErrIdentity
	}
	version := heads[0].Lookup("version")
	var number int64
	switch version.Type {
	case bson.TypeInt32:
		number = int64(version.Int32())
	case bson.TypeInt64:
		number = version.Int64()
	default:
		return "", ErrIdentity
	}
	dirty, ok := heads[0].Lookup("dirty").BooleanOK()
	if number < 0 || uint64(number) != b.Version || !ok || dirty {
		return "", ErrIdentity
	}
	return identity, nil
}

// The catalog caller establishes authorizedCollections=false/full visibility.
// This is a metadata comparison, not a source approval or write capability.
func matchMongoNamespaceMetadata(hello bson.Raw, collections map[string]bson.Raw, database string, b Binding) error {
	expected := b.NamespaceAnchor
	if expected == nil || expected.Validate() != nil || expected.Database != database || expected.Hash != b.AnchorHash || !hashPattern.MatchString(b.GenerationHash) {
		return ErrIdentity
	}
	rows := make([]bson.Raw, 0, len(collections))
	for name, raw := range collections {
		actualName, ok := raw.Lookup("name").StringValueOK()
		if !ok || name != actualName {
			return ErrIdentity
		}
		rows = append(rows, raw)
	}
	observed, err := identitymeta.MongoNamespaceAnchorFromMetadata(hello, rows, database, expected.EndpointSHA256)
	if err != nil || !identitymeta.MatchMongoNamespaceAnchors(expected, observed) {
		return ErrIdentity
	}
	return nil
}

// OrderedMongoSchema is a newly observed original BSON schema candidate, not
// independent approval. Its digest includes the original collection UUID and
// preserves every compound key/order and original option. No body is included.
type OrderedMongoSchema struct {
	data   MongoStructure
	digest string
}

func (s *OrderedMongoSchema) SHA256() string {
	if s == nil {
		return ""
	}
	return s.digest
}
func (*OrderedMongoSchema) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*OrderedMongoSchema) String() string               { return "private ordered Mongo schema candidate" }
func ReadOrderedMongoSchema(ctx context.Context, db *mongo.Database) (*OrderedMongoSchema, error) {
	if ctx == nil || ctx.Err() != nil || db == nil {
		return nil, ErrRead
	}
	q, c := context.WithTimeout(mongoMetadataContext(ctx), 30*time.Second)
	defer c()
	cur, e := db.ListCollections(q, bson.D{{Key: "name", Value: targetNames[3]}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		return nil, ErrRead
	}
	var docs []bson.Raw
	e = cur.All(q, &docs)
	ce := cur.Close(q)
	if e != nil || ce != nil || len(docs) != 1 || docs[0].Lookup("type").StringValue() != "collection" {
		return nil, ErrStructure
	}
	s := MongoStructure{Collection: append([]byte(nil), docs[0]...)}
	indexes, e := db.Collection(targetNames[3]).Indexes().List(q)
	if e != nil {
		return nil, ErrRead
	}
	for indexes.Next(q) {
		if len(s.Indexes) >= 1000 {
			_ = indexes.Close(q)
			return nil, ErrStructure
		}
		s.Indexes = append(s.Indexes, append([]byte(nil), indexes.Current...))
	}
	e = indexes.Err()
	ce = indexes.Close(q)
	if e != nil || ce != nil || len(s.Indexes) == 0 {
		return nil, ErrStructure
	}
	sort.Slice(s.Indexes, func(i, j int) bool {
		return bson.Raw(s.Indexes[i]).Lookup("name").StringValue() < bson.Raw(s.Indexes[j]).Lookup("name").StringValue()
	})
	if lenJSON(s) > metadataBudget {
		return nil, ErrStructure
	}
	if _, e = normalizedMongo(s); e != nil {
		return nil, e
	}
	return &OrderedMongoSchema{data: s, digest: jsonSHA(s)}, nil
}

// UUID is instance identity, not restorable collection schema. It is retained
// in the source archive and its approval; only equality at the new restore
// instance removes UUID, while preserving all remaining ordered BSON fields.
func normalizedMongo(s MongoStructure) (MongoStructure, error) {
	raw := bson.Raw(s.Collection)
	if raw.Validate() != nil {
		return MongoStructure{}, ErrStructure
	}
	es, e := raw.Elements()
	if e != nil {
		return MongoStructure{}, ErrStructure
	}
	doc := bson.D{}
	seen := map[string]bool{}
	for _, element := range es {
		k := element.Key()
		if seen[k] {
			return MongoStructure{}, ErrStructure
		}
		seen[k] = true
		switch k {
		case "name", "type", "options", "idIndex":
			doc = append(doc, bson.E{Key: k, Value: element.Value()})
		case "info":
			info, ok := element.Value().DocumentOK()
			if !ok {
				return MongoStructure{}, ErrStructure
			}
			items, e := info.Elements()
			if e != nil {
				return MongoStructure{}, ErrStructure
			}
			fields := bson.D{}
			for _, item := range items {
				if item.Key() != "uuid" {
					fields = append(fields, bson.E{Key: item.Key(), Value: item.Value()})
				}
			}
			doc = append(doc, bson.E{Key: k, Value: fields})
		default:
			return MongoStructure{}, ErrStructure
		}
	}
	if !seen["name"] || !seen["type"] || !seen["options"] || !seen["info"] || !seen["idIndex"] {
		return MongoStructure{}, ErrStructure
	}
	col, e := bson.Marshal(doc)
	if e != nil {
		return MongoStructure{}, ErrStructure
	}
	out := MongoStructure{Collection: col, Indexes: make([][]byte, len(s.Indexes))}
	for i, b := range s.Indexes {
		if bson.Raw(b).Validate() != nil {
			return MongoStructure{}, ErrStructure
		}
		var index bson.D
		if bson.Unmarshal(b, &index) != nil {
			return MongoStructure{}, ErrStructure
		}
		for _, field := range index {
			if field.Key == "ns" {
				return MongoStructure{}, ErrStructure
			}
		}
		out.Indexes[i] = append([]byte(nil), b...)
	}
	return out, nil
}
func schemaEqual(a, b MongoStructure) bool {
	x, e := normalizedMongo(a)
	if e != nil {
		return false
	}
	y, e := normalizedMongo(b)
	return e == nil && reflect.DeepEqual(x, y)
}
func mongoTargetCanonical(s MongoStructure) (map[string]any, error) {
	c, e := canonicalMongo(bson.Raw(s.Collection))
	if e != nil {
		return nil, e
	}
	var indexes []any
	for _, b := range s.Indexes {
		v, e := canonicalMongo(bson.Raw(b))
		if e != nil {
			return nil, e
		}
		indexes = append(indexes, v)
	}
	sort.Slice(indexes, func(i, j int) bool { return jsonSHA(indexes[i]) < jsonSHA(indexes[j]) })
	return map[string]any{"collection": c, "indexes": indexes}, nil
}
func verifyMongoObject(s MongoStructure, snapshot SourceSnapshot) error {
	raw := bson.Raw(s.Collection)
	subtype, id, ok := raw.Lookup("info", "uuid").BinaryOK()
	if !ok || subtype != 4 || len(id) != 16 || parts("mongodb-object-v1", hex.EncodeToString(id)) != snapshot.IdentityHash {
		return ErrIdentity
	}
	v, e := mongoTargetCanonical(s)
	if e != nil || jsonSHA(v) != snapshot.SchemaHash {
		return ErrStructure
	}
	return nil
}

// MySQL can expand an already equivalent column COLLATE clause with an explicit
// CHARACTER SET after replaying SHOW CREATE (notably a table previously ALTERed).
// Normalize only that redundant display spelling on the same verified UTF8MB4
// column; all column metadata, keys, options and remaining DDL stay exact.
func sqlStructuresEqual(a, b SQLStructure) bool {
	if !reflect.DeepEqual(a.Columns, b.Columns) || !reflect.DeepEqual(a.CharacterSets, b.CharacterSets) || !reflect.DeepEqual(a.ShowCreateEnvironment, b.ShowCreateEnvironment) {
		return false
	}
	return normalizedSQLDDL(a) == normalizedSQLDDL(b)
}

func normalizedSQLDDL(s SQLStructure) string {
	lines := strings.Split(s.DDL, "\n")
	for ci, column := range s.Columns {
		if len(column) != 6 || column[0] == nil || column[1] == nil || column[5] == nil || !strings.HasPrefix(*column[5], "utf8mb4_") || !sqlCollationPattern.MatchString(*column[5]) || ci >= len(s.CharacterSets) {
			continue
		}
		charset := s.CharacterSets[ci]
		if len(charset) != 2 || cell(charset, 0) != *column[0] || cell(charset, 1) != "utf8mb4" {
			continue
		}
		// Consume the clause only immediately after this exact column's datatype.
		// Never rewrite a default, comment, expression, key or table option.
		prefix := "  " + quote(*column[0]) + " " + *column[1]
		full := prefix + " CHARACTER SET utf8mb4 COLLATE " + *column[5]
		short := prefix + " COLLATE " + *column[5]
		for i, line := range lines {
			if strings.HasPrefix(line, full+" ") || strings.HasPrefix(line, full+",") {
				lines[i] = short + strings.TrimPrefix(line, full)
			}
		}
	}
	return strings.Join(lines, "\n")
}

var sqlCollationPattern = regexp.MustCompile(`^[a-z0-9_]+$`)
