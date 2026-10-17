package main

import (
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
)

func ownerSQLRows(columns []string, values ...[]driver.Value) *sqlmock.Rows {
	definitions := make([]*sqlmock.Column, len(columns))
	for i, name := range columns {
		definitions[i] = sqlmock.NewColumn(name).OfType("UNSIGNED BIGINT", "")
	}
	rows := sqlmock.NewRowsWithColumnDefinition(definitions...)
	for _, row := range values {
		rows.AddRow(row...)
	}
	return rows
}
func ownerSQLQuery(name string, columns []string) string {
	selected := make([]string, len(columns))
	for i, column := range columns {
		selected[i] = "x.`" + column + "`"
	}
	return "SELECT " + strings.Join(selected, ",") + " FROM `" + name + "` x WHERE x.id IN (?) ORDER BY x.`id` LIMIT 1000"
}
func TestGraphSQLOwnerLookupKeepsFixedParentColumnsAndNullableAssessment(t *testing.T) {
	for name, columns := range map[string][]string{
		"assessment_entry_intake_log": {"id", "org_id", "testee_id"},
		"clinician_relation":          {"id", "org_id", "testee_id"},
		"assessment":                  {"id", "org_id", "testee_id"},
		"plan_enrollment":             {"id", "org_id", "testee_id", "plan_id"},
		"assessment_task":             {"id", "org_id", "testee_id", "assessment_id", "plan_id", "enrollment_id"},
		"evaluation_outcome":          {"id", "org_id", "testee_id", "assessment_id"},
	} {
		t.Run(name, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			graphBatchExpectShape(mock, name, "id", "bigint", "bigint unsigned")
			values := make([]driver.Value, len(columns))
			expected := map[string]string{}
			for i, column := range columns {
				values[i] = "7"
				expected[column] = "7"
				if column == "assessment_id" && name == "assessment_task" {
					values[i] = nil
					delete(expected, column)
				}
			}
			mock.ExpectQuery(ownerSQLQuery(name, columns)).WithArgs("7").WillReturnRows(ownerSQLRows(columns, values))
			planner := cohortPlanner{reader: s}
			owners, err := planner.sqlLinkOwners(t.Context(), name, []string{"7"})
			if err != nil || !reflect.DeepEqual(owners["7"], expected) {
				t.Fatal("projected SQL parent identity/NULL semantics changed", owners, err)
			}
			if _, ok := s.sqlShapes[name]; !ok {
				t.Fatal("owner projection skipped original DDL/session metadata freeze")
			}
		})
	}
}
func TestGraphSQLOwnerProjectionLeavesRawOutcomeBodyAndFailuresIntact(t *testing.T) {
	columns := []string{"id", "org_id", "testee_id", "assessment_id"}
	for _, kind := range []string{"missing", "null_primary_key", "driver_error", "full_raw_body"} {
		t.Run(kind, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			graphBatchExpectShape(mock, "evaluation_outcome", "id", "bigint", "bigint unsigned")
			expected := mock.ExpectQuery(ownerSQLQuery("evaluation_outcome", columns)).WithArgs("201")
			failure := errors.New("owned projection query failed")
			switch kind {
			case "driver_error":
				expected.WillReturnError(failure)
			case "missing":
				expected.WillReturnRows(ownerSQLRows(columns))
			case "null_primary_key":
				expected.WillReturnRows(ownerSQLRows(columns, []driver.Value{nil, "1", "44", "101"}))
			default:
				expected.WillReturnRows(ownerSQLRows(columns, []driver.Value{"201", "1", "44", "101"}))
			}
			planner := cohortPlanner{reader: s}
			owners, err := planner.sqlLinkOwners(t.Context(), "evaluation_outcome", []string{"201"})
			if kind == "driver_error" {
				if !errors.Is(err, failure) {
					t.Fatal("original driver failure was masked", err)
				}
				return
			}
			if kind == "null_primary_key" {
				if err == nil {
					t.Fatal("missing physical identity was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if kind == "missing" {
				if len(owners) != 0 {
					t.Fatal("missing parent was invented")
				}
				return
			}
			body := strings.Repeat("original frozen report body ", 4096)
			mock.ExpectQuery("SELECT x.* FROM `evaluation_outcome` x WHERE x.id IN (?) ORDER BY x.`id` LIMIT 1000").WithArgs("201").WillReturnRows(sqlmock.NewRowsWithColumnDefinition(sqlmock.NewColumn("id").OfType("UNSIGNED BIGINT", ""), sqlmock.NewColumn("payload_json").OfType("LONGTEXT", "")).AddRow("201", body))
			seen := false
			err = s.sqlPages(t.Context(), "evaluation_outcome", "x.id IN (?)", []any{"201"}, func(r record) error {
				raw, ok := graphSQLValue(r, "payload_json")
				seen = ok && raw == body
				return nil
			})
			if err != nil || !seen {
				t.Fatal("complete raw reader lost its original body", err)
			}
		})
	}
}
func TestGraphMongoOwnerProjectionKeepsAllTechnicalTypesAndOriginalRawReader(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, name := range []string{"answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts"} {
		mt.Run(name, func(mt *mtest.T) {
			index := bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "name", Value: "_id_"}}
			collection := bson.D{{Key: "name", Value: name}, {Key: "type", Value: "collection"}, {Key: "options", Value: bson.D{}}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: make([]byte, 16)}}}}}
			doc := bson.D{{Key: "_id", Value: primitive.NewObjectID()}}
			fields := []string{"domain_id", "org_id", "testee_id", "assessment_id", "outcome_id", "generation_id", "interpretation_run_id", "report_id", "latest_run_id"}
			for _, field := range fields {
				doc = append(doc, bson.E{Key: field, Value: int64(7)})
			}
			body := strings.Repeat("full original unused report body ", 4096)
			full := append(append(bson.D{}, doc...), bson.E{Key: "conclusion", Value: body})
			mt.AddMockResponses(mtest.CreateCursorResponse(0, "owned."+name, mtest.FirstBatch, index), mtest.CreateCursorResponse(0, "owned.$cmd.listCollections", mtest.FirstBatch, collection), mtest.CreateCursorResponse(0, "owned."+name, mtest.FirstBatch, doc), mtest.CreateCursorResponse(0, "owned."+name, mtest.FirstBatch, full))
			s := newGraphStore(&liveStore{mongo: mt.Client, mongoDB: "owned"})
			planner := cohortPlanner{reader: s}
			owners, err := planner.mongoLinkOwners(mt.Context(), name, []string{"7"})
			if err != nil || len(owners["7"]) != 9 {
				mt.Fatal("projected Mongo owner lost a technical field", owners, err)
			}
			for _, field := range fields {
				if owners["7"][field] != "7" {
					mt.Fatal("Mongo typed parent field changed", field)
				}
			}
			for _, command := range []string{"listIndexes", "listCollections", "find"} {
				event := mt.GetStartedEvent()
				if event == nil || event.CommandName != command {
					mt.Fatal("metadata freeze/lookup order changed", command, event)
				}
				if command == "find" {
					projection := event.Command.Lookup("projection").Document()
					elements, e := projection.Elements()
					if e != nil || len(elements) != 10 {
						mt.Fatal("fixed owner projection is incomplete", e)
					}
					for _, field := range append([]string{"_id"}, fields...) {
						if projection.Lookup(field).Type != bson.TypeInt32 || projection.Lookup(field).Int32() != 1 {
							mt.Fatal("technical field omitted from owner projection", field)
						}
					}
				}
			}
			seen := false
			err = s.mongoPages(mt.Context(), name, bson.D{{Key: "domain_id", Value: int64(7)}}, func(r record) error { seen = bson.Raw(r.BSON).Lookup("conclusion").StringValue() == body; return nil })
			if err != nil || !seen {
				mt.Fatal("complete Mongo raw record changed", err)
			}
			event := mt.GetStartedEvent()
			if event == nil || event.Command.Lookup("projection").Type != 0 || mt.GetStartedEvent() != nil {
				mt.Fatal("projection escaped into complete raw/capture path")
			}
		})
	}
}
func TestGraphMongoProjectedOwnerRejectsAliasesDuplicatesAndMissingIdentity(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	for _, kind := range []string{"int32_alias", "unexpected_wrong_parent_type", "missing_domain", "missing_physical_id", "duplicate_domain", "null_parent", "zero_optional_parent", "missing_row"} {
		mt.Run(kind, func(mt *mtest.T) {
			doc := bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "domain_id", Value: int64(7)}}
			switch kind {
			case "int32_alias":
				doc[1].Value = int32(7)
			case "unexpected_wrong_parent_type":
				doc = append(doc, bson.E{Key: "latest_run_id", Value: "7"})
			case "missing_domain":
				doc = doc[:1]
			case "missing_physical_id":
				doc = doc[1:]
			case "null_parent":
				doc = append(doc, bson.E{Key: "latest_run_id", Value: nil})
			case "zero_optional_parent":
				doc = append(doc, bson.E{Key: "latest_run_id", Value: int64(0)})
			}
			docs := []bson.D{doc}
			if kind == "duplicate_domain" {
				other := append(bson.D{}, doc...)
				other[0].Value = primitive.NewObjectID()
				docs = append(docs, other)
			}
			if kind == "missing_row" {
				docs = nil
			}
			mt.AddMockResponses(mtest.CreateCursorResponse(0, "owned.report_generations", mtest.FirstBatch, docs...))
			s := newGraphStore(&liveStore{mongo: mt.Client, mongoDB: "owned"})
			s.mongoDefinitions["report_generations"] = graphBatchMongoDefinition(mt.T, "report_generations")
			planner := cohortPlanner{reader: s}
			owners, err := planner.mongoLinkOwners(mt.Context(), "report_generations", []string{"7"})
			if kind == "missing_row" {
				if err != nil || len(owners) != 0 {
					mt.Fatal("missing owner was invented", owners, err)
				}
			} else if kind == "zero_optional_parent" {
				if err != nil || len(owners["7"]) != 1 {
					mt.Fatal("optional zero parent changed its original absent-field semantics", owners, err)
				}
			} else if err == nil {
				mt.Fatal("projected owner weakened original typed identity rejection", kind)
			}
		})
	}
}

func TestGraphProjectedParentKeepsStatisticMissingAndCrossOwnerHolds(t *testing.T) {
	columns := []string{"id", "org_id", "testee_id", "assessment_id"}
	for _, kind := range []string{"original_owner", "foreign_org", "foreign_testee", "missing", "null_testee"} {
		t.Run(kind, func(t *testing.T) {
			s, mock := graphBatchMock(t)
			graphBatchExpectShape(mock, "evaluation_outcome", "id", "bigint", "bigint unsigned")
			values := []driver.Value{"201", "1", "44", "101"}
			switch kind {
			case "foreign_org":
				values[1] = "2"
			case "foreign_testee":
				values[2] = "45"
			case "null_testee":
				values[2] = nil
			}
			rows := ownerSQLRows(columns)
			if kind != "missing" {
				rows.AddRow(values...)
			}
			mock.ExpectQuery(ownerSQLQuery("evaluation_outcome", columns)).WithArgs("201").WillReturnRows(rows)
			dir := privateTestDir(t)
			index := graphIndex{Counts: map[string]int64{}, Holds: map[string]int64{}, RequiredActions: map[string]int64{}}
			planner := cohortPlanner{reader: s, context: t.Context(), selector: cohortSelector{Selection: selection{OrgID: "1"}}, writer: &graphWriter{dir: dir, index: &index, protected: map[string]bool{}}, dir: dir}
			r := graphSQLFixture("statistics_assessment_fact", "1", map[string]string{"org_id": "1", "testee_id": "44", "assessment_id": "101", "outcome_id": "201"})
			r.Columns = append(r.Columns, cell{Name: "source_type", Type: "VARCHAR", Raw: []byte("evaluation_outcome")}, cell{Name: "source_ref", Type: "VARCHAR", Raw: []byte("201")})
			if err := planner.statisticBatch(t.Context(), []record{r}); err != nil {
				t.Fatal(err)
			}
			if err := planner.flushHolds(); err != nil {
				t.Fatal(err)
			}
			err := validateGraphHolds(dir, index, map[string]bool{"44": true})
			if kind == "original_owner" {
				if err != nil || len(index.Holds) != 0 {
					t.Fatal("unchanged original owner was rejected", err)
				}
			} else {
				if err == nil || index.Holds["statistics_assessment_fact:typed_statistic_source_ownership_conflict"] != 1 {
					t.Fatal("projection lost the original component hold", kind, index.Holds)
				}
			}
		})
	}
}
