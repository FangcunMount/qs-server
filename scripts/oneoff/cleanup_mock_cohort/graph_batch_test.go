package main

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func graphBatchMock(t *testing.T) (*graphStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return newGraphStore(&liveStore{sql: conn}), mock
}

func graphBatchTestDefinition() []byte {
	return encode([]string{"CREATE TABLE fixed_original", "+00:00", "utf8mb4", "STRICT_TRANS_TABLES"})
}

func graphBatchExpectShape(mock sqlmock.Sqlmock, name, keyName, dataType, columnType string) {
	mock.ExpectQuery("SELECT k.COLUMN_NAME,c.DATA_TYPE,c.COLUMN_TYPE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=k.TABLE_SCHEMA AND c.TABLE_NAME=k.TABLE_NAME AND c.COLUMN_NAME=k.COLUMN_NAME WHERE k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME=? AND k.CONSTRAINT_NAME='PRIMARY' ORDER BY k.COLUMN_NAME").WithArgs(name).WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME", "DATA_TYPE", "COLUMN_TYPE"}).AddRow(keyName, dataType, columnType))
	mock.ExpectQuery("SHOW CREATE TABLE `" + name + "`").WillReturnRows(sqlmock.NewRows([]string{"Table", "Create Table"}).AddRow(name, "CREATE TABLE fixed_original"))
	mock.ExpectQuery("SELECT @@session.time_zone,@@session.character_set_results,@@session.sql_mode").WillReturnRows(sqlmock.NewRows([]string{"zone", "charset", "mode"}).AddRow("+00:00", "utf8mb4", "STRICT_TRANS_TABLES"))
}

func graphBatchRow(id, value string) record {
	return record{Object: keySQLForBatch(id), Definition: graphBatchTestDefinition(), Columns: []cell{{Name: "id", Type: "UNSIGNED BIGINT", Raw: []byte(id)}, {Name: "value", Type: "BLOB", Raw: []byte(value)}}}
}

func keySQLForBatch(id string) object {
	return object{Store: "mysql", Name: "assessment_score", Key: []cell{{Name: "id", Type: "uint64", Raw: []byte(id)}}}
}

func graphBatchRows(values ...[]driver.Value) *sqlmock.Rows {
	rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""), sqlmock.NewColumn("value").OfType("BLOB", []byte{}))
	for _, values := range values {
		rows.AddRow(values...)
	}
	return rows
}

func graphBatchExpectGlobalVisibility(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT DATABASE()").WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("qs"))
	mock.ExpectQuery("SHOW GRANTS FOR CURRENT_USER").WillReturnRows(sqlmock.NewRows([]string{"grants"}).AddRow("GRANT ALL PRIVILEGES ON *.* TO `synthetic`@`localhost`"))
}

func graphBatchExpectNoSideEffects(mock sqlmock.Sqlmock) {
	graphBatchExpectGlobalVisibility(mock)
	mock.ExpectQuery("SELECT (SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE=?),(SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE UNIQUE_CONSTRAINT_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME=? AND DELETE_RULE IN ('CASCADE','SET NULL'))").WithArgs("assessment_score", "assessment_score").WillReturnRows(sqlmock.NewRows([]string{"triggers", "cascades"}).AddRow(0, 0))
}

func graphBatchExpectPacket(mock sqlmock.Sqlmock, bytes int64) {
	mock.ExpectQuery("SELECT @@max_allowed_packet").WillReturnRows(sqlmock.NewRows([]string{"packet"}).AddRow(bytes))
}

func graphBatchExpectUniqueIndexes(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectQuery("SELECT INDEX_NAME,SEQ_IN_INDEX,COLUMN_NAME,SUB_PART,INDEX_TYPE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND NON_UNIQUE=0 ORDER BY INDEX_NAME,SEQ_IN_INDEX").WithArgs("assessment_score").WillReturnRows(rows)
}

func TestGraphBatchRestoreChecksActualUniqueCollisionsBeforeTransaction(t *testing.T) {
	for _, currentID := range []string{"1", "2"} {
		t.Run("actual_unique_row_"+currentID, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			r := graphBatchRow("1", "Case Sensitive Original")
			r.Columns[1].Type = "VARCHAR"
			row := func(populated bool) *sqlmock.Rows {
				out := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""), sqlmock.NewColumn("value").OfType("VARCHAR", ""))
				if populated {
					out.AddRow(currentID, "Case Sensitive Original")
				}
				return out
			}
			graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
			graphBatchExpectNoSideEffects(mock)
			mock.ExpectQuery("SELECT * FROM `assessment_score` LIMIT 0").WillReturnRows(row(false))
			mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND GENERATION_EXPRESSION IS NOT NULL AND GENERATION_EXPRESSION<>''").WithArgs("assessment_score").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}))
			graphBatchExpectPacket(mock, 64*1024*1024)
			graphBatchExpectUniqueIndexes(mock, sqlmock.NewRows([]string{"INDEX_NAME", "SEQ_IN_INDEX", "COLUMN_NAME", "SUB_PART", "INDEX_TYPE"}).AddRow("PRIMARY", 1, "id", nil, "BTREE").AddRow("uq_value", 1, "value", nil, "BTREE"))
			// No BINARY coercion: SQL must use the actual unique index's collation.
			mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (x.`value` = ?)").WithArgs([]byte("Case Sensitive Original")).WillReturnRows(row(true))
			err := s.preflightExactBatch(t.Context(), []record{r}, true)
			if currentID == "1" && err != nil || currentID != "1" && (err == nil || !strings.Contains(err.Error(), "unique constraint conflicts") || errors.Is(err, errGraphBatchBudget)) {
				t.Fatal("foreign physical PK was not rejected by read-only admission:", err)
			}
		})
	}
	t.Run("nullable_unique_tuple_has_no_SQL_index_conflict", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		r := graphBatchRow("1", "")
		r.Columns[1].Null, r.Columns[1].Raw = true, nil
		graphBatchExpectUniqueIndexes(mock, sqlmock.NewRows([]string{"INDEX_NAME", "SEQ_IN_INDEX", "COLUMN_NAME", "SUB_PART", "INDEX_TYPE"}).AddRow("uq_nullable", 1, "value", nil, "BTREE"))
		if err := graphSQLUniquePreflight(t.Context(), s.live.sql, []record{r}, graphSQLShape{definition: graphBatchTestDefinition()}, 64<<20); err != nil {
			t.Fatal("SQL NULL does not occupy a conflicting unique tuple:", err)
		}
	})
	for _, invalid := range []struct {
		name      string
		column    any
		prefix    any
		indexType string
	}{{"functional", nil, nil, "BTREE"}, {"prefix", "value", 4, "BTREE"}, {"unsupported_type", "value", nil, "HASH"}} {
		t.Run(invalid.name, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			graphBatchExpectUniqueIndexes(mock, sqlmock.NewRows([]string{"INDEX_NAME", "SEQ_IN_INDEX", "COLUMN_NAME", "SUB_PART", "INDEX_TYPE"}).AddRow("unknown_unique", 1, invalid.column, invalid.prefix, invalid.indexType))
			if err := graphSQLUniquePreflight(t.Context(), s.live.sql, []record{graphBatchRow("1", "original")}, graphSQLShape{}, 64<<20); err == nil || errors.Is(err, errGraphBatchBudget) {
				t.Fatal("unsupported uniqueness cannot become a size split:", err)
			}
		})
	}
}

func TestGraphBatchRestoreUniquePreflightIsRepeatedUnderTransactionLock(t *testing.T) {
	s, mock := graphBatchMock(t)
	r := graphBatchRow("1", "unique-original")
	r.Columns[1].Type = "VARCHAR"
	rows := func(id string) *sqlmock.Rows {
		out := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""), sqlmock.NewColumn("value").OfType("VARCHAR", ""))
		if id != "" {
			out.AddRow(id, "unique-original")
		}
		return out
	}
	expectPlan := func(locked bool) {
		graphBatchExpectNoSideEffects(mock)
		mock.ExpectQuery("SELECT * FROM `assessment_score` LIMIT 0").WillReturnRows(rows(""))
		mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND GENERATION_EXPRESSION IS NOT NULL AND GENERATION_EXPRESSION<>''").WithArgs("assessment_score").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}))
		graphBatchExpectPacket(mock, 64<<20)
		graphBatchExpectUniqueIndexes(mock, sqlmock.NewRows([]string{"INDEX_NAME", "SEQ_IN_INDEX", "COLUMN_NAME", "SUB_PART", "INDEX_TYPE"}).AddRow("uq_value", 1, "value", nil, "BTREE"))
		query := "SELECT x.* FROM `assessment_score` x WHERE (x.`value` = ?)"
		id := ""
		if locked {
			query += " FOR UPDATE"
			id = "2"
		}
		mock.ExpectQuery(query).WithArgs([]byte("unique-original")).WillReturnRows(rows(id))
	}
	graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
	expectPlan(false)
	mock.ExpectBegin()
	graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
	mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(rows(""))
	expectPlan(true)
	mock.ExpectRollback()
	if err := s.mutateExactBatch(t.Context(), []record{r}, true); err == nil || !strings.Contains(err.Error(), "unique constraint conflicts") {
		t.Fatal("new unique-key collision reached INSERT or COMMIT:", err)
	}
}

func graphBatchMongoDefinition(t *testing.T, name string, indexes ...bson.D) []byte {
	t.Helper()
	raw := func(v bson.D) bson.Raw {
		out, err := bson.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	all := []bson.Raw{raw(bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "name", Value: "_id_"}})}
	for _, index := range indexes {
		all = append(all, raw(append(bson.D{{Key: "v", Value: int32(2)}}, index...)))
	}
	return encode(struct {
		Collection bson.Raw
		Indexes    []bson.Raw
	}{raw(bson.D{{Key: "name", Value: name}, {Key: "type", Value: "collection"}, {Key: "options", Value: bson.D{}}}), all})
}
func TestGraphMongoBatchUniqueSpecificationsMatchOriginalMigrationResources(t *testing.T) {
	for _, file := range []string{"000001_init_collections.up.json", "000011_add_interpretation_lifecycle_collections.up.json", "000015_add_report_query_catalog.up.json", "000034_answering_start_unique.up.json", "000037_original_event_evidence.up.json", "000038_add_historical_event_references.up.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "internal", "pkg", "migration", "migrations", "mongodb", file))
		if err != nil {
			t.Fatal(err)
		}
		var commands []struct {
			Collection string            `json:"createIndexes"`
			Indexes    []json.RawMessage `json:"indexes"`
		}
		if err = json.Unmarshal(raw, &commands); err != nil {
			t.Fatal(err)
		}
		for _, command := range commands {
			if _, ok := graphMongoTables[command.Collection]; !ok {
				continue
			}
			unique := []bson.D{}
			for _, rawIndex := range command.Indexes {
				var index bson.D
				if err = bson.UnmarshalExtJSON(rawIndex, false, &index); err != nil {
					t.Fatal(err)
				}
				encoded, err := bson.Marshal(index)
				if err != nil {
					t.Fatal(err)
				}
				if v := bson.Raw(encoded).Lookup("unique"); v.Type == bson.TypeBoolean && v.Boolean() {
					unique = append(unique, index)
				}
			}
			if _, err = graphMongoUniqueIndexes(graphBatchMongoDefinition(t, command.Collection, unique...), command.Collection, "owned"); err != nil {
				t.Fatalf("fixed original unique specifications in %s/%s rejected: %v", file, command.Collection, err)
			}
		}
	}
}

func TestGraphMongoBatchUniquePartialAndMultikeyValues(t *testing.T) {
	startIndex := bson.D{{Key: "key", Value: bson.D{{Key: "start_context.id", Value: int32(1)}}}, {Key: "name", Value: "uk_answersheet_answering_start"}, {Key: "unique", Value: true}, {Key: "partialFilterExpression", Value: bson.D{{Key: "start_context.id", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}}}
	specs, err := graphMongoUniqueIndexes(graphBatchMongoDefinition(t, "answersheets", startIndex), "answersheets", "owned")
	if err != nil || len(specs) != 1 {
		t.Fatal(err)
	}
	for _, input := range []bson.D{{}, {{Key: "start_context", Value: bson.D{{Key: "id", Value: int64(0)}}}}} {
		raw, _ := bson.Marshal(input)
		values, err := graphMongoUniqueValues(raw, specs[0])
		if err != nil || len(values) != 0 {
			t.Fatal("ineligible partial document must not become an index entry:", err)
		}
	}
	raw, _ := bson.Marshal(bson.D{{Key: "start_context", Value: bson.D{{Key: "id", Value: int64(9007199254740993)}}}})
	values, err := graphMongoUniqueValues(raw, specs[0])
	if err != nil || len(values) != 1 || values[0][0].Type != bson.TypeInt64 || values[0][0].Int64() != 9007199254740993 {
		t.Fatal("raw BSON numeric unique value was widened/narrowed:", err)
	}
	entries := graphMongoUniqueSpec{keys: []string{"legacy_submission_evidence.entries.event_id"}, partial: bson.D{{Key: "legacy_submission_evidence.entries.event_id", Value: bson.D{{Key: "$type", Value: "string"}}}}, mode: "event_entries"}
	raw, _ = bson.Marshal(bson.D{{Key: "legacy_submission_evidence", Value: bson.D{{Key: "entries", Value: bson.A{bson.D{{Key: "event_id", Value: "original-a"}}, bson.D{{Key: "event_id", Value: "original-b"}}}}}}})
	values, err = graphMongoUniqueValues(raw, entries)
	if err != nil || len(values) != 2 || values[0][0].StringValue() != "original-a" || values[1][0].StringValue() != "original-b" {
		t.Fatal("original multikey elements lost:", err)
	}
	raw, _ = bson.Marshal(bson.D{{Key: "legacy_submission_evidence", Value: bson.D{{Key: "entries", Value: bson.A{bson.D{{Key: "event_id", Value: "original-a"}}, bson.D{{Key: "event_id", Value: int64(123)}}}}}}})
	if _, err = graphMongoUniqueValues(raw, entries); err == nil || errors.Is(err, errGraphBatchBudget) {
		t.Fatal("uninterpretable eligible multikey document accepted:", err)
	}
	changed := append(bson.D{}, startIndex...)
	for i := range changed {
		if changed[i].Key == "partialFilterExpression" {
			changed[i].Value = bson.D{{Key: "start_context.id", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}
		}
	}
	if _, err = graphMongoUniqueIndexes(graphBatchMongoDefinition(t, "answersheets", changed), "answersheets", "owned"); err == nil {
		t.Fatal("changed partial specification accepted")
	}
}

// Driver Mock only: these are actual provider Find commands/cursor decoding,
// not server/transaction/unique-index execution evidence.
func TestGraphMongoBatchUniqueProbeUsesBatchAndExactOriginalPhysicalIDs(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, foreign := range []bool{false, true} {
		mt.Run(strconv.FormatBool(foreign), func(mt *mtest.T) {
			definition := graphBatchMongoDefinition(mt.T, "answersheets", bson.D{{Key: "key", Value: bson.D{{Key: "domain_id", Value: int32(1)}}}, {Key: "name", Value: "idx_domain_id"}, {Key: "unique", Value: true}})
			id := primitive.NewObjectID()
			doc := bson.D{{Key: "_id", Value: id}, {Key: "domain_id", Value: int64(700)}}
			raw, err := bson.Marshal(doc)
			if err != nil {
				mt.Fatal(err)
			}
			original, err := graphMongoObject("answersheets", raw, definition)
			if err != nil {
				mt.Fatal(err)
			}
			if foreign {
				doc[0].Value = primitive.NewObjectID()
			}
			mt.AddMockResponses(mtest.CreateCursorResponse(0, "owned.answersheets", mtest.FirstBatch, doc))
			s := newGraphStore(&liveStore{mongo: mt.Client, mongoDB: "owned"})
			err = s.graphMongoUniquePreflight(mt.Context(), []record{original}, definition)
			if foreign && (err == nil || errors.Is(err, errGraphBatchBudget)) || !foreign && err != nil {
				mt.Fatal("unique collision returned the wrong original/foreign classification:", err)
			}
			event := mt.GetStartedEvent()
			if event == nil || event.CommandName != "find" {
				mt.Fatal("read-only collision check did not use the actual driver Find")
			}
			filter := event.Command.Lookup("filter").Document()
			branches, err := filter.Lookup("$or").Array().Values()
			if err != nil || len(branches) != 1 || branches[0].Document().Lookup("domain_id", "$eq").Type != bson.TypeInt64 {
				mt.Fatal("raw original BSON filter type changed")
			}
			if mt.GetStartedEvent() != nil {
				mt.Fatal("collision admission issued an unexpected mutation/retry")
			}
		})
	}
}

func TestGraphMongoBatchRestoreRepeatsUniqueCheckInsideManualTransaction(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("foreign_unique_after_admission", func(mt *mtest.T) {
		definition := graphBatchMongoDefinition(mt.T, "answersheets", bson.D{{Key: "key", Value: bson.D{{Key: "domain_id", Value: int32(1)}}}, {Key: "name", Value: "idx_domain_id"}, {Key: "unique", Value: true}})
		var metadata struct {
			Collection bson.Raw
			Indexes    []bson.Raw
		}
		if err := json.Unmarshal(definition, &metadata); err != nil {
			mt.Fatal(err)
		}
		indexes := make([]bson.D, len(metadata.Indexes))
		for i, raw := range metadata.Indexes {
			if err := bson.Unmarshal(raw, &indexes[i]); err != nil {
				mt.Fatal(err)
			}
		}
		var collection bson.D
		if err := bson.Unmarshal(metadata.Collection, &collection); err != nil {
			mt.Fatal(err)
		}
		doc := bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: int64(700)}}
		raw, _ := bson.Marshal(doc)
		original, err := graphMongoObject("answersheets", raw, definition)
		if err != nil {
			mt.Fatal(err)
		}
		foreign := append(bson.D{}, doc...)
		foreign[0].Value = primitive.NewObjectID()
		mt.AddMockResponses(
			mtest.CreateCursorResponse(0, "owned.answersheets", mtest.FirstBatch, indexes...),
			mtest.CreateCursorResponse(0, "owned.$cmd.listCollections", mtest.FirstBatch, collection),
			mtest.CreateCursorResponse(0, "owned.answersheets", mtest.FirstBatch), // Before-claim unique probe.
			mtest.CreateCursorResponse(0, "owned.answersheets", mtest.FirstBatch), // Locked physical-ID read.
			mtest.CreateCursorResponse(0, "owned.answersheets", mtest.FirstBatch, foreign),
			mtest.CreateSuccessResponse(), // Abort the real driver transaction, no insert.
		)
		s := newGraphStore(&liveStore{mongo: mt.Client, mongoDB: "owned"})
		if err = s.mutateExactBatch(mt.Context(), []record{original}, true); err == nil || !strings.Contains(err.Error(), "unique constraint conflicts") {
			mt.Fatal("foreign unique key escaped transaction's second read:", err)
		}
		finds := 0
		for event := mt.GetStartedEvent(); event != nil; event = mt.GetStartedEvent() {
			if event.CommandName == "find" {
				finds++
			}
			if event.CommandName == "insert" || event.CommandName == "commitTransaction" {
				mt.Fatal("foreign unique collision issued a mutation")
			}
		}
		if finds != 3 {
			mt.Fatal("expected batch unique admission, physical-ID read and locked unique recheck; got", finds)
		}
	})
}

func TestGraphBatchReadUsesActualPhysicalTypeAndPreservesMissingDefinitions(t *testing.T) {
	t.Run("shared_plan_is_readable_but_never_writable", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		o := keySQLForBatch("500")
		o.Name = "assessment_plan"
		graphBatchExpectShape(mock, "assessment_plan", "id", "bigint", "bigint unsigned")
		mock.ExpectQuery("SELECT x.* FROM `assessment_plan` x WHERE (`id` = ?)").WithArgs([]byte("500")).WillReturnRows(graphBatchRows([]driver.Value{"500", []byte("shared-original")}))
		got, err := s.readExactBatch(t.Context(), []object{o})
		if err != nil || len(got) != 1 || !got[0].Exists {
			t.Fatal("original shared protection object cannot be batch-read:", err)
		}
		if err = s.preflightExactBatch(t.Context(), []record{got[0].Record}, false); err == nil {
			t.Fatal("shared Plan read conferred mutation authority")
		}
	})
	t.Run("numeric_alias_before_data_query", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
		o := keySQLForBatch("1")
		o.Key[0].Type, o.Key[0].Raw = "text", []byte("01")
		if _, err := s.readExactBatch(t.Context(), []object{o}); err == nil || !strings.Contains(err.Error(), "coerce") || errors.Is(err, errGraphBatchBudget) {
			t.Fatal("numeric alias reached the physical data query:", err)
		}
	})
	t.Run("binary_text_and_returned_bytes", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		graphBatchExpectShape(mock, "interpretation_attention_projection", "event_id", "varchar", "varchar(128)")
		o := object{Store: "mysql", Name: "interpretation_attention_projection", Key: []cell{{Name: "event_id", Type: "text", Raw: []byte("abc")}}}
		mock.ExpectQuery("SELECT x.* FROM `interpretation_attention_projection` x WHERE (BINARY `event_id` = BINARY ?)").WithArgs([]byte("abc")).WillReturnRows(sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("event_id").OfType("VARCHAR", "")).AddRow("ABC"))
		if _, err := s.readExactBatch(t.Context(), []object{o}); err == nil || !strings.Contains(err.Error(), "physical key") {
			t.Fatal("collation alias accepted as the original row:", err)
		}
	})
	t.Run("one_batch_returns_input_order_and_missing_namespace", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
		mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) OR (`id` = ?) OR (`id` = ?)").WithArgs([]byte("1"), []byte("2"), []byte("3")).WillReturnRows(graphBatchRows([]driver.Value{"2", []byte("second")}, []driver.Value{"1", []byte("first")}))
		got, err := s.readExactBatch(t.Context(), []object{keySQLForBatch("1"), keySQLForBatch("2"), keySQLForBatch("3")})
		if err != nil || len(got) != 3 || !got[0].Exists || !got[1].Exists || got[2].Exists {
			t.Fatal("exact batch existence/order changed:", err)
		}
		if !sameRecord(got[0].Record, graphBatchRow("1", "first")) || !sameRecord(got[1].Record, graphBatchRow("2", "second")) || !reflect.DeepEqual(got[2].Record.Definition, graphBatchTestDefinition()) {
			t.Fatal("original rows or missing-row namespace not preserved")
		}
	})
}

func TestGraphBatchRejectsImplicitSQLWritesBeforeClaimAndUnderLock(t *testing.T) {
	query := "SELECT (SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE=?),(SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE UNIQUE_CONSTRAINT_SCHEMA=DATABASE() AND REFERENCED_TABLE_NAME=? AND DELETE_RULE IN ('CASCADE','SET NULL'))"
	for _, tc := range []struct {
		name               string
		triggers, cascades int
		restore            bool
	}{{"trigger_on_delete", 1, 0, false}, {"trigger_on_restore", 1, 0, true}, {"cascade_on_delete", 0, 1, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
			graphBatchExpectGlobalVisibility(mock)
			mock.ExpectQuery(query).WithArgs("assessment_score", "assessment_score").WillReturnRows(sqlmock.NewRows([]string{"triggers", "cascades"}).AddRow(tc.triggers, tc.cascades))
			if err := s.preflightExactBatch(t.Context(), []record{graphBatchRow("1", "original")}, tc.restore); err == nil || errors.Is(err, errGraphBatchBudget) {
				t.Fatal("implicit SQL write was not rejected before claim:", err)
			}
		})
	}
	t.Run("unreadable_metadata_is_not_a_size_retry", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
		denied := errors.New("metadata access denied")
		graphBatchExpectGlobalVisibility(mock)
		mock.ExpectQuery(query).WithArgs("assessment_score", "assessment_score").WillReturnError(denied)
		if err := s.preflightExactBatch(t.Context(), []record{graphBatchRow("1", "original")}, false); !errors.Is(err, denied) || errors.Is(err, errGraphBatchBudget) {
			t.Fatal("unknown trigger/FK state did not fail closed:", err)
		}
	})
	t.Run("fresh_locked_metadata_blocks_a_later_trigger", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		graphBatchExpectDeletePreflight(mock)
		mock.ExpectBegin()
		graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
		mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(graphBatchRows([]driver.Value{"1", []byte("original")}))
		graphBatchExpectGlobalVisibility(mock)
		mock.ExpectQuery(query).WithArgs("assessment_score", "assessment_score").WillReturnRows(sqlmock.NewRows([]string{"triggers", "cascades"}).AddRow(1, 0))
		mock.ExpectRollback()
		if err := s.mutateExactBatch(t.Context(), []record{graphBatchRow("1", "original")}, false); err == nil || errors.Is(err, errGraphBatchBudget) {
			t.Fatal("trigger change after claim reached the mutation:", err)
		}
	})
}

func TestGraphBatchDependencyVisibilityCannotBeInferredFromFilteredMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		grants []string
	}{{"foreign_schema_only", []string{"GRANT SELECT, TRIGGER ON `other`.* TO `synthetic`@`localhost`"}}, {"trigger_visibility_missing", []string{"GRANT SELECT ON *.* TO `synthetic`@`localhost`"}}, {"global_with_partial_revoke", []string{"GRANT ALL PRIVILEGES ON *.* TO `synthetic`@`localhost`", "REVOKE SELECT ON `retained`.* FROM `synthetic`@`localhost`"}}, {"role_alone", []string{"GRANT `maintenance_role`@`%` TO `synthetic`@`localhost`"}}} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
			rows := sqlmock.NewRows([]string{"grants"})
			for _, grant := range tc.grants {
				rows.AddRow(grant)
			}
			mock.ExpectQuery("SELECT DATABASE()").WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("qs"))
			mock.ExpectQuery("SHOW GRANTS FOR CURRENT_USER").WillReturnRows(rows)
			if err := s.preflightExactBatch(t.Context(), []record{graphBatchRow("1", "original")}, false); err == nil || errors.Is(err, errGraphBatchBudget) {
				t.Fatal("empty, possibly filtered metadata would confer deletion authority:", err)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		grants []string
	}{{"global_union", []string{"GRANT SELECT ON *.* TO `synthetic`@`localhost`", "GRANT TRIGGER ON *.* TO `synthetic`@`localhost`"}}, {"current_schema_all", []string{"GRANT ALL PRIVILEGES ON `qs`.* TO `synthetic`@`localhost`"}}, {"current_schema_union", []string{"GRANT SELECT ON `qs`.* TO `synthetic`@`localhost`", "GRANT TRIGGER ON `qs`.* TO `synthetic`@`localhost`"}}} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			mock.ExpectQuery("SELECT DATABASE()").WillReturnRows(sqlmock.NewRows([]string{"database"}).AddRow("qs"))
			rows := sqlmock.NewRows([]string{"grants"})
			for _, grant := range tc.grants {
				rows.AddRow(grant)
			}
			mock.ExpectQuery("SHOW GRANTS FOR CURRENT_USER").WillReturnRows(rows)
			if err := graphBatchSQLVisibility(t.Context(), s.live.sql); err != nil {
				t.Fatal("explicit direct target-schema privilege union was rejected:", err)
			}
		})
	}
}

func graphBatchExpectDeletePreflight(mock sqlmock.Sqlmock) {
	graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
	graphBatchExpectNoSideEffects(mock)
	graphBatchExpectPacket(mock, 64*1024*1024)
}

func TestGraphBatchWriteProtectsCurrentFactsAndDoesNotReplayUnknownCommit(t *testing.T) {
	for _, restore := range []bool{false, true} {
		name := "delete_changed_preimage"
		if restore {
			name = "restore_new_fact_conflict"
		}
		t.Run(name, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			r := graphBatchRow("1", "original")
			graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
			graphBatchExpectNoSideEffects(mock)
			if restore {
				mock.ExpectQuery("SELECT * FROM `assessment_score` LIMIT 0").WillReturnRows(graphBatchRows())
				mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND GENERATION_EXPRESSION IS NOT NULL AND GENERATION_EXPRESSION<>''").WithArgs("assessment_score").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}))
			}
			graphBatchExpectPacket(mock, 64*1024*1024)
			if restore {
				graphBatchExpectUniqueIndexes(mock, sqlmock.NewRows([]string{"INDEX_NAME", "SEQ_IN_INDEX", "COLUMN_NAME", "SUB_PART", "INDEX_TYPE"}))
			}
			mock.ExpectBegin()
			graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
			mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(graphBatchRows([]driver.Value{"1", []byte("new-fact")}))
			mock.ExpectRollback()
			if err := s.mutateExactBatch(t.Context(), []record{r}, restore); err == nil || !strings.Contains(err.Error(), "conflicts") {
				t.Fatal("changed current fact reached a write:", err)
			}
		})
	}
	t.Run("one_delete_commit_error_is_not_reissued", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		records := []record{graphBatchRow("1", "first"), graphBatchRow("2", "second")}
		graphBatchExpectDeletePreflight(mock)
		mock.ExpectBegin()
		graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
		mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) OR (`id` = ?) FOR UPDATE").WithArgs([]byte("1"), []byte("2")).WillReturnRows(graphBatchRows([]driver.Value{"1", []byte("first")}, []driver.Value{"2", []byte("second")}))
		graphBatchExpectNoSideEffects(mock)
		graphBatchExpectPacket(mock, 64*1024*1024)
		mock.ExpectExec("DELETE FROM `assessment_score` WHERE (`id` = ?) OR (`id` = ?)").WithArgs([]byte("1"), []byte("2")).WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) OR (`id` = ?) FOR UPDATE").WithArgs([]byte("1"), []byte("2")).WillReturnRows(graphBatchRows())
		unknown := errors.New("original commit response unavailable")
		mock.ExpectCommit().WillReturnError(unknown)
		if err := s.mutateExactBatch(t.Context(), records, false); !errors.Is(err, unknown) {
			t.Fatal("original commit outcome was lost or retried:", err)
		}
	})
}

func TestGraphBatchRestoreKeepsDefaultTimestampsAndOriginalCells(t *testing.T) {
	s, mock := graphBatchMock(t)
	r := graphBatchRow("1", string([]byte{0, 255, 42}))
	r.Columns = append(r.Columns, cell{Name: "created_at", Type: "DATETIME", Raw: []byte("2019-01-02 03:04:05")}, cell{Name: "computed", Type: "INT", Raw: []byte("9")})
	columnRows := func(populated bool) *sqlmock.Rows {
		rows := sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""), sqlmock.NewColumn("value").OfType("BLOB", []byte{}), sqlmock.NewColumn("created_at").OfType("DATETIME", ""), sqlmock.NewColumn("computed").OfType("INT", 0))
		if populated {
			rows.AddRow("1", []byte{0, 255, 42}, "2019-01-02 03:04:05", "9")
		}
		return rows
	}
	expectPlan := func() {
		graphBatchExpectNoSideEffects(mock)
		mock.ExpectQuery("SELECT * FROM `assessment_score` LIMIT 0").WillReturnRows(columnRows(false))
		mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND GENERATION_EXPRESSION IS NOT NULL AND GENERATION_EXPRESSION<>''").WithArgs("assessment_score").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("computed"))
		graphBatchExpectPacket(mock, 64*1024*1024)
		graphBatchExpectUniqueIndexes(mock, sqlmock.NewRows([]string{"INDEX_NAME", "SEQ_IN_INDEX", "COLUMN_NAME", "SUB_PART", "INDEX_TYPE"}))
	}
	graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
	expectPlan()
	mock.ExpectBegin()
	graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
	mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(columnRows(false))
	expectPlan()
	mock.ExpectExec("INSERT INTO `assessment_score` (`id`,`value`,`created_at`) VALUES (?,?,?)").WithArgs([]byte("1"), []byte{0, 255, 42}, []byte("2019-01-02 03:04:05")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT x.* FROM `assessment_score` x WHERE (`id` = ?) FOR UPDATE").WithArgs([]byte("1")).WillReturnRows(columnRows(true))
	mock.ExpectCommit()
	if err := s.mutateExactBatch(t.Context(), []record{r}, true); err != nil {
		t.Fatal("original timestamp/binary cells did not restore through actual SQL binding:", err)
	}
}

func TestGraphBatchPreflightRejectsOversizeAndForbiddenNamespacesBeforeClaim(t *testing.T) {
	t.Run("retained_mongo_wire_is_readable_but_never_mutated", func(t *testing.T) {
		o := graphBatchMongoKey(t, "original-wire")
		o.Name = "rm_outbox"
		if err := graphBatchObjects([]object{o}, false); err != nil {
			t.Fatal("retained wire must remain available for exact read-only checks:", err)
		}
		raw, err := bson.Marshal(bson.D{{Key: "_id", Value: "original-wire"}, {Key: "state", Value: "published"}})
		if err != nil {
			t.Fatal(err)
		}
		r := record{Object: o, BSON: raw, Definition: []byte("retained original namespace")}
		s := &graphStore{}
		for _, restore := range []bool{false, true} {
			if err := s.preflightExactBatch(t.Context(), []record{r}, restore); err == nil || !strings.Contains(err.Error(), "outside fixed graph") {
				t.Fatal("retained wire reached provider preflight instead of rejecting before any connection:", err)
			}
			if err := s.mutateExactBatch(t.Context(), []record{r}, restore); err == nil || !strings.Contains(err.Error(), "outside fixed graph") {
				t.Fatal("retained wire reached a write/transaction instead of rejecting before any connection:", err)
			}
		}
	})
	t.Run("actual_packet_is_too_small", func(t *testing.T) {
		s, mock := graphBatchMock(t)
		graphBatchExpectShape(mock, "assessment_score", "id", "bigint", "bigint unsigned")
		graphBatchExpectNoSideEffects(mock)
		graphBatchExpectPacket(mock, 4096)
		if err := s.preflightExactBatch(t.Context(), []record{graphBatchRow("1", "original")}, false); !errors.Is(err, errGraphBatchBudget) || !strings.Contains(err.Error(), "packet") {
			t.Fatal("oversize write was not rejected before transaction/claim:", err)
		}
	})
	for _, name := range []string{"assessment_plan", "assessment_entry", "ai_bridge_requests"} {
		t.Run(name, func(t *testing.T) {
			s, _ := graphBatchMock(t)
			r := graphBatchRow("1", "original")
			r.Object.Name = name
			if err := s.preflightExactBatch(t.Context(), []record{r}, false); err == nil {
				t.Fatal("shared/read-only namespace accepted for mutation")
			}
		})
	}
}

func TestGraphMongoBatchRetainsPhysicalBSONTypesAndRejectsAliases(t *testing.T) {
	objects := []object{}
	for _, id := range []any{int32(42), int64(43), "44"} {
		o := graphBatchMongoKey(t, id)
		objects = append(objects, o)
	}
	filter, err := graphMongoBatchFilter(objects)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(filter)
	if err != nil {
		t.Fatal(err)
	}
	values, err := bson.Raw(raw).Lookup("_id", "$in").Array().Values()
	if err != nil || len(values) != 3 || values[0].Type != bson.TypeInt32 || values[1].Type != bson.TypeInt64 || values[2].Type != bson.TypeString {
		t.Fatal("physical typed _id values were normalized or stringified:", err)
	}
	alias, _ := bson.Marshal(bson.D{{Key: "_id", Value: int64(42)}, {Key: "body", Value: "original"}})
	r := record{Object: objects[0], BSON: alias, Definition: []byte("original namespace")}
	if _, err = graphBatchRecords([]record{r}); err == nil {
		t.Fatal("numeric BSON alias accepted for delete/restore")
	}
	original, _ := bson.Marshal(bson.D{{Key: "_id", Value: int32(42)}, {Key: "body", Value: "original"}})
	r.BSON = original
	current := r
	current.BSON, _ = bson.Marshal(bson.D{{Key: "_id", Value: int32(42)}, {Key: "body", Value: "new-fact"}})
	if _, err = graphBatchPreimages([]record{r}, []graphReadResult{{current, true}}, true); err == nil {
		t.Fatal("restore could overwrite a different current BSON fact")
	}
}

func graphBatchMongoKey(t *testing.T, id any) object {
	t.Helper()
	raw, err := bson.Marshal(bson.D{{Key: "_id", Value: id}})
	if err != nil {
		t.Fatal(err)
	}
	v := bson.Raw(raw).Lookup("_id")
	return object{Store: "mongo", Name: "report_query_catalog", Key: []cell{{Name: "_id", Type: "bson:" + hexByte(byte(v.Type)), Raw: append([]byte{}, v.Value...)}}}
}

func hexByte(v byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&15]})
}
