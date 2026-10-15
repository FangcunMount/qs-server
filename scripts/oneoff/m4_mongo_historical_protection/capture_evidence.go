package main

import (
	"bytes"
	"encoding/json"
	"errors"
)

const maxCaptureExecutionBytes = 32 << 10
const maxCaptureReceiptBytes = 64 << 10

type captureEvidenceInput struct{ path, hash string }

// The trusted executor freezes the original stdout and observed process exit.
// The protection tool never creates or repairs this independent evidence. SHA
// equality binds those approved bytes; it is not a cryptographic signature.
type captureExecution struct {
	Format      int    `json:"format_version"`
	Kind        string `json:"kind"`
	ReceiptFile string `json:"receipt_file"`
	ReceiptHash string `json:"receipt_sha256"`
	Termination string `json:"termination"`
	ExitCode    *int   `json:"exit_code"`
}

func (input captureEvidenceInput) check(m material, materialHash, tool string) error {
	raw, e := readPrivate(input.path, input.hash, maxCaptureExecutionBytes)
	var execution captureExecution
	if e != nil || decodeCaptureDocument(raw, &execution, []string{"format_version", "kind", "receipt_file", "receipt_sha256", "termination", "exit_code"}, nil) != nil || execution.Format != 1 || execution.Kind != "m4_b_capture_execution" || !absolutePath(execution.ReceiptFile) || !nonzeroHash(execution.ReceiptHash) || execution.Termination != "exited" || execution.ExitCode == nil || *execution.ExitCode != 0 {
		return errors.New("capture_execution_evidence_rejected")
	}
	raw, e = readPrivate(execution.ReceiptFile, execution.ReceiptHash, maxCaptureReceiptBytes)
	var captured receipt
	if e != nil || decodeCaptureReceipt(raw, &captured) != nil || validateCaptureReceipt(captured, m, materialHash, tool) != nil {
		return errors.New("capture_success_receipt_rejected")
	}
	return nil
}

func decodeCaptureReceipt(raw []byte, captured *receipt) error {
	// encoding/json discards surplus elements when decoding a fixed array.
	// Check the original stdout shape before decoding its two observed counts.
	var shape struct {
		Counts []json.RawMessage `json:"exact_fixed_collection_counts"`
	}
	if json.Unmarshal(raw, &shape) != nil || len(shape.Counts) != len(captured.Counts) {
		return errors.New("capture_receipt_count_shape_rejected")
	}
	return decodeCaptureDocument(raw, captured, captureReceiptFields, []string{"release_error_category", "restored_verification"})
}

var captureReceiptFields = []string{
	"action", "compiled_tool_source", "b_execution_source", "b_execution_resources_sha256", "b_execution_resource_files",
	"complete", "database_read_only", "production_freeze_certified", "material_sha256", "mysql_identity_sha256", "mongo_identity_sha256", "mysql_clean_version", "mongo_clean_version", "exact_fixed_collection_counts",
	"sql_connection_closed", "sql_pool_closed", "mongo_closed", "mongo_cursors_exhausted_and_closed", "ledger_commit_issued", "ledger_commit_acknowledged", "ledger_commit_outcome_unknown", "application_ledger_abort_called", "mongo_snapshot_session_end_called", "mongo_transaction_release_unproven", "sql_transaction_release_unproven", "error_category", "total_budget_exceeded",
}

func validateCaptureReceipt(r receipt, m material, materialHash, tool string) error {
	if validateMaterial(m, tool, m.SQLName, m.MongoName) != nil || !nonzeroHash(materialHash) || r.Action != "capture" || !r.Complete || r.Error != "none" || !r.ReadOnly || r.FreezeCertified || r.MaterialHash != materialHash || r.ToolSource != tool || r.BSource != m.BSource || r.BResources != m.BResources || r.BResourceFiles != m.BResourceFiles || r.SQLIdentity != m.SQLIdentity || r.MongoIdentity != m.MongoIdentity || r.SQLVersion != m.SQLVersion || r.MongoVersion != 39 || r.ReleaseError != "" || r.BudgetExceeded || r.TransactionReleaseUnproven || r.MongoTransactionReleaseUnproven || r.LedgerCommitIssued || r.LedgerCommitAcknowledged || r.LedgerCommitOutcomeUnknown || r.LedgerAbortCalled || !r.SessionEndCalled || r.RestoredVerification != nil {
		return errors.New("capture_receipt_scope_or_completion_rejected")
	}
	for _, released := range []*bool{r.SQLConnectionClosed, r.SQLPoolClosed, r.MongoClosed, r.MongoCursorsClosed} {
		if released == nil || !*released {
			return errors.New("capture_receipt_release_unproven")
		}
	}
	for i, count := range r.Counts {
		if count == nil || *count != *m.Collections[i].Count {
			return errors.New("capture_receipt_count_rejected")
		}
	}
	return nil
}

// Only the new executor evidence and original stdout require this additional
// duplicate-member check. The original material/approval encoding is unchanged.
func decodeCaptureDocument(raw []byte, target any, required, optional []string) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, e := d.Token()
		if e != nil {
			return e
		}
		if delimiter, ok := token.(json.Delim); ok {
			seen := map[string]bool{}
			for d.More() {
				if delimiter == '{' {
					key, e := d.Token()
					name, ok := key.(string)
					if e != nil || !ok || seen[name] {
						return errors.New("capture_evidence_duplicate_member")
					}
					seen[name] = true
				}
				if e := walk(); e != nil {
					return e
				}
			}
			_, e = d.Token()
		}
		return e
	}
	if walk() != nil || strictDecode(raw, target) != nil {
		return errors.New("capture_evidence_document_rejected")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return errors.New("capture_evidence_document_rejected")
	}
	allowed := make(map[string]bool, len(required)+len(optional))
	for _, name := range required {
		value, present := fields[name]
		if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("capture_evidence_required_member_missing")
		}
		allowed[name] = true
	}
	for _, name := range optional {
		if value, present := fields[name]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("capture_evidence_null_member")
		}
		allowed[name] = true
	}
	for name := range fields {
		// encoding/json also accepts case-insensitive aliases of struct tags.
		// The original stdout/envelope schema permits only the exact names.
		if !allowed[name] {
			return errors.New("capture_evidence_unknown_member")
		}
	}
	return nil
}
