package evaluation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"reflect"
	"strconv"
	"time"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

// These errors never contain a connection, message body, or business reason.
var (
	ErrSQLHistoricalFactsInvalid       = errors.New("sql_historical_facts_invalid")
	ErrSQLHistoricalFactsConflict      = errors.New("sql_historical_facts_conflict")
	ErrSQLHistoricalFactsIdentity      = errors.New("sql_historical_database_identity_conflict")
	ErrSQLHistoricalFactsRead          = errors.New("sql_historical_facts_read_failed")
	ErrSQLHistoricalFactsBounds        = errors.New("sql_historical_facts_bound_exceeded")
	ErrSQLHistoricalFactsSerialization = errors.New("private_sql_historical_facts_serialization_forbidden")
	ErrSQLHistoricalOwnerAbsent        = errors.New("sql_historical_assessment_absent")
)

const SQLHistoricalOwnerRowLimit = 256
const sqlHistoricalOwnerByteLimit = 32 << 20
const SQLHistoricalClockComparisonRuleVersion = "sql-assessment-clock/v1"

type SQLHistoricalOwner struct {
	AssessmentID, OrgID, TesteeID, AnswerSheetID                                    uint64
	Status, QuestionnaireCode, QuestionnaireVersion                                 string
	ModelKind, ModelAlgorithm, ModelCode, ModelVersion                              string
	SubmittedAt, EvaluatedAt, FailedAt                                              *time.Time
	FailureReasonSHA256                                                             string
	OriginType                                                                      string
	OriginID                                                                        *string
	ConductingContextPresent                                                        bool
	ConductingContextBytes                                                          []byte
	ConductingContextSHA256                                                         string
	ActualSubmittedAtDataType, ActualEvaluatedAtDataType, ActualFailedAtDataType    string
	ActualSubmittedAtPrecision, ActualEvaluatedAtPrecision, ActualFailedAtPrecision int
	ClockComparisonRuleVersion                                                      string
}

type SQLHistoricalRun struct {
	ID, AssessmentID                                uint64
	ResourceID, Scope, Status, Origin               string
	Attempt                                         uint
	Retryable                                       bool
	FinishedAt, LeaseExpiresAt, NextAttemptAt       *time.Time
	RetryDisposition, RetryEventID, ActionRequestID string
}

type SQLHistoricalOutcome struct {
	ID, AssessmentID, OrgID, TesteeID uint64
	RunID                             string
	EvaluatedAt                       time.Time
	Invalid                           bool
}

type SQLHistoricalResponsibility struct {
	Store, ID, EventID, EventType, State string
	OrgID, AssessmentID, TesteeID        uint64
	LeasePresent, Unfinished, Invalid    bool
}

// This is a private local snapshot, not an authenticated historical source or
// a business-acceptance receipt. Global unbound rows and Mongo are outside it.
type SQLHistoricalFactsSnapshot struct {
	Owner            SQLHistoricalOwner
	Runs             []SQLHistoricalRun
	Outcomes         []SQLHistoricalOutcome
	Responsibilities []SQLHistoricalResponsibility
}

func (SQLHistoricalFactsSnapshot) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (SQLHistoricalFactsSnapshot) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (SQLHistoricalFactsSnapshot) String() string   { return "private SQL historical owner facts" }
func (SQLHistoricalFactsSnapshot) GoString() string { return "private SQL historical owner facts" }

// SQLHistoricalOwnerFacts is created only from an SDK-validated host-owned
// transaction. It retains exact rows for recheck but owns no pool or lifecycle.
type SQLHistoricalOwnerFacts struct {
	identity, eventID        string
	assessmentID             uint64
	rows                     map[string][]historicalSQLRow
	snapshot                 SQLHistoricalFactsSnapshot
	outcomeRecords           map[uint64]*evaluationfact.Record
	answerSheetAssociationID uint64
}

func sqlHistoricalIdentity(server, database string) string {
	h := sha256.New()
	for _, value := range []string{"mysql_database_identity_v1", server, database} {
		var frame [9]byte
		frame[0] = 1
		binary.BigEndian.PutUint64(frame[1:], uint64(len(value)))
		_, _ = h.Write(frame[:])
		_, _ = h.Write([]byte(value))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sqlHistoricalRows(tx *gorm.DB, table, predicate string, args ...any) (result []historicalSQLRow, err error) {
	return sqlHistoricalQueryRows(tx, "SELECT * FROM `"+table+"` WHERE "+predicate+" ORDER BY id", args...)
}

func sqlHistoricalQueryRows(tx *gorm.DB, query string, args ...any) (result []historicalSQLRow, err error) {
	return sqlHistoricalReadRows(tx, query+" LIMIT ? FOR UPDATE", append(args, SQLHistoricalOwnerRowLimit+1)...)
}

func sqlHistoricalReadRows(tx *gorm.DB, query string, args ...any) (result []historicalSQLRow, err error) {
	rows, err := tx.Raw(query, args...).Rows()
	if err != nil {
		return nil, ErrSQLHistoricalFactsRead
	}
	defer func() {
		if e := rows.Close(); err == nil && e != nil {
			err = ErrSQLHistoricalFactsRead
		}
	}()
	names, err := rows.Columns()
	if err != nil {
		return nil, ErrSQLHistoricalFactsRead
	}
	total := 0
	for rows.Next() {
		if len(result) >= SQLHistoricalOwnerRowLimit {
			return nil, ErrSQLHistoricalFactsBounds
		}
		values := make([]sql.RawBytes, len(names))
		arguments := make([]any, len(names))
		for i := range values {
			arguments[i] = &values[i]
		}
		if err := rows.Scan(arguments...); err != nil {
			return nil, ErrSQLHistoricalFactsRead
		}
		row := make(historicalSQLRow, len(names))
		for i, name := range names {
			if _, duplicate := row[name]; duplicate {
				return nil, ErrSQLHistoricalFactsInvalid
			}
			total += len(values[i])
			if total > sqlHistoricalOwnerByteLimit {
				return nil, ErrSQLHistoricalFactsBounds
			}
			if values[i] == nil {
				row[name] = nil
			} else {
				value := string(values[i])
				row[name] = &value
			}
		}
		result = append(result, row)
	}
	if rows.Err() != nil {
		return nil, ErrSQLHistoricalFactsRead
	}
	return result, nil
}

func sqlHistoricalUint(row historicalSQLRow, column string) (uint64, error) {
	s := valueOrEmpty(row[column])
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != s {
		return 0, ErrSQLHistoricalFactsInvalid
	}
	return n, nil
}

func sqlHistoricalTime(row historicalSQLRow, column string) (*time.Time, error) {
	s, exists := row[column]
	if !exists {
		return nil, ErrSQLHistoricalFactsInvalid
	}
	if s == nil {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999"} {
		if at, err := time.ParseInLocation(layout, *s, time.UTC); err == nil {
			at = at.UTC()
			return &at, nil
		}
	}
	return nil, ErrSQLHistoricalFactsInvalid
}

// PrepareSQLHistoricalOwnerFacts includes wrongly scoped candidate messages
// by actual payload owner as well as outer hints. Hints only select candidates;
// SDK/typed validation authenticates each selected current message separately.
// Unbound global rows require the coordinator's independent reverse coverage.
func PrepareSQLHistoricalOwnerFacts(ctx context.Context, assessmentID uint64, eventID, expectedIdentityHash string) (*SQLHistoricalOwnerFacts, error) {
	if assessmentID == 0 || eventID == "" || len(eventID) > 64 || !evidence.ValidSHA256(expectedIdentityHash) {
		return nil, ErrSQLHistoricalFactsInvalid
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return nil, ErrSQLHistoricalFactsRead
	}
	if sqlHistoricalIdentity(server, database) != expectedIdentityHash {
		return nil, ErrSQLHistoricalFactsIdentity
	}
	owner, err := historicalReadRow(tx, "assessment", "id=?", assessmentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSQLHistoricalOwnerAbsent
		}
		return nil, ErrSQLHistoricalFactsRead
	}
	if owner["deleted_at"] != nil {
		return nil, ErrSQLHistoricalFactsInvalid
	}
	facts := &SQLHistoricalOwnerFacts{identity: expectedIdentityHash, eventID: eventID, assessmentID: assessmentID, rows: map[string][]historicalSQLRow{"assessment": {owner}}, outcomeRecords: make(map[uint64]*evaluationfact.Record)}
	if err := facts.readOwners(tx); err != nil {
		return nil, err
	}
	if err := facts.readResponsibilities(tx); err != nil {
		return nil, err
	}
	return facts, nil
}

// PrepareSQLHistoricalAnswerSheetFacts reads the original global unique
// association before organization checks. Absence proves only local absence,
// never an independent-questionnaire Admission or business closure.
func PrepareSQLHistoricalAnswerSheetFacts(ctx context.Context, answerSheetID uint64, eventID, expectedIdentityHash string) (*SQLHistoricalOwnerFacts, error) {
	if answerSheetID == 0 || !evidence.ValidSHA256(expectedIdentityHash) {
		return nil, ErrSQLHistoricalFactsInvalid
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return nil, ErrSQLHistoricalFactsRead
	}
	if sqlHistoricalIdentity(server, database) != expectedIdentityHash {
		return nil, ErrSQLHistoricalFactsIdentity
	}
	var indexCount int
	if err := tx.Raw("SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='assessment' AND index_name='uk_answer_sheet_id' AND non_unique=0 AND seq_in_index=1 AND column_name='answer_sheet_id' AND sub_part IS NULL AND (SELECT COUNT(*) FROM information_schema.statistics s WHERE s.table_schema=DATABASE() AND s.table_name='assessment' AND s.index_name='uk_answer_sheet_id')=1").Scan(&indexCount).Error; err != nil {
		return nil, ErrSQLHistoricalFactsRead
	}
	if indexCount != 1 {
		return nil, ErrSQLHistoricalFactsConflict
	}
	owner, err := historicalReadRow(tx, "assessment", "answer_sheet_id=?", answerSheetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSQLHistoricalOwnerAbsent
		}
		return nil, ErrSQLHistoricalFactsConflict
	}
	id, err := sqlHistoricalUint(owner, "id")
	if err != nil {
		return nil, err
	}
	facts, err := PrepareSQLHistoricalOwnerFacts(ctx, id, eventID, expectedIdentityHash)
	if err != nil {
		return nil, err
	}
	if facts.snapshot.Owner.AnswerSheetID != answerSheetID {
		return nil, ErrSQLHistoricalFactsConflict
	}
	facts.answerSheetAssociationID = answerSheetID
	return facts, nil
}

// HasVerifiedAnswerSheetAssociation distinguishes the actual unique-index /
// global association entrypoint from an editable snapshot or an ID point read.
func (f *SQLHistoricalOwnerFacts) HasVerifiedAnswerSheetAssociation(answerSheetID uint64) bool {
	return f != nil && answerSheetID != 0 && f.answerSheetAssociationID == answerSheetID && f.snapshot.Owner.AnswerSheetID == answerSheetID && f.snapshot.Owner.AssessmentID == f.assessmentID
}

func (f *SQLHistoricalOwnerFacts) readOwners(tx *gorm.DB) error {
	owner := f.rows["assessment"][0]
	var err error
	o := &f.snapshot.Owner
	o.AssessmentID = f.assessmentID
	o.ClockComparisonRuleVersion = SQLHistoricalClockComparisonRuleVersion
	f.rows["assessment_clock_columns"], err = sqlHistoricalReadRows(tx, "SELECT COLUMN_NAME AS column_name,DATA_TYPE AS data_type,DATETIME_PRECISION AS datetime_precision,COLUMN_TYPE AS column_type,IS_NULLABLE AS is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='assessment' AND column_name IN ('submitted_at','evaluated_at','failed_at') ORDER BY COLUMN_NAME LIMIT 4")
	if err != nil || len(f.rows["assessment_clock_columns"]) != 3 {
		return ErrSQLHistoricalFactsInvalid
	}
	for _, column := range f.rows["assessment_clock_columns"] {
		dataType, textPrecision := valueOrEmpty(column["data_type"]), valueOrEmpty(column["datetime_precision"])
		precision, err := strconv.Atoi(textPrecision)
		if err != nil || dataType != "datetime" || strconv.Itoa(precision) != textPrecision || (precision != 0 && precision != 3 && precision != 6) {
			return ErrSQLHistoricalFactsInvalid
		}
		switch valueOrEmpty(column["column_name"]) {
		case "submitted_at":
			o.ActualSubmittedAtDataType, o.ActualSubmittedAtPrecision = dataType, precision
		case "evaluated_at":
			o.ActualEvaluatedAtDataType, o.ActualEvaluatedAtPrecision = dataType, precision
		case "failed_at":
			o.ActualFailedAtDataType, o.ActualFailedAtPrecision = dataType, precision
		default:
			return ErrSQLHistoricalFactsInvalid
		}
	}
	if o.OrgID, err = sqlHistoricalUint(owner, "org_id"); err != nil {
		return err
	}
	if o.TesteeID, err = sqlHistoricalUint(owner, "testee_id"); err != nil {
		return err
	}
	if o.AnswerSheetID, err = sqlHistoricalUint(owner, "answer_sheet_id"); err != nil {
		return err
	}
	o.Status, o.QuestionnaireCode, o.QuestionnaireVersion = valueOrEmpty(owner["status"]), valueOrEmpty(owner["questionnaire_code"]), valueOrEmpty(owner["questionnaire_version"])
	o.OriginType = valueOrEmpty(owner["origin_type"])
	if value := owner["origin_id"]; value != nil {
		copy := *value
		o.OriginID = &copy
	}
	context, exists := owner["conducting_context"]
	if !exists {
		return ErrSQLHistoricalFactsInvalid
	}
	o.ConductingContextPresent = context != nil
	if context != nil {
		o.ConductingContextBytes = []byte(*context)
		o.ConductingContextSHA256 = evidence.SourceDigest("assessment-conducting-context/v1", o.ConductingContextBytes).SHA256
	}
	o.ModelKind, o.ModelAlgorithm, o.ModelCode, o.ModelVersion = valueOrEmpty(owner["evaluation_model_kind"]), valueOrEmpty(owner["evaluation_model_algorithm"]), valueOrEmpty(owner["evaluation_model_code"]), valueOrEmpty(owner["evaluation_model_version"])
	if o.SubmittedAt, err = sqlHistoricalTime(owner, "submitted_at"); err != nil {
		return err
	}
	if o.EvaluatedAt, err = sqlHistoricalTime(owner, "evaluated_at"); err != nil {
		return err
	}
	if o.FailedAt, err = sqlHistoricalTime(owner, "failed_at"); err != nil {
		return err
	}
	if reason := owner["failure_reason"]; reason != nil {
		o.FailureReasonSHA256 = evidence.SourceDigest("sql-owner-failure-reason/v1", []byte(*reason)).SHA256
	}
	f.rows["runtime_checkpoint"], err = sqlHistoricalRows(tx, "runtime_checkpoint", "assessment_id=?", f.assessmentID)
	if err != nil {
		return err
	}
	for _, row := range f.rows["runtime_checkpoint"] {
		r := SQLHistoricalRun{ResourceID: valueOrEmpty(row["resource_id"]), Scope: valueOrEmpty(row["scope"]), Status: valueOrEmpty(row["status"]), Origin: valueOrEmpty(row["attempt_origin"]), RetryDisposition: valueOrEmpty(row["retry_disposition"]), RetryEventID: valueOrEmpty(row["retry_event_id"]), ActionRequestID: valueOrEmpty(row["action_request_id"])}
		if r.ID, err = sqlHistoricalUint(row, "id"); err != nil {
			return err
		}
		if r.AssessmentID, err = sqlHistoricalUint(row, "assessment_id"); err != nil {
			return err
		}
		attempt, e := sqlHistoricalUint(row, "attempt_no")
		if e != nil || attempt > 1<<32-1 {
			return ErrSQLHistoricalFactsInvalid
		}
		r.Attempt = uint(attempt)
		if valueOrEmpty(row["retryable"]) != "0" && valueOrEmpty(row["retryable"]) != "1" {
			return ErrSQLHistoricalFactsInvalid
		}
		r.Retryable = valueOrEmpty(row["retryable"]) == "1"
		if r.FinishedAt, err = sqlHistoricalTime(row, "finished_at"); err != nil {
			return err
		}
		if r.LeaseExpiresAt, err = sqlHistoricalTime(row, "lease_expires_at"); err != nil {
			return err
		}
		if r.NextAttemptAt, err = sqlHistoricalTime(row, "next_attempt_at"); err != nil {
			return err
		}
		if row["deleted_at"] != nil {
			r.Status = "deleted"
		}
		f.snapshot.Runs = append(f.snapshot.Runs, r)
	}
	f.rows["evaluation_outcome"], err = sqlHistoricalRows(tx, "evaluation_outcome", "assessment_id=?", f.assessmentID)
	if err != nil {
		return err
	}
	for _, row := range f.rows["evaluation_outcome"] {
		v := SQLHistoricalOutcome{RunID: valueOrEmpty(row["evaluation_run_id"])}
		if v.ID, err = sqlHistoricalUint(row, "id"); err != nil {
			return err
		}
		if v.AssessmentID, err = sqlHistoricalUint(row, "assessment_id"); err != nil {
			return err
		}
		if v.OrgID, err = sqlHistoricalUint(row, "org_id"); err != nil {
			return err
		}
		if v.TesteeID, err = sqlHistoricalUint(row, "testee_id"); err != nil {
			return err
		}
		at, e := sqlHistoricalTime(row, "evaluated_at")
		if e != nil || at == nil {
			return ErrSQLHistoricalFactsInvalid
		}
		v.EvaluatedAt = *at
		var po EvaluationOutcomePO
		if err := tx.Raw("SELECT * FROM evaluation_outcome WHERE id=? FOR UPDATE", v.ID).Scan(&po).Error; err != nil {
			return ErrSQLHistoricalFactsRead
		}
		record, e := outcomeFromPO(&po)
		v.Invalid = e != nil
		if e == nil {
			f.outcomeRecords[v.ID] = sqlHistoricalFactRecord(record)
		}
		f.snapshot.Outcomes = append(f.snapshot.Outcomes, v)
	}
	return nil
}

func sqlHistoricalFactRecord(record *domainoutcome.Record) *evaluationfact.Record {
	model, runtime := record.Model(), record.Runtime()
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: record.ID(), OrgID: record.OrgID(), AssessmentID: record.AssessmentID(), TesteeID: record.TesteeID(), RunID: record.RunID(),
		Model:            evaluationfact.ModelIdentity{Kind: model.Kind, Algorithm: model.Algorithm, Code: model.Code, Version: model.Version, Title: model.Title},
		Runtime:          evaluationfact.RuntimeIdentity{DecisionKind: runtime.DecisionKind},
		InputSnapshotRef: record.InputSnapshotRef(), SchemaVersion: record.SchemaVersion(), Payload: record.Payload(), ReportInput: record.ReportInput(), EvaluatedAt: record.EvaluatedAt(),
	})
}

// OutcomeRecord exposes one exact, validated original Outcome as an immutable
// copy. It never selects a current winner and cannot authenticate a source DTO
// or replace the coordinator's full-chain closure and snapshot recheck.
func (f *SQLHistoricalOwnerFacts) OutcomeRecord(outcomeID uint64) (*evaluationfact.Record, error) {
	if f == nil || outcomeID == 0 {
		return nil, ErrSQLHistoricalFactsInvalid
	}
	record := f.outcomeRecords[outcomeID]
	if record == nil {
		return nil, evaluationfact.ErrNotFound
	}
	if record.ID().Uint64() != outcomeID || record.AssessmentID().Uint64() != f.assessmentID || record.OrgID() <= 0 || uint64(record.OrgID()) != f.snapshot.Owner.OrgID || record.TesteeID() != f.snapshot.Owner.TesteeID {
		return nil, ErrSQLHistoricalFactsConflict
	}
	return evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: record.ID(), OrgID: record.OrgID(), AssessmentID: record.AssessmentID(), TesteeID: record.TesteeID(), RunID: record.RunID(), Model: record.Model(), Runtime: record.Runtime(),
		InputSnapshotRef: record.InputSnapshotRef(), SchemaVersion: record.SchemaVersion(), Payload: record.Payload(), ReportInput: record.ReportInput(), EvaluatedAt: record.EvaluatedAt(),
	}), nil
}

func (f *SQLHistoricalOwnerFacts) Snapshot() SQLHistoricalFactsSnapshot {
	if f == nil {
		return SQLHistoricalFactsSnapshot{}
	}
	v := f.snapshot
	copyTime := func(t *time.Time) *time.Time {
		if t == nil {
			return nil
		}
		v := *t
		return &v
	}
	v.Owner.SubmittedAt, v.Owner.EvaluatedAt, v.Owner.FailedAt = copyTime(v.Owner.SubmittedAt), copyTime(v.Owner.EvaluatedAt), copyTime(v.Owner.FailedAt)
	if v.Owner.OriginID != nil {
		copy := *v.Owner.OriginID
		v.Owner.OriginID = &copy
	}
	v.Owner.ConductingContextBytes = append([]byte(nil), v.Owner.ConductingContextBytes...)
	v.Runs = append([]SQLHistoricalRun(nil), v.Runs...)
	for i := range v.Runs {
		v.Runs[i].FinishedAt = copyTime(v.Runs[i].FinishedAt)
		v.Runs[i].LeaseExpiresAt = copyTime(v.Runs[i].LeaseExpiresAt)
		v.Runs[i].NextAttemptAt = copyTime(v.Runs[i].NextAttemptAt)
	}
	v.Outcomes = append([]SQLHistoricalOutcome(nil), v.Outcomes...)
	v.Responsibilities = append([]SQLHistoricalResponsibility(nil), v.Responsibilities...)
	return v
}

// Recheck reads actual locked rows again. It performs no CAS/write and cannot
// prove a future writer is stopped; that remains the maintenance coordinator's
// responsibility. Unknown/new columns and SQL NULL differences also conflict.
func (f *SQLHistoricalOwnerFacts) Recheck(ctx context.Context) error {
	if f == nil {
		return ErrSQLHistoricalFactsInvalid
	}
	if f.snapshot.Owner.ClockComparisonRuleVersion != SQLHistoricalClockComparisonRuleVersion {
		return ErrSQLHistoricalFactsConflict
	}
	var current *SQLHistoricalOwnerFacts
	var err error
	if f.answerSheetAssociationID != 0 {
		current, err = PrepareSQLHistoricalAnswerSheetFacts(ctx, f.answerSheetAssociationID, f.eventID, f.identity)
	} else {
		current, err = PrepareSQLHistoricalOwnerFacts(ctx, f.assessmentID, f.eventID, f.identity)
	}
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current.rows, f.rows) {
		return ErrSQLHistoricalFactsConflict
	}
	return nil
}
