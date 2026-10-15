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

func readCaptureEvidence(m material, materialHash, tool, path, hash string) error {
	raw, e := readPrivateLimit(path, hash, 32768)
	var execution captureExecutionEvidence
	if e != nil || decodeCaptureDocument(raw, &execution) != nil || execution.Format != 1 || execution.Kind != "m4_c_capture_execution" || execution.Termination != "exited" || execution.ExitCode == nil || *execution.ExitCode != 0 || !absolutePath(execution.ReceiptFile) || !nonzeroHash(execution.ReceiptHash) || path == m.CapturePath || execution.ReceiptFile == path || execution.ReceiptFile == m.CapturePath {
		return errors.New("original_capture_execution_evidence_rejected")
	}
	raw, e = readPrivateLimit(execution.ReceiptFile, execution.ReceiptHash, 65536)
	var r receipt
	if e != nil || decodeCaptureDocument(raw, &r) != nil || validateCaptureReceipt(r, m, materialHash, tool) != nil {
		return errors.New("original_capture_success_receipt_rejected")
	}
	return nil
}

// encoding/json otherwise accepts case-insensitive struct aliases and turns
// null scalar members into zero values. Compare the exact member/value shape
// emitted by these existing DTOs; the original bytes and their SHA stay intact.
func decodeCaptureDocument(raw []byte, target any) error {
	if strictDecode(raw, target) != nil {
		return errors.New("capture_evidence_document_rejected")
	}
	canonical, e := json.Marshal(target)
	if e != nil {
		return errors.New("capture_evidence_document_rejected")
	}
	decode := func(data []byte) (any, error) {
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		var value any
		e := d.Decode(&value)
		return value, e
	}
	original, originalErr := decode(raw)
	expected, expectedErr := decode(canonical)
	if originalErr != nil || expectedErr != nil || !reflect.DeepEqual(original, expected) {
		return errors.New("capture_evidence_document_rejected")
	}
	return nil
}

func validateCaptureReceipt(r receipt, m material, hash, tool string) error {
	if r.Action != "capture" || !r.Complete || !r.ReadOnly || r.Error != "none" || r.ReleaseError != "" || !nonzeroHash(hash) || r.MaterialHash != hash || r.ToolSource != tool || r.ToolSource != m.ToolSource || r.CSource != cSource || r.CResources != cResources || r.CResourceFiles != cResourceFiles || !r.SQLSnapshot || !r.MandatoryEOF || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.BudgetExceeded || r.TransactionReleaseUnproven || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR || r.ApprovalConsumed || len(r.TableCounts) != len(m.Tables) {
		return errors.New("original_capture_receipt_not_successful")
	}
	for _, table := range m.Tables {
		count, ok := r.TableCounts[table.Table]
		if !ok || count != len(table.Rows) {
			return errors.New("original_capture_receipt_scope_rejected")
		}
	}
	return nil
}
