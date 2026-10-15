package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
)

// The trusted executor freezes the original stdout and actual process exit.
// This is external execution evidence, not a signature or a tool-issued seal.
type captureExecutionEvidence struct {
	Format      int    `json:"format_version"`
	Kind        string `json:"kind"`
	Termination string `json:"termination"`
	ExitCode    *int   `json:"exit_code"`
	ReceiptFile string `json:"receipt_file"`
	ReceiptHash string `json:"receipt_sha256"`
}

// Original CLI documents use the literal typed members, including explicit
// false and zero values. encoding/json alone also accepts aliases and nulls.
// Compare decoded trees without rewriting the independently frozen raw bytes.
func decodeCaptureDocument(raw []byte, value any) error {
	if e := strictDecode(raw, value); e != nil {
		return e
	}
	canonical, e := json.Marshal(value)
	if e != nil {
		return errors.New("capture_document_shape_rejected")
	}
	var trees [2]any
	for i, body := range [][]byte{raw, canonical} {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if decoder.Decode(&trees[i]) != nil {
			return errors.New("capture_document_shape_rejected")
		}
	}
	if !reflect.DeepEqual(trees[0], trees[1]) {
		return errors.New("capture_document_shape_rejected")
	}
	return nil
}

func readCaptureEvidence(m material, materialHash, tool, path, hash string) error {
	raw, e := readPrivateLimit(path, hash, 32768)
	var execution captureExecutionEvidence
	if e != nil || decodeCaptureDocument(raw, &execution) != nil || execution.Format != 1 || execution.Kind != "m4_d_capture_execution" || execution.Termination != "exited" || execution.ExitCode == nil || *execution.ExitCode != 0 || !absolutePath(execution.ReceiptFile) || !nonzeroHash(execution.ReceiptHash) || path == m.CapturePath || execution.ReceiptFile == path || execution.ReceiptFile == m.CapturePath {
		return errors.New("original_capture_execution_evidence_rejected")
	}
	raw, e = readPrivateLimit(execution.ReceiptFile, execution.ReceiptHash, 65536)
	var r receipt
	if e != nil || decodeCaptureDocument(raw, &r) != nil || validateCaptureReceipt(r, m, materialHash, tool) != nil {
		return errors.New("original_capture_success_receipt_rejected")
	}
	return nil
}

func validateCaptureReceipt(r receipt, m material, hash, tool string) error {
	if r.DatabaseBoundary == nil || *r.DatabaseBoundary != m.RuntimeBoundary || r.DatabaseBoundaryRecord != nil || r.DatabaseBoundaryAdvanced || r.DatabaseBoundaryAdvanceUnknown {
		return errors.New("original_capture_receipt_not_successful")
	}
	if r.Action != "capture" || !r.Complete || !r.ReadOnly || r.Error != "none" || r.ReleaseError != "" || !nonzeroHash(hash) || r.MaterialHash != hash || r.ToolSource != tool || r.ToolSource != m.ToolSource || r.DSource != dSource || r.DResources != dResources || r.DResourceFiles != dResourceFiles || !r.SQLSnapshot || !r.MandatoryEOF || !r.RetainedEOF || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.BudgetExceeded || r.TransactionReleaseUnproven || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR || r.ApprovalConsumed || r.RuntimeGateAcquired || r.RuntimeGateReleased || r.RuntimeGateReleaseUnproven || r.MarkerWritten || r.MarkerWriteUnknown || r.MarkerHash != "" || len(r.TableCounts) != len(m.Tables) || len(r.RetainedCounts) != len(m.Retained) {
		return errors.New("original_capture_receipt_not_successful")
	}
	for _, table := range m.Tables {
		count, ok := r.TableCounts[table.Table]
		if !ok || count != len(table.Rows) {
			return errors.New("original_capture_receipt_scope_rejected")
		}
	}
	for _, table := range m.Retained {
		count, ok := r.RetainedCounts[table.Table]
		if !ok || count != len(table.Rows) {
			return errors.New("original_capture_receipt_scope_rejected")
		}
	}
	return nil
}
