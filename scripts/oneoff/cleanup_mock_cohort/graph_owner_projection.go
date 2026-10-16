package main

import (
	"context"
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// These lookups only feed the existing typed owner maps. Complete discovery,
// fingerprints and capture/restore continue through the unprojected readers.
func (s *graphStore) sqlOwnerPages(ctx context.Context, name string, ids []string, emit func(record) error) error {
	columns := map[string][]string{
		"assessment_entry_intake_log": {"id", "org_id", "testee_id"},
		"clinician_relation":          {"id", "org_id", "testee_id"},
		"assessment":                  {"id", "org_id", "testee_id"},
		"plan_enrollment":             {"id", "org_id", "testee_id", "plan_id"},
		"assessment_task":             {"id", "org_id", "testee_id", "assessment_id", "plan_id", "enrollment_id"},
		"evaluation_outcome":          {"id", "org_id", "testee_id", "assessment_id"},
	}[name]
	if len(columns) == 0 {
		return errors.New("SQL ownership namespace outside fixed lookup catalog")
	}
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 1000 {
		return errors.New("ownership lookup exceeds fixed SQL batch")
	}
	selected := make([]string, len(columns))
	for i, column := range columns {
		selected[i] = "x.`" + column + "`"
	}
	slots, args := placeholders(ids)
	return s.sqlPagesSelected(ctx, name, "x.id IN ("+slots+")", args, strings.Join(selected, ","), emit)
}

func (s *graphStore) mongoOwnerPages(ctx context.Context, name string, ids []string, emit func(record) error) error {
	switch name {
	case "answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts":
	default:
		return errors.New("mongo ownership namespace outside fixed lookup catalog")
	}
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 500 {
		return errors.New("mongo link ownership batch exceeds fixed 500")
	}
	// Keep all nine fields the old decoder inspected, even where a field is
	// normally absent. Unexpected wrong types must not disappear in projection.
	projection := bson.D{}
	for _, key := range []string{"_id", "domain_id", "org_id", "testee_id", "assessment_id", "outcome_id", "generation_id", "interpretation_run_id", "report_id", "latest_run_id"} {
		projection = append(projection, bson.E{Key: key, Value: 1})
	}
	return s.mongoPagesSelected(ctx, name, bson.D{{Key: "domain_id", Value: bson.D{{Key: "$in", Value: graphMongoIDs(ids)}}}}, projection, emit)
}
