package main

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type diagnosticBucket struct {
	TypeLabel  string `json:"type_label"`
	TypeHash   string `json:"type_hash"`
	StateLabel string `json:"state_label"`
	StateHash  string `json:"state_hash"`
	Records    uint64 `json:"records"`
}
type diagnosticHistogram struct {
	Database       string             `json:"database"`
	Name           string             `json:"name"`
	Present        *bool              `json:"present"`
	Complete       bool               `json:"complete"`
	Buckets        []diagnosticBucket `json:"buckets"`
	ErrorCategory  string             `json:"error_category"`
	DiagnosticOnly bool               `json:"diagnostic_only"`
}

func typeLabel(s string) string {
	switch s {
	case "request", "change", "cancel", "prepare", "start", "answer",
		"answersheet.submitted", "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed", "interpretation.report.generated", "interpretation.report.failed", "interpretation.retry.requested", "task.opened.reminder.requested",
		"footprint.entry_opened", "footprint.intake_confirmed", "footprint.testee_profile_created", "footprint.care_relationship_established", "footprint.care_relationship_transferred", "footprint.answersheet_submitted", "footprint.assessment_created", "footprint.report_generated",
		"interpretation.ai_explanation.requested", "interpretation.ai_explanation.retry.requested", "interpretation.ai_explanation.lease_recovery.requested", "interpretation.ai_explanation.generated", "interpretation.ai_explanation.failed", "interpretation.ai_explanation.prompt_evaluation.step_requested",
		"assessment.submitted", "assessment.evaluated", "assessment.interpreted", "assessment.failed", "report.generated",
		"questionnaire.changed", "scale.changed", "task.opened", "task.completed", "task.expired", "task.canceled":
		return s
	}
	return "unknown_type"
}
func getMongoDB() string { return os.Getenv("MONGODB_DBNAME") }
func stateLabel(s string) string {
	switch s {
	case "pending", "publishing", "published", "failed", "quarantined", "retry_wait", "0", "1", "historical_mapping":
		return s
	}
	return "unknown_state"
}
func bucket(kind, state string, count uint64) diagnosticBucket {
	return diagnosticBucket{TypeLabel: typeLabel(kind), TypeHash: hashParts("diagnostic_type_v1", kind), StateLabel: stateLabel(state), StateHash: hashParts("diagnostic_state_v1", state), Records: count}
}
func sqlHistogram(ctx context.Context, tx *sql.Tx, name string) (r diagnosticHistogram, err error) {
	r = diagnosticHistogram{Database: "mysql", Name: name, Buckets: []diagnosticBucket{}, ErrorCategory: "none", DiagnosticOnly: true}
	rows, e := scanSQL(ctx, tx, "SELECT TABLE_TYPE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", name)
	if e != nil {
		return r, e
	}
	present := len(rows) != 0
	r.Present = &present
	if !present {
		r.Complete = true
		return r, nil
	}
	if len(rows) != 1 || val(rows[0], 0) != "BASE TABLE" {
		return r, category("histogram_namespace_kind_rejected")
	}
	kindCol, stateCol := "event_type", "status"
	if name == "ai_bridge_commands" {
		kindCol, stateCol = "kind", "delivered"
	}
	if name == "ai_messaging_legacy_commands" {
		kindCol, stateCol = "source_kind", ""
	}
	columns, e := scanSQL(ctx, tx, "SELECT COLUMN_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=?", name)
	if e != nil {
		return r, e
	}
	got := map[string]bool{}
	for _, c := range columns {
		got[val(c, 0)] = true
	}
	if !got[kindCol] || (stateCol != "" && !got[stateCol]) {
		return r, category("histogram_schema_rejected")
	}
	stateExpr := "'historical_mapping'"
	kindExpr := "CAST(" + quote(kindCol) + " AS BINARY)"
	group := kindExpr
	if stateCol != "" {
		stateExpr = "CAST(" + quote(stateCol) + " AS BINARY)"
		group += "," + stateExpr
	}
	counts, e := scanSQL(ctx, tx, "SELECT /*+ MAX_EXECUTION_TIME(15000) */ "+kindExpr+","+stateExpr+",COUNT(*) FROM "+quote(name)+" GROUP BY "+group+" LIMIT 129")
	if e != nil {
		return r, category("histogram_query_failed_or_timed_out")
	}
	if len(counts) > 128 {
		return r, category("histogram_bucket_bound_exceeded")
	}
	for _, c := range counts {
		if len(c) != 3 || c[0] == nil || c[1] == nil {
			return r, category("histogram_schema_rejected")
		}
		n, e := strconv.ParseUint(val(c, 2), 10, 64)
		if e != nil {
			return r, category("histogram_count_invalid")
		}
		r.Buckets = append(r.Buckets, bucket(val(c, 0), val(c, 1), n))
	}
	sort.Slice(r.Buckets, func(i, j int) bool {
		return r.Buckets[i].TypeHash+r.Buckets[i].StateHash < r.Buckets[j].TypeHash+r.Buckets[j].StateHash
	})
	r.Complete = true
	return r, nil
}
func mongoHistogram(ctx context.Context) (r diagnosticHistogram, err error) {
	r = diagnosticHistogram{Database: "mongodb", Name: "domain_event_outbox", Buckets: []diagnosticBucket{}, ErrorCategory: "none", DiagnosticOnly: true}
	client, e := mongoOpen(ctx)
	if e != nil {
		return r, e
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := client.Disconnect(c); err == nil && closeErr != nil {
			err = category("mongo_close_failed")
		}
	}()
	db := client.Database(getMongoDB())
	q, cancel := queryContext(ctx)
	defer cancel()
	cursor, e := db.ListCollections(q, bson.D{{Key: "name", Value: r.Name}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if e != nil {
		return r, category("histogram_metadata_query_failed")
	}
	var collections []bson.Raw
	e = cursor.All(q, &collections)
	closeErr := cursor.Close(q)
	if e != nil || closeErr != nil {
		return r, category("histogram_metadata_query_failed")
	}
	present := len(collections) != 0
	r.Present = &present
	if !present {
		r.Complete = true
		return r, nil
	}
	if len(collections) != 1 || collections[0].Lookup("type").StringValue() != "collection" {
		return r, category("histogram_namespace_kind_rejected")
	}
	// No sample, payload, IDs, $lookup, $merge or $out. A full aggregate counts
	// every row; excess distinct buckets or timeout remains unknown/incomplete.
	pipeline := []bson.D{{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "event_type", Value: "$event_type"}, {Key: "status", Value: "$status"}}}, {Key: "records", Value: bson.D{{Key: "$sum", Value: 1}}}}}}, {{Key: "$limit", Value: 129}}}
	cursor, e = db.Collection(r.Name).Aggregate(q, pipeline, options.Aggregate().SetMaxTime(15*time.Second).SetAllowDiskUse(false).SetCollation(&options.Collation{Locale: "simple"}))
	if e != nil {
		return r, category("histogram_query_failed_or_timed_out")
	}
	var counts []bson.M
	e = cursor.All(q, &counts)
	closeErr = cursor.Close(q)
	if e != nil || closeErr != nil {
		return r, category("histogram_query_failed_or_timed_out")
	}
	if len(counts) > 128 {
		return r, category("histogram_bucket_bound_exceeded")
	}
	for _, c := range counts {
		id, ok := c["_id"].(bson.M)
		if !ok {
			return r, category("histogram_schema_rejected")
		}
		kind, ok := id["event_type"].(string)
		if !ok {
			return r, category("histogram_schema_rejected")
		}
		state, ok := id["status"].(string)
		if !ok {
			return r, category("histogram_schema_rejected")
		}
		var n uint64
		switch v := c["records"].(type) {
		case int32:
			if v < 0 {
				return r, category("histogram_count_invalid")
			}
			n = uint64(v)
		case int64:
			if v < 0 {
				return r, category("histogram_count_invalid")
			}
			n = uint64(v)
		default:
			return r, category("histogram_count_invalid")
		}
		r.Buckets = append(r.Buckets, bucket(kind, state, n))
	}
	sort.Slice(r.Buckets, func(i, j int) bool {
		return r.Buckets[i].TypeHash+r.Buckets[i].StateHash < r.Buckets[j].TypeHash+r.Buckets[j].StateHash
	})
	r.Complete = true
	return r, nil
}
func discoverHistograms(ctx context.Context, sqlReady, mongoReady bool) []diagnosticHistogram {
	result := make([]diagnosticHistogram, 0, 4)
	if sqlReady {
		db, e := mysqlOpen(ctx)
		if e == nil {
			defer func() { _ = db.Close() }()
			tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
			if e == nil {
				defer func() { _ = tx.Rollback() }()
				for _, name := range []string{"domain_event_outbox", "ai_bridge_commands", "ai_messaging_legacy_commands"} {
					r, e := sqlHistogram(ctx, tx, name)
					if e != nil {
						r.ErrorCategory = e.Error()
					}
					result = append(result, r)
				}
			}
		}
	}
	if len(result) != 3 {
		result = []diagnosticHistogram{}
		for _, name := range []string{"domain_event_outbox", "ai_bridge_commands", "ai_messaging_legacy_commands"} {
			result = append(result, diagnosticHistogram{Database: "mysql", Name: name, Buckets: []diagnosticBucket{}, DiagnosticOnly: true, ErrorCategory: "identity_metadata_not_ready"})
		}
	}
	if mongoReady {
		r, e := mongoHistogram(ctx)
		if e != nil {
			r.ErrorCategory = e.Error()
		}
		return append(result, r)
	}
	return append(result, diagnosticHistogram{Database: "mongodb", Name: "domain_event_outbox", Buckets: []diagnosticBucket{}, DiagnosticOnly: true, ErrorCategory: "identity_metadata_not_ready"})
}
