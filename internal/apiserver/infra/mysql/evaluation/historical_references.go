package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	evidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"gorm.io/gorm"
)

type historicalSQLRow map[string]*string

type historicalSQLBaseline struct {
	table, column, server, database string
	id                              uint64
	row                             historicalSQLRow
	runs                            map[string]historicalSQLRow
}

// The baseline is opaque and can only originate from a real borrowed SQL
// transaction. Every column, including unknown columns, is frozen. Neither
// helper establishes source authenticity or permission to retire a message.
type AssessmentHistoricalBaseline struct {
	baseline *historicalSQLBaseline
	record   AssessmentPO
}

type OutcomeHistoricalBaseline struct {
	baseline *historicalSQLBaseline
	record   EvaluationOutcomePO
}

func historicalTx(ctx context.Context) (*gorm.DB, error) {
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx == nil || tx.Statement == nil || historicalNil(tx.Statement.ConnPool) {
		return nil, hostmysql.ErrActiveTransactionRequired
	}
	if prepared, ok := tx.Statement.ConnPool.(*gorm.PreparedStmtTX); ok && (prepared == nil || historicalNil(prepared.Tx)) {
		return nil, hostmysql.ErrActiveTransactionRequired
	}
	if _, err := sdkmysql.BindGORM(tx); err != nil {
		return nil, err
	}
	return tx.WithContext(ctx), nil
}

func historicalNil(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	default:
		return false
	}
}

func historicalDatabase(tx *gorm.DB) (string, string, error) {
	var identity struct {
		Server   string
		Database string
	}
	if err := tx.Raw("SELECT @@server_uuid AS server,DATABASE() AS `database`").Scan(&identity).Error; err != nil {
		return "", "", err
	}
	if identity.Server == "" || identity.Database == "" {
		return "", "", evidence.ErrHistoricalReferenceInvalid
	}
	return identity.Server, identity.Database, nil
}

func historicalReadRow(tx *gorm.DB, table, predicate string, args ...any) (result historicalSQLRow, err error) {
	rows, err := tx.Raw("SELECT * FROM `"+table+"` WHERE "+predicate+" LIMIT 2 FOR UPDATE", args...).Rows()
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, gorm.ErrRecordNotFound
	}
	values := make([]sql.RawBytes, len(names))
	arguments := make([]any, len(names))
	for i := range arguments {
		arguments[i] = &values[i]
	}
	if err := rows.Scan(arguments...); err != nil {
		return nil, err
	}
	result = make(historicalSQLRow, len(names))
	for i, name := range names {
		if _, duplicate := result[name]; duplicate {
			return nil, evidence.ErrHistoricalReferenceInvalid
		}
		if values[i] == nil {
			result[name] = nil
		} else {
			value := string(values[i])
			result[name] = &value
		}
	}
	if rows.Next() {
		return nil, evidence.ErrHistoricalReferenceConflict
	}
	return result, rows.Err()
}

func historicalRunKey(run evidence.HistoricalRunReferenceV1) string {
	return run.RunID + ":" + strconv.FormatUint(uint64(run.Attempt), 10)
}

func prepareHistorical(ctx context.Context, table, column string, id, assessmentID uint64, runs []evidence.HistoricalRunReferenceV1) (*historicalSQLBaseline, error) {
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	if id == 0 || assessmentID == 0 || len(runs) > evidence.HistoricalReferenceMaxEntries {
		return nil, evidence.ErrHistoricalReferenceInvalid
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return nil, err
	}
	row, err := historicalReadRow(tx, table, "id=?", id)
	if err != nil {
		return nil, err
	}
	if _, exists := row[column]; !exists {
		return nil, evidence.ErrHistoricalReferenceInvalid
	}
	if _, err := historicalDecode(row[column]); err != nil {
		return nil, err
	}
	baseline := &historicalSQLBaseline{table: table, column: column, server: server, database: database, id: id, row: row, runs: map[string]historicalSQLRow{}}
	for _, run := range runs {
		if run.RunID == "" || len(run.RunID) > 128 || run.Attempt == 0 {
			return nil, evidence.ErrHistoricalReferenceInvalid
		}
		key := historicalRunKey(run)
		if _, exists := baseline.runs[key]; exists {
			return nil, evidence.ErrHistoricalReferenceConflict
		}
		runRow, err := historicalReadRow(tx, "runtime_checkpoint", "scope='evaluation_run' AND assessment_id=? AND attempt_no=? AND deleted_at IS NULL", assessmentID, run.Attempt)
		if err != nil {
			return nil, err
		}
		if valueOrEmpty(runRow["scope"]) != "evaluation_run" || valueOrEmpty(runRow["resource_id"]) != run.RunID || (valueOrEmpty(runRow["status"]) != "failed" && valueOrEmpty(runRow["status"]) != "succeeded") || runRow["finished_at"] == nil || runRow["lease_expires_at"] != nil {
			return nil, evidence.ErrHistoricalReferenceConflict
		}
		baseline.runs[key] = runRow
	}
	return baseline, nil
}

// PrepareAssessmentHistoricalReferences locks one terminal assessment and any
// explicitly identified original attempts in the caller-owned transaction.
func PrepareAssessmentHistoricalReferences(ctx context.Context, id uint64, runs ...evidence.HistoricalRunReferenceV1) (*AssessmentHistoricalBaseline, error) {
	b, err := prepareHistorical(ctx, "assessment", "historical_lifecycle_evidence", id, id, runs)
	if err != nil {
		return nil, err
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	var po AssessmentPO
	if err := tx.First(&po, id).Error; err != nil {
		return nil, err
	}
	if po.ID.Uint64() != id || po.OrgID <= 0 || po.TesteeID == 0 || po.AnswerSheetID == 0 || po.DeletedAt != nil || po.QuestionnaireCode == "" || po.QuestionnaireVersion == "" || (po.Status != "evaluated" && po.Status != "failed") {
		return nil, evidence.ErrHistoricalReferenceInvalid
	}
	return &AssessmentHistoricalBaseline{baseline: b, record: po}, nil
}

func PrepareOutcomeHistoricalReferences(ctx context.Context, id uint64, runs ...evidence.HistoricalRunReferenceV1) (*OutcomeHistoricalBaseline, error) {
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	// Obtain the immutable owner before preparing the full locked snapshot.
	var po EvaluationOutcomePO
	if err := tx.Raw("SELECT * FROM evaluation_outcome WHERE id=? FOR UPDATE", id).Scan(&po).Error; err != nil {
		return nil, err
	}
	if po.ID != id || id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if _, err := outcomeFromPO(&po); err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.RunID != po.EvaluationRunID {
			return nil, evidence.ErrHistoricalReferenceConflict
		}
	}
	b, err := prepareHistorical(ctx, "evaluation_outcome", "historical_committed_evidence", id, po.AssessmentID, runs)
	if err != nil {
		return nil, err
	}
	return &OutcomeHistoricalBaseline{baseline: b, record: po}, nil
}

func historicalCloneRecord[T any](value T) (T, error) {
	var copy T
	raw, err := json.Marshal(value)
	if err != nil {
		return copy, err
	}
	err = json.Unmarshal(raw, &copy)
	return copy, err
}

func (b *AssessmentHistoricalBaseline) Record() (AssessmentPO, error) {
	if b == nil || b.baseline == nil {
		return AssessmentPO{}, evidence.ErrHistoricalReferenceInvalid
	}
	return historicalCloneRecord(b.record)
}

func (b *OutcomeHistoricalBaseline) Record() (EvaluationOutcomePO, error) {
	if b == nil || b.baseline == nil {
		return EvaluationOutcomePO{}, evidence.ErrHistoricalReferenceInvalid
	}
	return historicalCloneRecord(b.record)
}

func historicalAllowed(table, eventType string) bool {
	if table == "evaluation_outcome" {
		return eventType == "evaluation.outcome.committed"
	}
	return table == "assessment" && (eventType == "evaluation.requested" || eventType == "evaluation.retry.requested" || eventType == "evaluation.failed")
}

func historicalBinding(b *historicalSQLBaseline, eventType string, run *evidence.HistoricalRunReferenceV1) (string, error) {
	if b == nil || !historicalAllowed(b.table, eventType) {
		return "", evidence.ErrHistoricalReferenceInvalid
	}
	var originalRun historicalSQLRow
	if run != nil {
		row, exists := b.runs[historicalRunKey(*run)]
		if !exists {
			return "", evidence.ErrHistoricalReferenceInvalid
		}
		originalRun = row
	}
	return historicalStableBinding(b.server, b.database, b.table, eventType, b.row, originalRun)
}

func (b *AssessmentHistoricalBaseline) BindingSHA256(eventType string, run *evidence.HistoricalRunReferenceV1) (string, error) {
	if b == nil {
		return "", evidence.ErrHistoricalReferenceInvalid
	}
	return historicalBinding(b.baseline, eventType, run)
}

func (b *OutcomeHistoricalBaseline) BindingSHA256(eventType string, run *evidence.HistoricalRunReferenceV1) (string, error) {
	if b == nil {
		return "", evidence.ErrHistoricalReferenceInvalid
	}
	return historicalBinding(b.baseline, eventType, run)
}

func historicalDecode(raw *string) (*evidence.HistoricalReferenceSetV1, error) {
	if raw == nil {
		return nil, nil
	}
	return evidence.DecodeHistoricalReferenceSetJSON([]byte(*raw))
}

func historicalSameFacts(left, right historicalSQLRow, excluded string) bool {
	if len(left) != len(right) {
		return false
	}
	for name, value := range left {
		if name != excluded {
			other, exists := right[name]
			if !exists || !reflect.DeepEqual(value, other) {
				return false
			}
		}
	}
	return true
}

func appendHistorical(ctx context.Context, b *historicalSQLBaseline, entries []evidence.HistoricalReferenceEntryV1) error {
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	if b == nil || b.id == 0 || len(entries) == 0 || len(entries) > evidence.HistoricalReferenceMaxEntries || !historicalAllowed(b.table, entries[0].EventType) {
		return evidence.ErrHistoricalReferenceInvalid
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return err
	}
	if server != b.server || database != b.database {
		return evidence.ErrHistoricalReferenceConflict
	}
	current, err := historicalReadRow(tx, b.table, "id=?", b.id)
	if err != nil {
		return err
	}
	if !historicalSameFacts(current, b.row, b.column) {
		return evidence.ErrHistoricalReferenceConflict
	}
	for _, row := range b.runs {
		run, err := historicalReadRow(tx, "runtime_checkpoint", "id=?", valueOrEmpty(row["id"]))
		if err != nil {
			return err
		}
		if !historicalSameFacts(run, row, "") {
			return evidence.ErrHistoricalReferenceConflict
		}
	}
	previous, err := historicalDecode(b.row[b.column])
	if err != nil {
		return err
	}
	stored, err := historicalDecode(current[b.column])
	if err != nil {
		return err
	}
	// A concurrent append is allowed; removal or alteration of baseline entries
	// is not. Normal business fields have already passed an exact comparison.
	if previous != nil {
		if stored == nil {
			return evidence.ErrHistoricalReferenceConflict
		}
		combined, err := stored.Append(previous.Entries...)
		if err != nil || !reflect.DeepEqual(combined, stored) {
			return evidence.ErrHistoricalReferenceConflict
		}
	}
	if stored != nil {
		for _, entry := range stored.Entries {
			if !historicalAllowed(b.table, entry.EventType) {
				return evidence.ErrHistoricalReferenceConflict
			}
		}
	}
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		binding, err := historicalBinding(b, entry.EventType, entry.Run)
		if err != nil {
			return err
		}
		if entry.Proof.BusinessBindingSHA256 != binding {
			return evidence.ErrHistoricalReferenceConflict
		}
	}
	next, err := stored.Append(entries...)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(stored, next) {
		return nil
	}
	body, err := json.Marshal(next)
	if err != nil {
		return err
	}
	query := "UPDATE `" + b.table + "` SET `" + b.column + "`=?"
	// Explicitly preserve MySQL ON UPDATE clocks; a maintenance attachment is
	// not a new business transition and must not change its frozen baseline.
	if _, exists := current["updated_at"]; exists {
		query += ",updated_at=updated_at"
	}
	query += " WHERE id=? AND "
	args := []any{string(body), b.id}
	if current[b.column] == nil {
		query += "`" + b.column + "` IS NULL"
	} else {
		query += "CAST(`" + b.column + "` AS BINARY)=?"
		args = append(args, []byte(*current[b.column]))
	}
	result := tx.Exec(query, args...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: SQL CAS", evidence.ErrHistoricalReferenceConflict)
	}
	return nil
}

func AppendAssessmentHistoricalReferences(ctx context.Context, baseline *AssessmentHistoricalBaseline, entries ...evidence.HistoricalReferenceEntryV1) error {
	if baseline == nil {
		return evidence.ErrHistoricalReferenceInvalid
	}
	return appendHistorical(ctx, baseline.baseline, entries)
}

func AppendOutcomeHistoricalReferences(ctx context.Context, baseline *OutcomeHistoricalBaseline, entries ...evidence.HistoricalReferenceEntryV1) error {
	if baseline == nil {
		return evidence.ErrHistoricalReferenceInvalid
	}
	return appendHistorical(ctx, baseline.baseline, entries)
}
