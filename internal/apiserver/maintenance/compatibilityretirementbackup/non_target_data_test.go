package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func nonTargetMockSQL(t *testing.T, name string, migration bool) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, e := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := mock.ExpectationsWereMet(); e != nil {
			t.Error(e)
		}
		_ = db.Close()
	})
	columns := sqlmock.NewRows([]string{"name", "type", "null", "default", "extra", "collation"})
	key := "id"
	if migration {
		key = "version"
		columns.AddRow("version", "bigint", "NO", nil, "", nil).AddRow("dirty", "tinyint(1)", "NO", nil, "", nil)
	} else {
		columns.AddRow("id", "varchar(64)", "NO", nil, "", "utf8mb4_bin").AddRow("body", "blob", "YES", nil, "", nil)
	}
	mock.ExpectQuery("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? ORDER BY ORDINAL_POSITION").WithArgs(name).WillReturnRows(columns)
	mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.key_column_usage WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION").WithArgs(name).WillReturnRows(sqlmock.NewRows([]string{"name"}).AddRow(key))
	return db, mock
}

func TestNonTargetSQLFixedUpperFullPageNeedsActualEOF(t *testing.T) {
	db, mock := nonTargetMockSQL(t, "all_business", false)
	mock.ExpectQuery("SELECT CAST(`id` AS BINARY) FROM `all_business` FORCE INDEX (PRIMARY) ORDER BY `id` DESC LIMIT 1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("0999"))
	rows := sqlmock.NewRows([]string{"id", "body"})
	for i := 0; i < 1000; i++ {
		rows.AddRow(fmt.Sprintf("%04d", i), fmt.Sprintf("fact-%d", i))
	}
	first := "SELECT CAST(`id` AS BINARY),CAST(`body` AS BINARY) FROM `all_business` FORCE INDEX (PRIMARY) WHERE ((`id` <= ?)) ORDER BY `id` LIMIT 1000"
	mock.ExpectQuery(first).WithArgs([]byte("0999")).WillReturnRows(rows)
	last := "SELECT CAST(`id` AS BINARY),CAST(`body` AS BINARY) FROM `all_business` FORCE INDEX (PRIMARY) WHERE ((`id` <= ?)) AND ((`id` > ?)) ORDER BY `id` LIMIT 1000"
	mock.ExpectQuery(last).WithArgs([]byte("0999"), []byte("0999")).WillReturnRows(sqlmock.NewRows([]string{"id", "body"}))
	v, e := nonTargetSQLData(t.Context(), db, "all_business", 99)
	if e != nil || v.Rows != 1000 || v.Pages != 2 || v.Bytes == 0 || !hashPattern.MatchString(v.DataSHA256) {
		t.Fatalf("full page did not reach explicit EOF: %+v %v", v, e)
	}
}

func TestNonTargetSQLNullEmptyAndEmptyPrimaryKeyRemainDistinct(t *testing.T) {
	var values []nonTargetDataObject
	for _, body := range []any{nil, []byte{}} {
		db, mock := nonTargetMockSQL(t, "all_business", false)
		// CAST AS BINARY is returned by the actual MySQL driver as []byte;
		// a mock string("") would exercise database/sql's string conversion.
		mock.ExpectQuery("SELECT CAST(`id` AS BINARY) FROM `all_business` FORCE INDEX (PRIMARY) ORDER BY `id` DESC LIMIT 1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow([]byte{}))
		mock.ExpectQuery("SELECT CAST(`id` AS BINARY),CAST(`body` AS BINARY) FROM `all_business` FORCE INDEX (PRIMARY) WHERE ((`id` <= ?)) ORDER BY `id` LIMIT 1000").WithArgs([]byte{}).WillReturnRows(sqlmock.NewRows([]string{"id", "body"}).AddRow([]byte{}, body))
		v, e := nonTargetSQLData(t.Context(), db, "all_business", 99)
		if e != nil || v.Rows != 1 || v.Pages != 1 {
			t.Fatalf("valid empty primary key discarded: %+v %v", v, e)
		}
		values = append(values, v)
	}
	if values[0].DataSHA256 == values[1].DataSHA256 {
		t.Fatal("NULL and empty original value had equal evidence")
	}
}

func TestNonTargetSQLNoPKAndInterruptedRowsCannotBeComplete(t *testing.T) {
	t.Run("no-primary", func(t *testing.T) {
		db, mock, e := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = db.Close() }()
		mock.ExpectQuery("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? ORDER BY ORDINAL_POSITION").WithArgs("all_business").WillReturnRows(sqlmock.NewRows([]string{"name", "type", "null", "default", "extra", "collation"}).AddRow("body", "blob", "YES", nil, "", nil))
		mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.key_column_usage WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? AND CONSTRAINT_NAME='PRIMARY' ORDER BY ORDINAL_POSITION").WithArgs("all_business").WillReturnRows(sqlmock.NewRows([]string{"name"}))
		if _, e = nonTargetSQLData(t.Context(), db, "all_business", 99); e == nil {
			t.Fatal("keyless store silently excluded")
		}
		if e = mock.ExpectationsWereMet(); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("read-interrupted", func(t *testing.T) {
		db, mock := nonTargetMockSQL(t, "all_business", false)
		mock.ExpectQuery("SELECT CAST(`id` AS BINARY) FROM `all_business` FORCE INDEX (PRIMARY) ORDER BY `id` DESC LIMIT 1").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("b"))
		mock.ExpectQuery("SELECT CAST(`id` AS BINARY),CAST(`body` AS BINARY) FROM `all_business` FORCE INDEX (PRIMARY) WHERE ((`id` <= ?)) ORDER BY `id` LIMIT 1000").WithArgs([]byte("b")).WillReturnRows(sqlmock.NewRows([]string{"id", "body"}).AddRow("a", "original").AddRow("b", "tail").RowError(1, errors.New("read interrupted")))
		if v, e := nonTargetSQLData(t.Context(), db, "all_business", 99); e == nil || v.DataSHA256 != "" {
			t.Fatal("interrupted source became complete digest")
		}
	})
}

func TestNonTargetCompositeBoundsPreserveNativeTupleAndArgumentOrder(t *testing.T) {
	keys := []string{"org", "command"}
	values := []any{[]byte("0007"), []byte{}}
	upper, args := nonTargetSQLPredicate(keys, values, true)
	if upper != "((`org` < ?) OR (`org` = ? AND `command` <= ?))" || !reflect.DeepEqual(args, []any{values[0], values[0], values[1]}) {
		t.Fatal("composite upper changed native key order")
	}
	lower, args := nonTargetSQLPredicate(keys, values, false)
	if lower != "((`org` > ?) OR (`org` = ? AND `command` > ?))" || !reflect.DeepEqual(args, []any{values[0], values[0], values[1]}) {
		t.Fatal("composite last token lost key order/empty value")
	}
}

func TestNonTargetNativePairHeadIsTheOnlyDataDifference(t *testing.T) {
	var values []nonTargetDataObject
	for _, head := range []uint64{99, 100} {
		db, mock := nonTargetMockSQL(t, "schema_migrations", true)
		mock.ExpectQuery("SELECT CAST(`version` AS BINARY) FROM `schema_migrations` FORCE INDEX (PRIMARY) ORDER BY `version` DESC LIMIT 1").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(fmt.Sprint(head)))
		mock.ExpectQuery("SELECT CAST(`version` AS BINARY),CAST(`dirty` AS BINARY) FROM `schema_migrations` FORCE INDEX (PRIMARY) WHERE ((`version` <= ?)) ORDER BY `version` LIMIT 1000").WithArgs([]byte(fmt.Sprint(head))).WillReturnRows(sqlmock.NewRows([]string{"version", "dirty"}).AddRow(fmt.Sprint(head), "0"))
		v, e := nonTargetSQLData(t.Context(), db, "schema_migrations", head)
		if e != nil {
			t.Fatal(e)
		}
		values = append(values, v)
	}
	if !reflect.DeepEqual(values[0], values[1]) {
		t.Fatal("exact allowed native head transition made baseline permanently unequal")
	}
	for _, item := range []struct {
		name string
		doc  bson.D
		ok   bool
	}{
		{"clean38", bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "version", Value: int32(38)}, {Key: "dirty", Value: false}}, true},
		{"clean39", bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}}, true},
		{"dirty", bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "version", Value: int64(39)}, {Key: "dirty", Value: true}}, false},
		{"extra-data", bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}, {Key: "business_fact", Value: 1}}, false},
		{"duplicate", bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "version", Value: int64(39)}, {Key: "version", Value: int64(39)}}, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			raw, e := bson.Marshal(item.doc)
			if e != nil {
				t.Fatal(e)
			}
			head := uint64(39)
			if item.name == "clean38" {
				head = 38
			}
			if (nonTargetMigrationDocument(raw, head) == nil) != item.ok {
				t.Fatal("unapproved metadata data difference allowed")
			}
		})
	}
}

func TestNonTargetDTOOrBudgetWindowCannotMintAcceptance(t *testing.T) {
	for _, v := range []*NonTargetDataBaseline{nil, {}, {objects: []nonTargetDataObject{{Database: "mysql", Name: "all_business"}}}} {
		if _, e := json.Marshal(v); v != nil && e == nil {
			t.Fatal("private data baseline serialized")
		}
		if VerifyCompleteNonTargetData(context.Background(), v, new(TargetRecoveryPlan), nil, BorrowedSources{}) == nil {
			t.Fatal("summary/unissued plan produced actual complete readback")
		}
	}
	if v, e := CaptureCompleteNonTargetData(context.Background(), new(Archive), BorrowedSources{}, nil); e == nil || v != nil {
		t.Fatal("missing genuine historical/paired epoch produced full data baseline")
	}
}

func TestNonTargetMongoPreservesActualOrderedRMDocumentToken(t *testing.T) {
	token := func(doc bson.D) bson.RawValue {
		raw, e := bson.Marshal(doc)
		if e != nil {
			t.Fatal(e)
		}
		return bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: raw}
	}
	a := token(bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "10"}, {Key: "destination", Value: "qs.a"}})
	b := token(bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "2"}, {Key: "destination", Value: "qs.a"}})
	if nonTargetMongoPKKind("rm_outbox", a) != "object" {
		t.Fatal("current SDK's actual ordered BSON identity is unsupported")
	}
	if cmp, e := nonTargetMongoPKCompare("rm_outbox", a, b); e != nil || cmp >= 0 {
		t.Fatal("native simple string order became numeric event order")
	}
	for _, bad := range []bson.RawValue{
		token(bson.D{{Key: "message_id", Value: "10"}, {Key: "producer", Value: "qs-server"}, {Key: "destination", Value: "qs.a"}}),
		token(bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: int64(10)}, {Key: "destination", Value: "qs.a"}}),
		token(bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "10"}, {Key: "message_id", Value: "qs.a"}}),
	} {
		if nonTargetMongoPKKind("rm_outbox", bad) != "" {
			t.Fatal("reordered/mixed/duplicate document token accepted")
		}
		if _, e := nonTargetMongoPKCompare("rm_outbox", bad, b); e == nil {
			t.Fatal("ambiguous BSON order was guessed")
		}
	}
	if nonTargetMongoPKKind("unapproved_document_id", a) != "" {
		t.Fatal("RM protocol generalized to another stored object")
	}
}

func TestNonTargetMongoTypeBracketingCannotHidePartOfACollection(t *testing.T) {
	for name, groups := range map[string][]nonTargetMongoTypeGroup{
		"mixed":        {{"string", 10}, {"objectId", 2}},
		"missing-id":   {{"missing", 1}},
		"unsupported":  {{"array", 1}},
		"other-object": {{"object", 1}},
		"zero-group":   {{"string", 0}},
		"negative":     {{"string", -1}},
		"budget":       {{"string", 1_000_001}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, e := nonTargetMongoTypes("all_business", groups); e == nil {
				t.Fatal("typed range could hide a whole-namespace gap")
			}
		})
	}
	if kind, count, e := nonTargetMongoTypes("rm_outbox", []nonTargetMongoTypeGroup{{"object", 7}}); e != nil || kind != "object" || count != 7 {
		t.Fatal("exact pinned RM document ID type/count rejected")
	}
}
