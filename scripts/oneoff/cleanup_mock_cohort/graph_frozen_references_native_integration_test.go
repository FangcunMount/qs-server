//go:build integration

package main

import (
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The normal owned provider fixture supplies actual SQL101/Mongo39. This tests
// query semantics in the server; the separate CLI lane tests clearance admission.
func TestMockCohortDirectedReferencesNativeOwned(t *testing.T) {
	h := newCohortNative(t)
	ctx := t.Context()
	roots := map[string]bool{"999001": true}
	f := graphFrozenReferences{ids: map[string]map[string]bool{"mysql/assessment": {"101": true, "9007199254740994": true}, "mysql/plan_enrollment": {"501": true}, "mysql/answering_start": {"71": true}, "mongo/interpret_report_artifacts": {"601": true}}}
	for n, test := range []struct {
		field string
		value any
		match bool
	}{
		{"assessment_id", int64(101), true}, {"assessment_id", int32(101), true},
		{"assessment_id", float64(101), true}, {"assessment_id", "101", true},
		{"assessment_id", float64(9007199254740994), true}, {"assessment_id", float64(9007199254740993), false},
		{"assessment_id", "+000101", true}, {"assessment_id", "000101", true},
		{"assessment_id", 101.5, false}, {"report_id", int64(101), false},
		{"start_context", bson.D{{Key: "id", Value: "+00071"}}, true},
		{"attribution", bson.D{{Key: "enrollment_id", Value: "000501"}}, true},
	} {
		id := primitive.NewObjectID()
		doc := bson.D{{Key: "_id", Value: id}, {Key: "domain_id", Value: int64(900000 + n)}, {Key: test.field, Value: test.value}}
		coll := h.client.Database(h.name).Collection("answersheets")
		_, err := coll.InsertOne(ctx, doc)
		require.NoError(t, err)
		filter, err := graphFrozenMongoReferenceFilter("answersheets", roots, nil, f)
		require.NoError(t, err)
		filter = bson.D{{Key: "$and", Value: bson.A{filter, bson.D{{Key: "_id", Value: id}}}}}
		count, err := coll.CountDocuments(ctx, filter)
		require.NoError(t, err)
		if test.match {
			require.EqualValues(t, 1, count, "case %d must select alias/nested candidate", n)
		} else {
			require.Zero(t, count, "case %d mixed independent ID domains", n)
		}
		g := group{IAM: []byte(`{"user_ids":[],"profile_ids":[],"login_identity_ids":[]}`), QS: qsScope{Graph: &graphBinding{RootIDs: []string{"999001"}}}}
		err = graphRelationshipReferences(graphFrozenRuleDeleteContext(ctx, true), newLeanReader(newGraphStore(h.store)), g, f, false)
		if test.match {
			require.ErrorContains(t, err, "new or foreign reverse reference", "case %d must block actual global clearance", n)
		} else {
			require.NoError(t, err, "case %d is a distinct typed domain", n)
		}
		ack, err := coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
		require.NoError(t, err)
		require.EqualValues(t, 1, ack.DeletedCount)
	}
	h.exec("INSERT INTO statistics_assessment_fact(id,org_id,fact_key,core_hash,fact_type,occurred_at,stat_date,source_type,source_ref,testee_id,assessment_id,outcome_id,answersheet_id,report_id,enrollment_id,task_id) VALUES(811,1,'direct-native',REPEAT('b',64),'assessment_completed','2026-01-01','2026-01-01','evaluation_outcome','201',45,101,201,700,501,501,601)")
	for _, test := range []struct {
		scope, id string
		want      int
	}{
		{"mysql/assessment", "101", 1}, {"mongo/answersheets", "700", 1},
		{"mongo/interpret_report_artifacts", "501", 1}, {"mongo/interpret_report_artifacts", "700", 0},
		{"mysql/assessment", "700", 0}, {"mysql/assessment_task", "601", 1},
	} {
		refs := graphFrozenReferences{ids: map[string]map[string]bool{test.scope: {test.id: true}}}
		count := 0
		err := graphFrozenStatisticsReferences(ctx, newLeanReader(newGraphStore(h.store)), roots, nil, nil, refs, func(r record) error { count++; return nil })
		require.NoError(t, err)
		require.Equal(t, test.want, count, fmt.Sprintf("%s/%s", test.scope, test.id))
	}
	// Original source_ref equality uses SQL052's PAD SPACE collation; this is
	// not a numeric cast and must not confuse a different source domain.
	h.exec("UPDATE statistics_assessment_fact SET source_ref='201 ' WHERE id=811")
	for _, kind := range []string{"evaluation_outcome", "Evaluation_Outcome"} {
		count := 0
		err := graphFrozenStatisticsReferences(ctx, newLeanReader(newGraphStore(h.store)), roots, nil, map[string]map[string]bool{kind: {"201": true}}, graphFrozenReferences{}, func(r record) error { count++; return nil })
		require.NoError(t, err)
		if kind == "evaluation_outcome" {
			require.Equal(t, 1, count)
		} else {
			require.Zero(t, count)
		}
	}
	t.Log("directed_reference_actual_mongo_aliases_nested_and_sql_typed_domains=true synthetic_only=true")
}

// SQL052 fixes source_ref to utf8mb4_unicode_ci, independently of a client's
// connection character set. Use the real directed path, including PAD SPACE
// and exact source domains, in each supported client character set.
func TestMockCohortDirectedStatisticsSessionCharsetsNativeOwned(t *testing.T) {
	h := newCohortNative(t)
	ctx := t.Context()
	h.exec("INSERT INTO statistics_assessment_fact(id,org_id,fact_key,core_hash,fact_type,occurred_at,stat_date,source_type,source_ref,testee_id,assessment_id,outcome_id) VALUES(811,1,'charset-native',REPEAT('b',64),'assessment_completed','2026-01-01','2026-01-01','evaluation_outcome','201 ',45,101,201)")
	for i, ref := range []string{"+000201", "000201", "9007199254740993"} {
		h.exec("INSERT INTO statistics_assessment_fact(id,org_id,fact_key,core_hash,fact_type,occurred_at,stat_date,source_type,source_ref) VALUES(?,1,?,REPEAT('b',64),'assessment_completed','2026-01-01','2026-01-01','evaluation_outcome',?)", 812+i, fmt.Sprintf("charset-native-%d", i), ref)
	}
	defer func() {
		_, err := h.store.sql.ExecContext(ctx, "SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci")
		require.NoError(t, err)
	}()
	for _, session := range []struct{ charset, collation string }{
		{"utf8mb3", "utf8mb3_general_ci"},
		{"latin1", "latin1_swedish_ci"},
		{"utf8mb4", "utf8mb4_0900_ai_ci"},
	} {
		t.Run(session.charset, func(t *testing.T) {
			_, err := h.store.sql.ExecContext(ctx, "SET NAMES "+session.charset+" COLLATE "+session.collation)
			require.NoError(t, err)
			var charset, collation string
			require.NoError(t, h.store.sql.QueryRowContext(ctx, "SELECT @@character_set_connection,@@collation_connection").Scan(&charset, &collation))
			require.Equal(t, session.charset, charset)
			require.Equal(t, session.collation, collation)
			// Preserve proof of the original failure; no retry or changed SQL052 DDL.
			var oldCount int
			err = h.store.sql.QueryRowContext(ctx, "SELECT COUNT(*) FROM statistics_assessment_fact AS x WHERE x.source_ref IN (SELECT ids.id COLLATE utf8mb4_unicode_ci FROM JSON_TABLE(?, '$[*]' COLUMNS(id VARCHAR(64) PATH '$')) AS ids)", `["201"]`).Scan(&oldCount)
			if session.charset == "utf8mb4" {
				require.NoError(t, err)
				require.Equal(t, 1, oldCount)
			} else {
				var driverError *mysql.MySQLError
				require.ErrorAs(t, err, &driverError)
				require.EqualValues(t, 1253, driverError.Number)
				t.Logf("original_expression_mysql_error=%d session_charset=%s session_collation=%s", driverError.Number, charset, collation)
			}
			for _, test := range []struct {
				kind, id string
				want     int
			}{
				{"evaluation_outcome", "201", 1},              // SQL052 PAD SPACE preserved.
				{"Evaluation_Outcome", "201", 0},              // Case-insensitive column must not expand source type.
				{"assessment", "201", 0},                      // Independent typed source domain.
				{"evaluation_outcome", "101", 0},              // Parent number is not a source_ref.
				{"evaluation_outcome", "9007199254740993", 1}, // Exact decimal text above 2^53.
				{"evaluation_outcome", "9007199254740992", 0}, // No float/numeric alias expansion.
			} {
				count := 0
				err := graphFrozenStatisticsReferences(ctx, newLeanReader(newGraphStore(h.store)), nil, nil, map[string]map[string]bool{test.kind: {test.id: true}}, graphFrozenReferences{}, func(record) error { count++; return nil })
				require.NoError(t, err)
				require.Equal(t, test.want, count)
			}
		})
	}
}
