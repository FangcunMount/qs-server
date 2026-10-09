//go:build integration

package evaluation

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"gorm.io/gorm"
)

func cycleNativeInsert(t *testing.T, db *gorm.DB, row historicalSQLRow) {
	t.Helper()
	if err := db.Exec("INSERT INTO rm_outbox(producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,next_attempt_at,version,transport_confirmed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6),?,?)", valueOrEmpty(row["producer"]), []byte(valueOrEmpty(row["message_id"])), valueOrEmpty(row["destination"]), valueOrEmpty(row["event_type"]), valueOrEmpty(row["schema_version"]), valueOrEmpty(row["scope"]), valueOrEmpty(row["content_type"]), valueOrEmpty(row["occurred_at"]), []byte(valueOrEmpty(row["payload"])), []byte(valueOrEmpty(row["fingerprint"])), valueOrEmpty(row["state"]), valueOrEmpty(row["version"]), row["transport_confirmed_at"]).Error; err != nil {
		t.Fatal(err)
	}
}

func cycleNativeObserve(t *testing.T, db *gorm.DB, limits SQLResponsibilityLimits) (*SQLHistoricalResponsibilityCycle, error) {
	t.Helper()
	var cycle *SQLHistoricalResponsibilityCycle
	err := db.Transaction(func(tx *gorm.DB) error {
		server, database, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		cycle, err = PrepareSQLHistoricalResponsibilityCycle(hostmysql.WithTx(t.Context(), tx), sqlHistoricalIdentity(server, database), limits)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return cycle, err
}

func TestSQLResponsibilityCycleBoundedPagingNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	for i := 0; i < 13; i++ {
		id := fmt.Sprintf("paged-%02d", i)
		cycleNativeInsert(t, db, cycleTestMessage(t, id, "evaluation.requested", "Evaluation", "42", cycleRequestedPayload()))
		if db.Exec("INSERT INTO qs_rm_evaluation_request_ref(event_id,assessment_id,org_id) VALUES(?,42,7)", id).Error != nil {
			t.Fatal("synthetic reference insert failed")
		}
	}
	for _, request := range []string{"A", "a", "a\x00", "a ", "z", "Z"} {
		if db.Exec("INSERT INTO qs_rm_replay_requests(org_id,request_id,store_name,reason,input_hash) VALUES(7,?,'assessment-mysql-outbox','synthetic',REPEAT(CHAR(0),32))", []byte(request)).Error != nil {
			t.Fatal("synthetic request insert failed")
		}
		for ordinal := 0; ordinal < 2; ordinal++ {
			if db.Exec("INSERT INTO qs_rm_replay_items(org_id,request_id,ordinal,event_id,expected_failure_count,authorized,reason) VALUES(7,?,?,'paged-00',0,0,'not_manual_required')", []byte(request), ordinal).Error != nil {
				t.Fatal("synthetic item insert failed")
			}
		}
	}
	limits := DefaultSQLResponsibilityLimits()
	limits.PageRows = 3
	cycle, err := cycleNativeObserve(t, db, limits)
	if err != nil {
		t.Fatal(err)
	}
	report := cycle.Report()
	if len(report.Ledgers) != 8 || report.Observed != 44 || report.Blocking != 0 || !report.ActualTransactionReadOnlyRR || !report.WriterFenceRequired || report.DropReady {
		t.Fatalf("complete bounded native observation wrong counts/status: observed=%d blocking=%d", report.Observed, report.Blocking)
	}
	if report.Ledgers[0].Rows != 13 || report.Ledgers[0].Pages != 5 || report.Ledgers[6].Rows != 12 || report.Ledgers[6].Pages != 4 || len(cycle.ForEvent("paged-00")) != 14 {
		t.Fatal("keyset pages skipped binary/composite PK or exact event references")
	}
	if len(cycle.ForAssessment(42)) != 38 {
		t.Fatal("cached owner index incomplete")
	}
	t.Log("8 exact-schema ledgers scanned once with binary/composite fixed upper pagination; source row hash and SDK fingerprint retained separately")
}

func TestSQLResponsibilityCycleCompositeIndexRangeNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	// A large, single-organization prefix prevents a first-column-only range
	// from looking selective. Inspect the actual production page predicate.
	for first := 0; first < 10_000; first += 250 {
		var parents, items []string
		var parentArgs, itemArgs []any
		for i := first; i < first+250; i++ {
			id := []byte(fmt.Sprintf("range-%05d", i))
			parents = append(parents, "(7,?,'assessment-mysql-outbox','synthetic',REPEAT(CHAR(0),32))")
			parentArgs = append(parentArgs, id)
			for ordinal := 0; ordinal < 2; ordinal++ {
				items = append(items, "(7,?,?,'synthetic-event',0,0,'not_found')")
				itemArgs = append(itemArgs, id, ordinal)
			}
		}
		if err := db.Exec("INSERT INTO qs_rm_replay_requests(org_id,request_id,store_name,reason,input_hash) VALUES "+strings.Join(parents, ","), parentArgs...).Error; err != nil {
			t.Fatal("owned range parents insert failed")
		}
		if err := db.Exec("INSERT INTO qs_rm_replay_items(org_id,request_id,ordinal,event_id,expected_failure_count,authorized,reason) VALUES "+strings.Join(items, ","), itemArgs...).Error; err != nil {
			t.Fatal("owned range items insert failed")
		}
	}
	for _, spec := range sqlResponsibilityTables[5:7] {
		upper := []string{"7", "range-09999"}
		after := []string{"7", "range-09499"}
		if len(spec.keys) == 3 {
			upper = append(upper, "1")
			after = append(after, "1")
		}
		predicate, args := cycleKeyPredicate(spec, upper, true)
		lower, lowerArgs := cycleKeyPredicate(spec, after, false)
		predicate += " AND " + lower
		args = append(args, lowerArgs...)
		args = append(args, 3)
		query := "SELECT * FROM `" + spec.name + "` WHERE " + predicate + " ORDER BY " + strings.Join(spec.keys, ",") + " LIMIT ?"
		rows, _, _, err := cycleQuery(db, "EXPLAIN FORMAT=JSON "+query, 1, args...)
		if err != nil || len(rows) != 1 || rows[0]["EXPLAIN"] == nil {
			t.Fatal("owned range explain failed", err)
		}
		var plan map[string]any
		if json.Unmarshal([]byte(*rows[0]["EXPLAIN"]), &plan) != nil {
			t.Fatal("owned range explain invalid")
		}
		var table map[string]any
		var find func(any)
		find = func(value any) {
			switch node := value.(type) {
			case map[string]any:
				if node["table_name"] == spec.name {
					table = node
				}
				for _, child := range node {
					find(child)
				}
			case []any:
				for _, child := range node {
					find(child)
				}
			}
		}
		find(plan)
		parts, _ := table["used_key_parts"].([]any)
		t.Logf("fixed page actual plan store=%s access_type=%v key=%v used_key_parts=%v rows_examined_per_scan=%v", spec.name, table["access_type"], table["key"], parts, table["rows_examined_per_scan"])
		if table["access_type"] != "range" || table["key"] != "PRIMARY" || len(parts) != len(spec.keys) {
			t.Fatal("fixed page is not a full composite primary-key index range")
		}
		for i, part := range parts {
			if part != spec.keys[i] {
				t.Fatal("fixed page index range does not cover actual ordered PK")
			}
		}
		page, _, _, err := cycleQuery(db, query, 3, args...)
		if err != nil || len(page) != 3 || valueOrEmpty(page[0]["request_id"]) != "range-09500" {
			t.Fatal("fixed page range result is incorrect", err)
		}
	}
}

func TestSQLResponsibilityCycleActualROAndVisibilityNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	for _, opts := range []*sql.TxOptions{{Isolation: sql.LevelRepeatableRead}, {Isolation: sql.LevelReadCommitted, ReadOnly: true}} {
		err := db.Transaction(func(tx *gorm.DB) error {
			s, d, e := historicalDatabase(tx)
			if e != nil {
				return e
			}
			_, e = PrepareSQLHistoricalResponsibilityCycle(hostmysql.WithTx(t.Context(), tx), sqlHistoricalIdentity(s, d), DefaultSQLResponsibilityLimits())
			if !errors.Is(e, ErrSQLResponsibilityTransaction) {
				t.Fatalf("actual wrong mode not rejected: %v", e)
			}
			return nil
		}, opts)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("UPDATE performance_schema.setup_consumers SET ENABLED='NO' WHERE NAME='events_transactions_current'").Error; err != nil {
		t.Fatal("local fixture instrumentation change failed")
	}
	defer func() {
		if err := db.Exec("UPDATE performance_schema.setup_consumers SET ENABLED='YES' WHERE NAME='events_transactions_current'").Error; err != nil {
			t.Error("local fixture instrumentation restore failed")
		}
	}()
	cycle, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if cycle != nil || !errors.Is(err, ErrSQLResponsibilityVisibility) {
		t.Fatal("invisible correct host Tx was misclassified or accepted", err)
	}
	t.Log("actual ACTIVE transaction mode verified; missing instrumentation classified as visibility gap, no session fallback")
}

func TestSQLResponsibilityCycleFreshFullHashAndAboveUpperNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	for _, id := range []string{"old-one", "old-two"} {
		cycleNativeInsert(t, db, cycleTestMessage(t, id, "evaluation.failed", "Evaluation", "42", eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, Reason: "synthetic", FailedAt: time.Now().UTC()}))
	}
	cycle, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		fresh, e := cycle.RecheckFresh(hostmysql.WithTx(t.Context(), tx))
		if e != nil || !fresh.Identical || fresh.AboveUpperRows != 0 || fresh.DropReady {
			t.Fatal("same actual data new snapshot mismatch", e)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		s, d, e := historicalDatabase(tx)
		if e != nil {
			return e
		}
		observed, e := PrepareSQLHistoricalResponsibilityCycle(hostmysql.WithTx(t.Context(), tx), sqlHistoricalIdentity(s, d), DefaultSQLResponsibilityLimits())
		if e != nil {
			return e
		}
		_, e = observed.RecheckFresh(hostmysql.WithTx(t.Context(), tx))
		if !errors.Is(e, ErrSQLResponsibilityFresh) {
			t.Fatal("old RR snapshot accepted as fresh")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"UPDATE rm_outbox SET last_error_code='changed-below-upper' WHERE id=1", "DELETE FROM rm_outbox WHERE id=1"} {
		if db.Exec(mutation).Error != nil {
			t.Fatal("synthetic below-bound mutation failed")
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			fresh, e := cycle.RecheckFresh(hostmysql.WithTx(t.Context(), tx))
			if !errors.Is(e, ErrSQLResponsibilityChanged) || fresh.Identical || fresh.AboveUpperRows != 0 || len(fresh.ChangedStores) != 1 {
				t.Fatal("below-bound edit/deletion missed", e)
			}
			return nil
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
			t.Fatal(err)
		}
	}
	cycleNativeInsert(t, db, cycleTestMessage(t, "fresh-above", "evaluation.failed", "Evaluation", "42", eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, Reason: "synthetic", FailedAt: time.Now().UTC()}))
	if err := db.Transaction(func(tx *gorm.DB) error {
		fresh, e := cycle.RecheckFresh(hostmysql.WithTx(t.Context(), tx))
		if !errors.Is(e, ErrSQLResponsibilityChanged) || fresh.AboveUpperRows != 1 {
			t.Fatal("fresh above-bound responsibility missed", e)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	t.Log("new actual snapshot rescans full schema and row hashes, detecting edits/deletes below upper and insert above it")
}

func TestSQLResponsibilityCycleCapsAndSchemaNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	cycleNativeInsert(t, db, cycleTestMessage(t, "one", "evaluation.failed", "Evaluation", "42", eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, Reason: "synthetic", FailedAt: time.Now().UTC()}))
	cycleNativeInsert(t, db, cycleTestMessage(t, "two", "evaluation.failed", "Evaluation", "42", eventpayload.EvaluationFailedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, Reason: "synthetic", FailedAt: time.Now().UTC()}))
	for _, limits := range []SQLResponsibilityLimits{{PageRows: 1, MaxRows: 1, MaxBytes: 1 << 20, MaxRetainedBytes: 1 << 20}, {PageRows: 1, MaxRows: 100, MaxBytes: 1, MaxRetainedBytes: 1 << 20}, {PageRows: 1, MaxRows: 100, MaxBytes: 1 << 20, MaxRetainedBytes: 1}} {
		c, e := cycleNativeObserve(t, db, limits)
		if c != nil || !errors.Is(e, ErrSQLResponsibilityBounds) {
			t.Fatal("cap truncated into complete cycle", e)
		}
	}
	cycle, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil {
		t.Fatal(err)
	}
	if db.Exec("ALTER TABLE rm_outbox ADD COLUMN future_unknown VARBINARY(10) NULL").Error != nil {
		t.Fatal("owned future column DDL failed")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		fresh, e := cycle.RecheckFresh(hostmysql.WithTx(t.Context(), tx))
		if !errors.Is(e, ErrSQLResponsibilityChanged) || fresh.Identical {
			t.Fatal("future column schema change ignored", e)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	if db.Exec("ALTER TABLE qs_rm_evaluation_request_ref DROP PRIMARY KEY").Error != nil {
		t.Fatal("owned PK change failed")
	}
	c, e := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if c != nil || !errors.Is(e, ErrSQLResponsibilitySchema) {
		t.Fatal("non-PK pagination accepted", e)
	}
}

func TestSQLResponsibilityCycleGlobalInvalidAndOutsideScopeNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	other := eventpayload.TaskOpenedReminderRequestedData{TaskID: "task-one", PlanID: "plan-one", OrgID: 7, TesteeID: "21", OpenAt: time.Now().UTC(), ScheduleRevision: 1}
	row := cycleTestMessage(t, "other-pending", "task.opened.reminder.requested", "AssessmentTask", "task-one", other)
	row["state"] = pointerValue("pending")
	row["transport_confirmed_at"] = nil
	cycleNativeInsert(t, db, row)
	clean, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil || clean.Report().Blocking != 0 || clean.Report().OutsideRetirement != 1 || clean.UnboundEventCount() != 1 {
		t.Fatal("current other business backlog treated as retired blocker", err)
	}
	bad := cycleTestMessage(t, "unknown", "unknown.type", "Evaluation", "42", cycleRequestedPayload())
	cycleNativeInsert(t, db, bad)
	orphan := cycleRequestedPayload()
	orphan.AssessmentID = 999
	cycleNativeInsert(t, db, cycleTestMessage(t, "orphan", "evaluation.requested", "Evaluation", "999", orphan))
	if db.Exec("INSERT INTO qs_rm_evaluation_request_ref(event_id,assessment_id,org_id) VALUES('orphan',999,7),('missing-current',42,8)").Error != nil {
		t.Fatal("synthetic wrong owner ref failed")
	}
	tamper := cycleTestMessage(t, "tampered", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload())
	tamper["fingerprint"] = pointerValue(string(make([]byte, 32)))
	cycleNativeInsert(t, db, tamper)
	copyRow := cycleTestMessage(t, "duplicate", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload())
	cycleNativeInsert(t, db, copyRow)
	copyRow["destination"] = pointerValue("different-destination")
	cycleNativeInsert(t, db, copyRow)
	observed, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil {
		t.Fatal(err)
	}
	if observed.Report().Observed != 8 || observed.Report().Blocking < 7 {
		t.Fatal("global rows hidden by org/JOIN or SDK fingerprint bypassed")
	}
	for _, id := range []string{"unknown", "orphan", "missing-current", "tampered", "duplicate"} {
		badCount := 0
		for _, v := range observed.ForEvent(id) {
			if v.Invalid {
				badCount++
			}
		}
		if badCount == 0 {
			t.Fatal("global invalid identity not exposed", id)
		}
	}
	t.Log("unknown wire/type, SDK tamper, cross organization, orphan and duplicate references observed globally without organization/JOIN filtering")
}

func TestSQLResponsibilityCycleActualOutcomeOrphanNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	p := eventpayload.EvaluationOutcomeCommittedData{OrgID: 7, AssessmentID: 42, TesteeID: 21, OutcomeID: "999", EvaluationRunID: "42:1", CommittedAt: time.Now().UTC()}
	cycleNativeInsert(t, db, cycleTestMessage(t, "orphan-outcome", "evaluation.outcome.committed", "Evaluation", "42", p))
	cycle, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil {
		t.Fatal(err)
	}
	rows := cycle.ForEvent("orphan-outcome")
	if len(rows) != 1 || !rows[0].Invalid {
		t.Fatal("existing Assessment hid missing exact original Outcome")
	}
}

func TestSQLResponsibilityCycleFreshOwnerAndHeldWireNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	row := cycleTestMessage(t, "held-original", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload())
	if err := db.Exec("INSERT INTO retry_event_hold(event_id,message_id,org_id,provider,topic_name,channel_name,payload_json,original_delivery_attempt,blocked_reason,blocked_at,status,retry_disposition,replayed_at) VALUES('held-original','synthetic-transport',7,'nsq','synthetic-topic','synthetic-channel',?,1,'synthetic',UTC_TIMESTAMP(3),'replayed',NULL,UTC_TIMESTAMP(3))", valueOrEmpty(row["payload"])).Error; err != nil {
		t.Fatal(err)
	}
	cycle, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil || cycle.Report().Blocking != 0 {
		t.Fatal("valid current held original rejected", err)
	}
	if err := db.Exec("UPDATE assessment SET org_id=8 WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		fresh, e := cycle.RecheckFresh(hostmysql.WithTx(t.Context(), tx))
		if !errors.Is(e, ErrSQLResponsibilityChanged) || fresh.Identical || len(fresh.ChangedStores) != 1 || fresh.ChangedStores[0] != "business_owner_anchors" {
			t.Fatal("unchanged ledger masked changed actual owner", e)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	if db.Exec("UPDATE assessment SET org_id=7 WHERE id=42").Error != nil {
		t.Fatal("owner restore failed")
	}
	outer, recognized, e := legacy.Decode([]byte(valueOrEmpty(row["payload"])))
	if e != nil || !recognized {
		t.Fatal("synthetic outer decode failed")
	}
	outer.Metadata["occurred_at"] = "wrong-original-clock"
	mutated, e := legacy.Encode(outer, legacy.Revision2)
	if e != nil {
		t.Fatal(e)
	}
	if db.Exec("UPDATE retry_event_hold SET payload_json=? WHERE id=1", string(mutated)).Error != nil {
		t.Fatal("held synthetic mutation failed")
	}
	current, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil || current.Report().Blocking != 1 || !current.ForEvent("held-original")[0].Invalid {
		t.Fatal("canonical but conflicting outer clock accepted", err)
	}
	t.Log("actual owner anchor edits and held outer/inner metadata conflicts detected independently of standard fingerprint/source row hash")
}

func TestSQLResponsibilityCycleConstraintAndGlobalActionNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	for _, action := range []string{"interpretation.readmit_outcome", "cache.manual_warmup", "unknown-future-action"} {
		if db.Exec("INSERT INTO system_governance_action_runs(request_id,action_id,org_id,input_json,status,started_at) VALUES(?,?,7,JSON_OBJECT(),'running',UTC_TIMESTAMP(3))", action, action).Error != nil {
			t.Fatal("owned action insert failed")
		}
	}
	cycle, err := cycleNativeObserve(t, db, DefaultSQLResponsibilityLimits())
	if err != nil {
		t.Fatal(err)
	}
	if cycle.Report().Blocking != 1 || cycle.Report().OutsideRetirement != 1 || cycle.Report().Unknown != 1 || cycle.UnboundEventCount() != 1 || len(cycle.ForOrganizationActions(7)) != 2 {
		t.Fatal("unknown/re-admission responsibility hidden when no current owner message exists")
	}
	if db.Exec("ALTER TABLE rm_outbox ADD CONSTRAINT synthetic_version_check CHECK(version>=0)").Error != nil {
		t.Fatal("owned CHECK DDL failed")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		fresh, e := cycle.RecheckFresh(hostmysql.WithTx(t.Context(), tx))
		if !errors.Is(e, ErrSQLResponsibilityChanged) || fresh.Identical || len(fresh.ChangedStores) != 1 || fresh.ChangedStores[0] != "rm_outbox" {
			t.Fatal("CHECK-only schema modification omitted", e)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	t.Log("SHOW CREATE CHECK-only change and current re-admission/unknown organization-wide actions observed without business JOIN or current owner prerequisite")
}
