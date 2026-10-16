//go:build integration

package main

import (
	"fmt"
	"testing"

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
