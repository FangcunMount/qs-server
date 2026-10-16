package main

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSQLNumericAliasRejectedFromActualPKMetadata(t *testing.T) {
	db, m, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	m.ExpectQuery("SELECT k.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE").WithArgs("assessment_score").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME", "DATA_TYPE", "COLUMN_TYPE"}).AddRow("id", "bigint", "bigint unsigned"))
	o := object{Store: "mysql", Name: "assessment_score", Key: []cell{{Name: "id", Type: "text", Raw: []byte("01")}}}
	if _, _, e = readSQL(t.Context(), conn, o, false); e == nil || !strings.Contains(e.Error(), "cannot coerce") {
		t.Fatalf("numeric SQL coercion must be rejected before reading/deleting row: %v", e)
	}
	if e = m.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestSQLTextKeysUseBinaryAndActualStoredBytes(t *testing.T) {
	db, m, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	conn, e := db.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = conn.Close() }()
	m.ExpectQuery("SELECT k.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE").WithArgs("interpretation_attention_projection").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME", "DATA_TYPE", "COLUMN_TYPE"}).AddRow("event_id", "varchar", "varchar(128)"))
	m.ExpectQuery("SHOW CREATE TABLE").WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow("interpretation_attention_projection", "CREATE TABLE actual"))
	m.ExpectQuery("SELECT @@session.time_zone").WillReturnRows(sqlmock.NewRows([]string{"zone", "charset", "mode"}).AddRow("+00:00", "utf8mb4", "STRICT_TRANS_TABLES"))
	m.ExpectQuery("SELECT \\* FROM `interpretation_attention_projection` WHERE BINARY `event_id` = BINARY \\?").WithArgs([]byte("abc")).WillReturnRows(sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("event_id").OfType("VARCHAR", "")).AddRow("ABC"))
	o := object{Store: "mysql", Name: "interpretation_attention_projection", Key: []cell{{Name: "event_id", Type: "text", Raw: []byte("abc")}}}
	if _, _, e = readSQL(context.Background(), conn, o, false); e == nil || !strings.Contains(e.Error(), "original key bytes") {
		t.Fatalf("even a query match must verify the actual physical key: %v", e)
	}
	if e = m.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestMongoNumericAndCollationAliasesCannotOwnProtectedObject(t *testing.T) {
	for _, pair := range [][2]any{{int64(42), int32(42)}, {"REAL", "real"}} {
		raw, e := bson.Marshal(bson.D{{Key: "_id", Value: pair[0]}, {Key: "keep", Value: true}})
		if e != nil {
			t.Fatal(e)
		}
		typ, value, e := bson.MarshalValue(pair[1])
		if e != nil {
			t.Fatal(e)
		}
		o := object{Store: "mongo", Name: "report_query_catalog", Key: []cell{{Name: "_id", Type: fmtBSONType(byte(typ)), Raw: value}}}
		if e = exactMongoKey(raw, o); e == nil {
			t.Fatal("Mongo equal query values must not replace exact BSON identity")
		}
	}
}

func fmtBSONType(b byte) string {
	const hex = "0123456789abcdef"
	return "bson:" + string([]byte{hex[b>>4], hex[b&15]})
}

func TestSQLBITRestorationKeepsRawBits(t *testing.T) {
	v, e := sqlValue(cell{Type: "BIT", Raw: []byte{0xff, 0x80, 0x01}})
	if e != nil || v != uint64(0xff8001) {
		t.Fatal("BIT bytes must not be inserted as a numeric string")
	}
	v, e = sqlValue(cell{Type: "JSON", Null: true})
	if e != nil || v != nil {
		t.Fatal("SQL NULL restoration changed")
	}
	v, e = sqlValue(cell{Type: "JSON", Raw: []byte("null")})
	if e != nil || string(v.([]byte)) != "null" {
		t.Fatal("JSON null restoration changed")
	}
}
