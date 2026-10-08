package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"slices"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

var batchBusinessTables = []string{"assessment", "runtime_checkpoint", "evaluation_outcome"}

func (b *SQLHistoricalOwnerBatch) read(ctx context.Context, tx *gorm.DB, table, index, predicate string, args ...any) error {
	if err := b.cycle.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	remaining := b.limits.MaxRows - int(b.report.Rows)
	if remaining <= 0 {
		return ErrSQLHistoricalBatchBounds
	}
	query := "SELECT * FROM `" + table + "` FORCE INDEX (`" + index + "`) WHERE " + predicate + " ORDER BY id LIMIT ?"
	rows, names, bytes, err := cycleQuery(tx, query, remaining, append(args, remaining+1)...)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(names, b.columns[table]) {
		return ErrSQLHistoricalBatchConflict
	}
	if bytes > b.limits.MaxBytes-b.report.Bytes {
		return ErrSQLHistoricalBatchBounds
	}
	b.rows[table] = rows
	b.report.Rows += uint64(len(rows))
	b.report.Bytes += bytes
	return nil
}

func (b *SQLHistoricalOwnerBatch) capture(ctx context.Context) error {
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || sqlHistoricalIdentity(server, database) != b.report.DatabaseIdentitySHA256 {
		return ErrSQLHistoricalFactsIdentity
	}
	for _, name := range batchBusinessTables {
		columns, schema, _, e := cycleSchema(tx, sqlResponsibilityTable{name: name, keys: []string{"id"}})
		if e != nil {
			return e
		}
		b.columns[name], b.schema[name] = columns, schema
	}
	// All association and owner predicates are global, without an organization
	// or soft-delete filter that could hide another owner of the same sheet.
	if len(b.request.AnswerSheetIDs) > 0 {
		if err = b.verifyIndex(tx, "assessment", "uk_answer_sheet_id", "answer_sheet_id", true); err != nil {
			return err
		}
	}
	if err = b.verifyIndex(tx, "runtime_checkpoint", "idx_runtime_checkpoint_assessment_id", "assessment_id", false); err != nil {
		return err
	}
	if err = b.verifyIndex(tx, "evaluation_outcome", "uk_evaluation_outcome_assessment_id", "assessment_id", true); err != nil {
		return err
	}
	var selected []historicalSQLRow
	if len(b.request.AssessmentIDs) > 0 {
		if err = b.read(ctx, tx, "assessment", "PRIMARY", "id IN ?", b.request.AssessmentIDs); err != nil {
			return err
		}
		selected = append(selected, b.rows["assessment"]...)
	}
	if len(b.request.AnswerSheetIDs) > 0 {
		if err = b.read(ctx, tx, "assessment", "uk_answer_sheet_id", "answer_sheet_id IN ?", b.request.AnswerSheetIDs); err != nil {
			return err
		}
		selected = append(selected, b.rows["assessment"]...)
	}
	// Two exact indexed reads avoid OR/ORDER/LIMIT choosing a full PK scan.
	// An owner requested by both IDs must have exactly identical raw columns.
	byID := map[uint64]historicalSQLRow{}
	for _, row := range selected {
		id, e := sqlHistoricalUint(row, "id")
		if e != nil {
			return e
		}
		if prior := byID[id]; prior != nil && !reflect.DeepEqual(prior, row) {
			return ErrSQLHistoricalBatchConflict
		}
		byID[id] = row
	}
	selected = nil
	selectedIDs := make([]uint64, 0, len(byID))
	for id := range byID {
		selectedIDs = append(selectedIDs, id)
	}
	slices.Sort(selectedIDs)
	for _, id := range selectedIDs {
		selected = append(selected, byID[id])
	}
	b.rows["assessment"] = selected
	ids := make([]uint64, 0, len(b.rows["assessment"]))
	for _, row := range b.rows["assessment"] {
		id, e := sqlHistoricalUint(row, "id")
		sheet, a := sqlHistoricalUint(row, "answer_sheet_id")
		if e != nil || a != nil || row["deleted_at"] != nil || b.owners[id] != nil || (!slices.Contains(b.request.AssessmentIDs, id) && !slices.Contains(b.request.AnswerSheetIDs, sheet)) {
			return ErrSQLHistoricalBatchConflict
		}
		if prior := b.sheets[sheet]; prior != 0 && prior != id {
			return ErrSQLHistoricalBatchConflict
		}
		b.sheets[sheet] = id
		b.owners[id] = &SQLHistoricalOwnerFacts{identity: b.report.DatabaseIdentitySHA256, assessmentID: id, outcomeRecords: map[uint64]*evaluationfact.Record{}}
		ids = append(ids, id)
	}
	if len(ids) > b.limits.MaxOwners {
		return ErrSQLHistoricalBatchBounds
	}
	for _, id := range b.request.AssessmentIDs {
		if b.owners[id] == nil {
			b.absent[id] = true
			b.report.AssessmentAbsentCount++
		}
	}
	for _, id := range b.request.AnswerSheetIDs {
		if b.sheets[id] == 0 {
			b.absentSheet[id] = true
			b.report.AnswerSheetAbsentCount++
		}
	}
	if len(ids) > 0 {
		for _, pair := range [][2]string{{"runtime_checkpoint", "idx_runtime_checkpoint_assessment_id"}, {"evaluation_outcome", "uk_evaluation_outcome_assessment_id"}} {
			if err = b.read(ctx, tx, pair[0], pair[1], "assessment_id IN ?", ids); err != nil {
				return err
			}
		}
	} else {
		b.rows["runtime_checkpoint"], b.rows["evaluation_outcome"] = []historicalSQLRow{}, []historicalSQLRow{}
	}
	clockRows, _, _, err := cycleQuery(tx, "SELECT TABLE_NAME AS table_name,COLUMN_NAME AS column_name,DATA_TYPE AS data_type,DATETIME_PRECISION AS datetime_precision,COLUMN_TYPE AS column_type,IS_NULLABLE AS is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND ((table_name='assessment' AND column_name IN ('submitted_at','evaluated_at','failed_at')) OR (table_name='runtime_checkpoint' AND column_name IN ('finished_at','lease_expires_at','next_attempt_at')) OR (table_name='evaluation_outcome' AND column_name='evaluated_at')) ORDER BY TABLE_NAME,COLUMN_NAME", 7)
	if err != nil || len(clockRows) != 7 {
		return ErrSQLHistoricalBatchInvalid
	}
	b.rows["business_clock_columns"] = clockRows
	if err = b.decodeOwners(tx, ids, clockRows); err != nil {
		return err
	}
	for _, name := range batchBusinessTables {
		_, schema, _, e := cycleSchema(tx, sqlResponsibilityTable{name: name, keys: []string{"id"}})
		if e != nil || schema != b.schema[name] {
			return ErrSQLHistoricalBatchConflict
		}
	}
	if err = b.cycle.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	b.report.OwnerCount = uint64(len(ids))
	b.report.BusinessRowsSHA256 = b.rowsHash()
	return nil
}

func (b *SQLHistoricalOwnerBatch) verifyIndex(tx *gorm.DB, table, index, column string, unique bool) error {
	rows, _, _, err := cycleQuery(tx, "SELECT INDEX_NAME AS index_name,NON_UNIQUE AS non_unique,SEQ_IN_INDEX AS seq_in_index,COLUMN_NAME AS column_name,SUB_PART AS sub_part,COLLATION AS collation,INDEX_TYPE AS index_type,IS_VISIBLE AS is_visible,EXPRESSION AS expression FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=? ORDER BY SEQ_IN_INDEX", 2, table, index)
	if err != nil || len(rows) != 1 {
		return ErrSQLHistoricalBatchConflict
	}
	r := rows[0]
	nonUnique := "1"
	if unique {
		nonUnique = "0"
	}
	if valueOrEmpty(r["column_name"]) != column || valueOrEmpty(r["seq_in_index"]) != "1" || valueOrEmpty(r["non_unique"]) != nonUnique || r["sub_part"] != nil || valueOrEmpty(r["collation"]) != "A" || valueOrEmpty(r["index_type"]) != "BTREE" || valueOrEmpty(r["is_visible"]) != "YES" || r["expression"] != nil {
		return ErrSQLHistoricalBatchConflict
	}
	b.rows["verified_index:"+table+":"+index] = rows
	return nil
}

func (b *SQLHistoricalOwnerBatch) decodeOwners(tx *gorm.DB, ids []uint64, clocks []historicalSQLRow) error {
	for _, row := range b.rows["assessment"] {
		id, _ := sqlHistoricalUint(row, "id")
		if err := batchDecodeOwner(b.owners[id], row, clocks); err != nil {
			return err
		}
	}
	for _, row := range b.rows["runtime_checkpoint"] {
		id, err := sqlHistoricalUint(row, "assessment_id")
		facts := b.owners[id]
		if err != nil || facts == nil || len(facts.snapshot.Runs) >= SQLHistoricalOwnerRowLimit {
			return ErrSQLHistoricalBatchBounds
		}
		run, err := batchDecodeRun(row)
		if err != nil {
			return err
		}
		facts.snapshot.Runs = append(facts.snapshot.Runs, run)
	}
	var pos []EvaluationOutcomePO
	if len(ids) > 0 {
		if err := tx.Raw("SELECT * FROM evaluation_outcome FORCE INDEX (uk_evaluation_outcome_assessment_id) WHERE assessment_id IN ? ORDER BY id LIMIT ?", ids, b.limits.MaxRows+1).Scan(&pos).Error; err != nil {
			return ErrSQLHistoricalFactsRead
		}
	}
	if len(pos) != len(b.rows["evaluation_outcome"]) {
		return ErrSQLHistoricalBatchConflict
	}
	for i, row := range b.rows["evaluation_outcome"] {
		id, err := sqlHistoricalUint(row, "assessment_id")
		facts := b.owners[id]
		po := &pos[i]
		rowID, e := sqlHistoricalUint(row, "id")
		org, a := sqlHistoricalUint(row, "org_id")
		testee, c := sqlHistoricalUint(row, "testee_id")
		at, d := sqlHistoricalTime(row, "evaluated_at")
		if err != nil || e != nil || a != nil || c != nil || d != nil || at == nil || facts == nil || po.ID != rowID || po.AssessmentID != id || po.OrgID <= 0 || uint64(po.OrgID) != org || po.TesteeID != testee || po.EvaluationRunID != valueOrEmpty(row["evaluation_run_id"]) || !po.EvaluatedAt.Equal(*at) || org != facts.snapshot.Owner.OrgID || testee != facts.snapshot.Owner.TesteeID {
			return ErrSQLHistoricalBatchConflict
		}
		record, err := outcomeFromPO(po)
		v := SQLHistoricalOutcome{ID: rowID, AssessmentID: id, OrgID: org, TesteeID: testee, RunID: po.EvaluationRunID, EvaluatedAt: *at, Invalid: err != nil}
		if err == nil {
			facts.outcomeRecords[rowID] = sqlHistoricalFactRecord(record)
		}
		facts.snapshot.Outcomes = append(facts.snapshot.Outcomes, v)
	}
	for id, facts := range b.owners {
		for _, current := range b.cycle.ForAssessment(id) {
			facts.snapshot.Responsibilities = append(facts.snapshot.Responsibilities, SQLHistoricalResponsibility{Store: current.Store, ID: current.PrimaryKeySHA256, EventID: current.EventID, EventType: current.EventType, State: current.State, OrgID: current.OrgID, AssessmentID: current.AssessmentID, TesteeID: current.TesteeID, LeasePresent: current.LeasePresent, Unfinished: current.Unfinished, Invalid: current.Invalid || current.OwnerUnproven})
		}
	}
	return nil
}

func batchDecodeOwner(f *SQLHistoricalOwnerFacts, row historicalSQLRow, clocks []historicalSQLRow) error {
	o := &f.snapshot.Owner
	o.AssessmentID, o.ClockComparisonRuleVersion = f.assessmentID, SQLHistoricalClockComparisonRuleVersion
	var err error
	if o.OrgID, err = sqlHistoricalUint(row, "org_id"); err != nil {
		return err
	}
	if o.TesteeID, err = sqlHistoricalUint(row, "testee_id"); err != nil {
		return err
	}
	if o.AnswerSheetID, err = sqlHistoricalUint(row, "answer_sheet_id"); err != nil {
		return err
	}
	o.Status, o.QuestionnaireCode, o.QuestionnaireVersion = valueOrEmpty(row["status"]), valueOrEmpty(row["questionnaire_code"]), valueOrEmpty(row["questionnaire_version"])
	o.ModelKind, o.ModelAlgorithm, o.ModelCode, o.ModelVersion = valueOrEmpty(row["evaluation_model_kind"]), valueOrEmpty(row["evaluation_model_algorithm"]), valueOrEmpty(row["evaluation_model_code"]), valueOrEmpty(row["evaluation_model_version"])
	o.OriginType = valueOrEmpty(row["origin_type"])
	if row["origin_id"] != nil {
		value := *row["origin_id"]
		o.OriginID = &value
	}
	context, exists := row["conducting_context"]
	if !exists {
		return ErrSQLHistoricalBatchInvalid
	}
	o.ConductingContextPresent = context != nil
	if context != nil {
		o.ConductingContextBytes = []byte(*context)
		o.ConductingContextSHA256 = evidence.SourceDigest("assessment-conducting-context/v1", o.ConductingContextBytes).SHA256
	}
	if o.SubmittedAt, err = sqlHistoricalTime(row, "submitted_at"); err != nil {
		return err
	}
	if o.EvaluatedAt, err = sqlHistoricalTime(row, "evaluated_at"); err != nil {
		return err
	}
	if o.FailedAt, err = sqlHistoricalTime(row, "failed_at"); err != nil {
		return err
	}
	if row["failure_reason"] != nil {
		o.FailureReasonSHA256 = evidence.SourceDigest("sql-owner-failure-reason/v1", []byte(*row["failure_reason"])).SHA256
	}
	for _, clock := range clocks {
		dataType, precisionText := valueOrEmpty(clock["data_type"]), valueOrEmpty(clock["datetime_precision"])
		precision, err := strconv.Atoi(precisionText)
		if err != nil || dataType != "datetime" || strconv.Itoa(precision) != precisionText || (precision != 0 && precision != 3 && precision != 6) || valueOrEmpty(clock["table_name"]) != "assessment" && precision == 0 {
			return ErrSQLHistoricalBatchInvalid
		}
		if valueOrEmpty(clock["table_name"]) != "assessment" {
			continue
		}
		switch valueOrEmpty(clock["column_name"]) {
		case "submitted_at":
			o.ActualSubmittedAtDataType, o.ActualSubmittedAtPrecision = dataType, precision
		case "evaluated_at":
			o.ActualEvaluatedAtDataType, o.ActualEvaluatedAtPrecision = dataType, precision
		case "failed_at":
			o.ActualFailedAtDataType, o.ActualFailedAtPrecision = dataType, precision
		default:
			return ErrSQLHistoricalBatchInvalid
		}
	}
	return nil
}

func batchDecodeRun(row historicalSQLRow) (SQLHistoricalRun, error) {
	r := SQLHistoricalRun{ResourceID: valueOrEmpty(row["resource_id"]), Scope: valueOrEmpty(row["scope"]), Status: valueOrEmpty(row["status"]), Origin: valueOrEmpty(row["attempt_origin"]), RetryDisposition: valueOrEmpty(row["retry_disposition"]), RetryEventID: valueOrEmpty(row["retry_event_id"]), ActionRequestID: valueOrEmpty(row["action_request_id"])}
	var err error
	if r.ID, err = sqlHistoricalUint(row, "id"); err != nil {
		return r, err
	}
	if r.AssessmentID, err = sqlHistoricalUint(row, "assessment_id"); err != nil {
		return r, err
	}
	attempt, err := sqlHistoricalUint(row, "attempt_no")
	if err != nil || attempt > 1<<32-1 || valueOrEmpty(row["retryable"]) != "0" && valueOrEmpty(row["retryable"]) != "1" {
		return r, ErrSQLHistoricalBatchInvalid
	}
	r.Attempt, r.Retryable = uint(attempt), valueOrEmpty(row["retryable"]) == "1"
	if r.FinishedAt, err = sqlHistoricalTime(row, "finished_at"); err != nil {
		return r, err
	}
	if r.LeaseExpiresAt, err = sqlHistoricalTime(row, "lease_expires_at"); err != nil {
		return r, err
	}
	if r.NextAttemptAt, err = sqlHistoricalTime(row, "next_attempt_at"); err != nil {
		return r, err
	}
	if row["deleted_at"] != nil {
		r.Status = "deleted"
	}
	return r, nil
}

func (b *SQLHistoricalOwnerBatch) rowsHash() string {
	h := sha256.New()
	cycleFrame(h, []byte("sql-business-owner-batch-raw/v1"), true)
	keys := make([]string, 0, len(b.rows))
	for key := range b.rows {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		cycleFrame(h, []byte(key), true)
		cycleFrame(h, []byte(b.schema[key]), true)
		for _, row := range b.rows[key] {
			columns := make([]string, 0, len(row))
			for column := range row {
				columns = append(columns, column)
			}
			slices.Sort(columns)
			cycleFrame(h, []byte(cycleRowDigest(columns, row)), true)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
