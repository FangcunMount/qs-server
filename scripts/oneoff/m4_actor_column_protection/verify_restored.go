package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
)

// A separate observation never changes a failed/unknown restore receipt and
// never consumes an operation or authorizes the first runtime command.
func verifyRestored(original, restored material) error {
	if original.Format != restored.Format || original.RuntimeBoundary != restored.RuntimeBoundary || original.Kind != restored.Kind || original.ToolSource != restored.ToolSource || original.CSource != restored.CSource || original.CResources != restored.CResources || original.CResourceFiles != restored.CResourceFiles || original.SessionZone != restored.SessionZone || original.SessionCharset != restored.SessionCharset || original.SessionSQLMode != restored.SessionSQLMode || original.Binding != restored.Binding || !bytes.Equal(original.MongoHead, restored.MongoHead) || original.LedgerDDL != restored.LedgerDDL || !reflect.DeepEqual(original.Ledger, restored.Ledger) {
		return errors.New("restored_original_identity_or_ledger_rejected")
	}
	if !reflect.DeepEqual(original.Tables, restored.Tables) {
		return errors.New("restored_original_columns_values_or_complete_range_rejected")
	}
	return nil
}

// Validate the original consumed responsibility, including unknown outcomes.
// Success of the previous restore process is deliberately not a prerequisite:
// COMMIT can have succeeded on the server while its reply was lost.
func verifyConsumedRecoveryInputs(ctx context.Context, m material, a approval, materialPath, materialHash, approvalPath, approvalHash, evidencePath, evidenceHash, tool string) error {
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	if e := rereadApprovedInputs(m, a, materialPath, materialHash, approvalPath, approvalHash, tool); e != nil {
		return e
	}
	if e := readCaptureEvidence(m, materialHash, tool, evidencePath, evidenceHash); e != nil {
		return e
	}
	if e := checkRecoveryBoundary(m, a, materialHash, true); e != nil {
		return e
	}
	path, e := recoveryJournalPath(m, materialHash)
	if e != nil {
		return e
	}
	raw, e := readOwnedPrivate(path, maxRecoveryJournalBytes)
	operations, parseError := parseRecoveryOperations(raw, materialHash)
	if e != nil || parseError != nil {
		return errors.New("recovery_operation_journal_invalid_or_lost")
	}
	found := false
	for _, operation := range operations {
		if operation.Operation == a.Operation {
			found = operation.ApprovalHash == approvalHash && operation.MaterialHash == materialHash
		}
	}
	if !found {
		return errors.New("original_recovery_approval_not_consumed")
	}
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	return nil
}
