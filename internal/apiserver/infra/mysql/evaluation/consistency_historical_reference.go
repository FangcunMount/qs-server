package evaluation

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

// The persisted anchor is reconstructable after an ordinary aggregate Save.
// Full-row, NULL and future-column freezing remains a separate CAS invariant.
// It authenticates an owner binding, never original message bytes or delivery.
func historicalStableBinding(server, database, table, eventType string, row, run historicalSQLRow) (string, error) {
	if server == "" || database == "" || !historicalAllowed(table, eventType) {
		return "", evidence.ErrHistoricalReferenceInvalid
	}
	for _, name := range []string{"id", "org_id", "testee_id"} {
		n, err := strconv.ParseUint(valueOrEmpty(row[name]), 10, 64)
		if err != nil || n == 0 || strconv.FormatUint(n, 10) != valueOrEmpty(row[name]) {
			return "", evidence.ErrHistoricalReferenceInvalid
		}
	}
	fields := []*string{evidence.String(server), evidence.String(database), evidence.String(table), evidence.String(eventType)}
	appendFields := func(names []string) error {
		for _, name := range names {
			value, exists := row[name]
			if !exists {
				return evidence.ErrHistoricalReferenceInvalid
			}
			fields = append(fields, evidence.String(name), value)
		}
		return nil
	}
	if table == "assessment" {
		if valueOrEmpty(row["answer_sheet_id"]) == "" || valueOrEmpty(row["answer_sheet_id"]) == "0" || valueOrEmpty(row["questionnaire_code"]) == "" || valueOrEmpty(row["questionnaire_version"]) == "" {
			return "", evidence.ErrHistoricalReferenceInvalid
		}
		if err := appendFields([]string{"id", "org_id", "testee_id", "answer_sheet_id", "questionnaire_code", "questionnaire_version", "origin_type", "origin_id"}); err != nil {
			return "", err
		}
		// These optional identities use the domain mapper's empty semantics;
		// nullable raw facts are still compared exactly by the write CAS.
		for _, name := range []string{"evaluation_model_kind", "evaluation_model_code", "evaluation_model_version"} {
			if _, exists := row[name]; !exists {
				return "", evidence.ErrHistoricalReferenceInvalid
			}
			fields = append(fields, evidence.String(name), evidence.String(valueOrEmpty(row[name])))
		}
		if context, exists := row["conducting_context"]; !exists {
			return "", evidence.ErrHistoricalReferenceInvalid
		} else if context == nil {
			fields = append(fields, nil)
		} else {
			fields = append(fields, evidence.String(evidence.SourceDigest("assessment-conducting-context/v1", []byte(*context)).SHA256))
		}
	} else {
		if valueOrEmpty(row["assessment_id"]) == "" || valueOrEmpty(row["assessment_id"]) == "0" || valueOrEmpty(row["evaluation_run_id"]) == "" || valueOrEmpty(row["model_kind"]) == "" || valueOrEmpty(row["model_code"]) == "" {
			return "", evidence.ErrHistoricalReferenceInvalid
		}
		if err := appendFields([]string{"id", "org_id", "assessment_id", "testee_id", "evaluation_run_id", "model_kind", "model_sub_kind", "model_algorithm", "model_code", "model_version", "model_title", "decision_kind", "input_snapshot_ref", "schema_version"}); err != nil {
			return "", err
		}
		for _, name := range []string{"payload_json", "report_input_json"} {
			value, exists := row[name]
			if !exists {
				return "", evidence.ErrHistoricalReferenceInvalid
			}
			if value == nil {
				fields = append(fields, nil)
			} else {
				fields = append(fields, evidence.String(evidence.SourceDigest("outcome-immutable-"+name+"/v1", []byte(*value)).SHA256))
			}
		}
		at, err := historicalMillisecond(valueOrEmpty(row["evaluated_at"]))
		if err != nil {
			return "", err
		}
		fields = append(fields, evidence.String(at))
	}
	if run == nil {
		fields = append(fields, nil)
	} else {
		owner := valueOrEmpty(row["id"])
		if table == "evaluation_outcome" {
			owner = valueOrEmpty(row["assessment_id"])
			if valueOrEmpty(run["resource_id"]) != valueOrEmpty(row["evaluation_run_id"]) {
				return "", evidence.ErrHistoricalReferenceConflict
			}
		}
		if valueOrEmpty(run["scope"]) != "evaluation_run" || valueOrEmpty(run["assessment_id"]) != owner {
			return "", evidence.ErrHistoricalReferenceConflict
		}
		for _, name := range []string{"scope", "resource_id", "assessment_id", "attempt_no"} {
			if valueOrEmpty(run[name]) == "" {
				return "", evidence.ErrHistoricalReferenceInvalid
			}
			fields = append(fields, evidence.String(name), run[name])
		}
	}
	return evidence.BindingDigest("sql-historical-business-anchor/v1", fields...), nil
}

func historicalMillisecond(raw string) (string, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999"} {
		at, err := time.ParseInLocation(layout, raw, time.UTC)
		if err == nil && at.Nanosecond()%1_000_000 == 0 {
			return evidence.MillisecondTime(at), nil
		}
	}
	return "", evidence.ErrHistoricalReferenceInvalid
}

func historicalAuditRows(db *gorm.DB, table, predicate string, limit int, args ...any) (result []historicalSQLRow, err error) {
	rows, err := db.Table(table).Where(predicate, args...).Limit(limit + 1).Rows()
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
	for rows.Next() {
		if len(result) >= limit {
			return nil, evidence.ErrHistoricalReferenceConflict
		}
		values := make([]sql.RawBytes, len(names))
		args := make([]any, len(names))
		for i := range args {
			args[i] = &values[i]
		}
		if err := rows.Scan(args...); err != nil {
			return nil, err
		}
		row := make(historicalSQLRow, len(names))
		for i, name := range names {
			if _, exists := row[name]; exists {
				return nil, evidence.ErrHistoricalReferenceInvalid
			}
			if values[i] == nil {
				row[name] = nil
			} else {
				copy := string(values[i])
				row[name] = &copy
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// This is business-record-local, bounded provenance inspection. It does not
// establish full legacy source coverage or cross-store global uniqueness.
func (r *consistencyReadModel) listHistoricalReferences(ctx context.Context, ids []uint64) (map[uint64][]evaluationconsistency.HistoricalReferenceEvidence, error) {
	db := dbWithTransactionContext(ctx, r.db)
	server, database, err := historicalDatabase(db)
	if err != nil {
		return nil, err
	}
	assessments, err := historicalAuditRows(db, "assessment", "id IN ?", len(ids), ids)
	if err != nil {
		return nil, err
	}
	byAssessment := map[string]historicalSQLRow{}
	for _, row := range assessments {
		id := valueOrEmpty(row["id"])
		if _, exists := byAssessment[id]; exists {
			return nil, evidence.ErrHistoricalReferenceConflict
		}
		byAssessment[id] = row
	}
	outcomes, err := historicalAuditRows(db, "evaluation_outcome", "assessment_id IN ?", len(ids), ids)
	if err != nil {
		return nil, err
	}
	result := map[uint64][]evaluationconsistency.HistoricalReferenceEvidence{}
	type seenReference struct {
		assessment uint64
		index      int
	}
	seenID, seenSource := map[string]seenReference{}, map[string]seenReference{}
	inspect := func(table, slot string, row historicalSQLRow) error {
		owner := valueOrEmpty(row["id"])
		assessment := owner
		if table == "evaluation_outcome" {
			assessment = valueOrEmpty(row["assessment_id"])
		}
		id, err := strconv.ParseUint(assessment, 10, 64)
		if err != nil || id == 0 {
			return evidence.ErrHistoricalReferenceInvalid
		}
		raw, exists := row[slot]
		if !exists {
			return fmt.Errorf("%w: historical schema readiness", evidence.ErrHistoricalReferenceInvalid)
		}
		if raw == nil {
			return nil
		}
		set, err := historicalDecode(raw)
		if err != nil {
			result[id] = append(result[id], evaluationconsistency.HistoricalReferenceEvidence{Owner: table, OwnerID: owner, InvalidReason: "invalid historical reference set"})
			return nil
		}
		for _, entry := range set.Entries {
			out := evaluationconsistency.HistoricalReferenceEvidence{Owner: table, OwnerID: owner, EventID: entry.EventID, EventType: entry.EventType, Class: entry.Proof.Class}
			out.HistoricalReason = entry.Proof.Verification.Reason
			if entry.Run != nil {
				out.RunID, out.Attempt = entry.Run.RunID, entry.Run.Attempt
			}
			if table == "evaluation_outcome" {
				idColumn, hasID := row["committed_event_id"]
				proofColumn, hasProof := row["committed_event_evidence"]
				out.LegacyCanonicalAbsent = hasID && hasProof && idColumn == nil && proofColumn == nil
			}
			business := byAssessment[assessment]
			if business == nil || business["deleted_at"] != nil || valueOrEmpty(business["org_id"]) != valueOrEmpty(row["org_id"]) || valueOrEmpty(business["testee_id"]) != valueOrEmpty(row["testee_id"]) {
				out.InvalidReason = "historical owner graph conflicts"
			}
			var run historicalSQLRow
			if entry.Run != nil {
				runs, err := historicalAuditRows(db, "runtime_checkpoint", "CAST(scope AS BINARY)=? AND assessment_id=? AND attempt_no=? AND deleted_at IS NULL", 1, "evaluation_run", id, entry.Run.Attempt)
				if err != nil {
					return err
				}
				if len(runs) != 1 || valueOrEmpty(runs[0]["resource_id"]) != entry.Run.RunID || (valueOrEmpty(runs[0]["status"]) != "succeeded" && valueOrEmpty(runs[0]["status"]) != "failed") || runs[0]["finished_at"] == nil || runs[0]["lease_expires_at"] != nil {
					out.InvalidReason = "historical original run is missing or conflicts"
				} else {
					run = runs[0]
				}
			}
			binding, err := historicalStableBinding(server, database, table, entry.EventType, row, run)
			if err != nil || (entry.Run != nil && run == nil) || binding != entry.Proof.BusinessBindingSHA256 {
				out.InvalidReason = "historical business anchor conflicts"
			}
			index := len(result[id])
			result[id] = append(result[id], out)
			source := entry.Source.Database + "/" + entry.Source.Object + "/" + entry.Source.PrimaryKeyKind + "/" + entry.Source.PrimaryKeySHA256
			for _, reference := range []struct {
				key  string
				seen map[string]seenReference
			}{{entry.EventID, seenID}, {source, seenSource}} {
				if previous, exists := reference.seen[reference.key]; exists {
					result[id][index].InvalidReason = "historical original source reference is duplicated"
					result[previous.assessment][previous.index].InvalidReason = "historical original source reference is duplicated"
				} else {
					reference.seen[reference.key] = seenReference{assessment: id, index: index}
				}
			}
		}
		return nil
	}
	for _, row := range assessments {
		if err := inspect("assessment", "historical_lifecycle_evidence", row); err != nil {
			return nil, err
		}
	}
	for _, row := range outcomes {
		if err := inspect("evaluation_outcome", "historical_committed_evidence", row); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func committedHistoricalDisposition(outbox *evaluationconsistency.CommittedOutboxEvidence, references []evaluationconsistency.HistoricalReferenceEvidence) *evaluationconsistency.CommittedHistoricalEvidence {
	if outbox == nil || !outbox.LegacyCanonicalAbsent || outbox.Class != "" || outbox.RowCount != 0 || outbox.InvalidReason != canonicalMissingClassificationReason {
		return nil
	}
	result := &evaluationconsistency.CommittedHistoricalEvidence{Class: evidence.RetiredVerified, OutcomeID: outbox.OutcomeID, RunID: outbox.RunID}
	covered := false
	for _, reference := range references {
		if reference.Owner != "evaluation_outcome" {
			continue
		}
		if !reference.LegacyCanonicalAbsent || reference.OwnerID != outbox.OutcomeID || reference.EventType != "evaluation.outcome.committed" || reference.InvalidReason != "" || reference.RunID == "" || reference.RunID != outbox.RunID || reference.Attempt == 0 {
			return nil
		}
		switch reference.Class {
		case evidence.RetiredVerified:
		case evidence.Unverifiable:
			if reference.HistoricalReason == "" {
				return nil
			}
			result.Class = evidence.Unverifiable
			result.Reasons = append(result.Reasons, reference.HistoricalReason)
		default:
			return nil
		}
		covered = true
	}
	if !covered {
		return nil
	}
	return result
}
