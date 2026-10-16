package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// These queries select candidates only. The existing typed relationship callback
// remains authoritative, including noncanonical string and numeric BSON aliases.
func graphFrozenMongoReferenceFilter(name string, roots, users map[string]bool, f graphFrozenReferences) (bson.D, error) {
	switch name {
	case "answersheets", "interpret_report_artifacts", "report_query_catalog", "report_generations", "interpretation_runs":
	default:
		return nil, errors.New("directed reference namespace outside fixed catalog")
	}
	or := bson.A{}
	add := func(field string, ids map[string]bool, artifact bool) error {
		if len(ids) == 0 {
			return nil
		}
		values := bson.A{}
		for _, id := range sortedGraphIDs(ids) {
			if !graphDecimal(id) {
				return errors.New("directed reference requires exact positive int64 identity")
			}
			n, _ := strconv.ParseInt(id, 10, 64)
			values = append(values, n, id)
		}
		// $in covers canonical strings and Mongo numeric equality. Only strings
		// with '+' or leading zero can additionally match the original ParseInt
		// fallback. Return those rare candidates too, without expanding every doc.
		match := bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: field, Value: bson.D{{Key: "$in", Value: values}}}},
			bson.D{{Key: field, Value: primitive.Regex{Pattern: `^(?:\+[0-9]+|0[0-9]+)$`}}},
		}}}
		if artifact {
			match = bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "source_kind", Value: "artifact"}}, match}}}
		}
		or = append(or, match)
		return nil
	}
	fields := []struct{ field, scope string }{
		{"domain_id", "mongo/" + name}, {"assessment_id", "mysql/assessment"},
		{"outcome_id", "mysql/evaluation_outcome"}, {"generation_id", "mongo/report_generations"},
		{"interpretation_run_id", "mongo/interpretation_runs"}, {"latest_run_id", "mongo/interpretation_runs"},
		{"report_id", "mongo/interpret_report_artifacts"}, {"sort_report_id", "mongo/interpret_report_artifacts"},
		{"task_id", "mysql/assessment_task"}, {"attribution.task_id", "mysql/assessment_task"},
		{"attribution.enrollment_id", "mysql/plan_enrollment"}, {"start_context.id", "mysql/answering_start"},
	}
	if err := add("testee_id", roots, false); err != nil {
		return nil, err
	}
	if name == "answersheets" {
		if err := add("filler_id", users, false); err != nil {
			return nil, err
		}
	}
	for _, field := range fields {
		if err := add(field.field, f.ids[field.scope], false); err != nil {
			return nil, err
		}
	}
	if name == "report_query_catalog" {
		if err := add("source_id", f.ids["mongo/interpret_report_artifacts"], true); err != nil {
			return nil, err
		}
	}
	if len(or) == 0 {
		return nil, errors.New("directed reference has no frozen identity domain")
	}
	filter := bson.D{{Key: "$or", Value: or}}
	raw, err := bson.Marshal(filter)
	if err != nil || len(raw) > graphMongoCommandBytes {
		return nil, errors.Join(err, errors.New("directed reference exceeds fixed Mongo command budget"))
	}
	return filter, nil
}

func graphFrozenStatisticsReferences(ctx context.Context, r graphReader, roots, users map[string]bool, sources map[string]map[string]bool, f graphFrozenReferences, emit func(record) error) error {
	packet := uint64(4 << 20) // bounded synthetic readers; real readers use their server's limit.
	if live, ok := r.(*leanGraphReader); ok {
		if live == nil || live.store == nil || live.store.live == nil {
			return errors.New("directed SQL reader unavailable")
		}
		if err := live.store.live.readSQL().QueryRowContext(ctx, "SELECT @@max_allowed_packet").Scan(&packet); err != nil {
			return err
		}
	}
	for _, table := range []string{"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact"} {
		where, args, err := graphFrozenStatisticsPredicate(table, roots, users, sources, f)
		if err != nil {
			return err
		}
		if packet <= 8192 || uint64(len(where)+len(encode(args))+8192) >= packet || len(encode(args)) > graphBatchBytes {
			return errors.New("directed reference exceeds actual SQL packet budget")
		}
		if err = r.sqlPages(ctx, table, where, args, emit); err != nil {
			return err
		}
	}
	return nil
}

func graphFrozenStatisticsPredicate(table string, roots, users map[string]bool, sources map[string]map[string]bool, f graphFrozenReferences) (string, []any, error) {
	if !leanStatisticsCursorTable(table) {
		return "", nil, errors.New("directed statistics namespace outside fixed catalog")
	}
	predicates, args := []string{}, []any{}
	add := func(column string, ids map[string]bool, text bool, kind string) error {
		if len(ids) == 0 {
			return nil
		}
		for id := range ids {
			if !graphDecimal(id) {
				return errors.New("directed statistics requires exact positive int64 identity")
			}
		}
		encoded, err := json.Marshal(sortedGraphIDs(ids))
		if err != nil {
			return err
		}
		typ, selected := "BIGINT", "ids.id"
		if text {
			typ = "VARCHAR(64)"
			// SQL052's source_ref is utf8mb4_unicode_ci (including PAD SPACE).
			// Preserve that existing IN comparison rather than mixing JSON_TABLE's
			// connection collation or silently changing it to binary equality.
			selected += " COLLATE utf8mb4_unicode_ci"
		}
		predicate := fmt.Sprintf("x.`%s` IN (SELECT %s FROM JSON_TABLE(?, '$[*]' COLUMNS(id %s PATH '$')) AS ids)", column, selected, typ)
		if kind != "" {
			predicate = "(x.source_type=? AND BINARY x.source_type=BINARY ? AND " + predicate + ")"
			args = append(args, kind, kind)
		}
		predicates, args = append(predicates, predicate), append(args, string(encoded))
		return nil
	}
	if err := add("testee_id", roots, false, ""); err != nil {
		return "", nil, err
	}
	if table == "statistics_assessment_fact" {
		if err := add("filler_id", users, false, ""); err != nil {
			return "", nil, err
		}
	}
	kinds := make([]string, 0, len(sources))
	for kind := range sources {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if err := add("source_ref", sources[kind], true, kind); err != nil {
			return "", nil, err
		}
	}
	for _, link := range []struct{ table, column, scope string }{
		{"statistics_assessment_fact", "answersheet_id", "mongo/answersheets"},
		{"statistics_assessment_fact", "assessment_id", "mysql/assessment"},
		{"statistics_assessment_fact", "outcome_id", "mysql/evaluation_outcome"},
		{"statistics_assessment_fact", "report_id", "mongo/interpret_report_artifacts"},
		{"statistics_assessment_fact", "enrollment_id", "mysql/plan_enrollment"},
		{"statistics_assessment_fact", "task_id", "mysql/assessment_task"},
		{"statistics_plan_fact", "enrollment_id", "mysql/plan_enrollment"},
		{"statistics_plan_fact", "task_id", "mysql/assessment_task"},
	} {
		if link.table == table {
			if err := add(link.column, f.ids[link.scope], false, ""); err != nil {
				return "", nil, err
			}
		}
	}
	if len(predicates) == 0 {
		return "", nil, errors.New("directed statistics has no frozen identity domain")
	}
	return "(" + strings.Join(predicates, ") OR (") + ")", args, nil
}
