package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
)

// The trusted executor freezes the original stdout and actual process exit.
// This external evidence is neither a tool-generated seal nor a signature.
type captureExecutionEvidence struct {
	Format      int    `json:"format_version"`
	Kind        string `json:"kind"`
	ReceiptFile string `json:"receipt_file"`
	ReceiptHash string `json:"receipt_sha256"`
	Termination string `json:"termination"`
	ExitCode    *int   `json:"exit_code"`
}

func canonicalPrivatePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

// Only the new execution proof and its original stdout use this stricter wire
// shape. Compare JSON members/types without changing the trusted original bytes.
// In particular, Go's case-insensitive aliases and null/default bools are not
// evidence that the compiled capture actually emitted its literal receipt.
func decodeCaptureEvidence(raw []byte, target any) error {
	if e := strictDecode(raw, target); e != nil {
		return e
	}
	canonical, e := json.Marshal(target)
	if e != nil {
		return errors.New("capture_evidence_shape_rejected")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if token, e := d.Token(); e != nil || token != json.Delim('{') {
		return errors.New("capture_evidence_shape_rejected")
	}
	original := map[string]any{}
	for d.More() {
		token, e := d.Token()
		key, ok := token.(string)
		if _, duplicate := original[key]; e != nil || !ok || duplicate {
			return errors.New("capture_evidence_shape_rejected")
		}
		var value any
		if d.Decode(&value) != nil {
			return errors.New("capture_evidence_shape_rejected")
		}
		original[key] = value
	}
	if _, e := d.Token(); e != nil {
		return errors.New("capture_evidence_shape_rejected")
	}
	var typed any
	d = json.NewDecoder(bytes.NewReader(canonical))
	d.UseNumber()
	if d.Decode(&typed) != nil {
		return errors.New("capture_evidence_shape_rejected")
	}
	if !reflect.DeepEqual(original, typed) {
		return errors.New("capture_evidence_shape_rejected")
	}
	return nil
}

func readCaptureEvidence(m material, materialHash, tool, path, hash string) error {
	raw, e := readPrivate(path, hash, 32768)
	var execution captureExecutionEvidence
	if e != nil || decodeCaptureEvidence(raw, &execution) != nil || execution.Format != 1 || execution.Kind != "m4_a_capture_execution" || execution.Termination != "exited" || execution.ExitCode == nil || *execution.ExitCode != 0 || !canonicalPrivatePath(execution.ReceiptFile) || !nonzeroHash(execution.ReceiptHash) || path == m.CapturePath || execution.ReceiptFile == path || execution.ReceiptFile == m.CapturePath {
		return errors.New("original_capture_execution_evidence_rejected")
	}
	raw, e = readPrivate(execution.ReceiptFile, execution.ReceiptHash, 65536)
	var r receipt
	if e != nil || decodeCaptureEvidence(raw, &r) != nil || validateCaptureReceipt(r, m, materialHash, tool) != nil {
		return errors.New("original_capture_success_receipt_rejected")
	}
	return nil
}

func validateCaptureReceipt(r receipt, m material, hash, tool string) error {
	if r.Action != "capture" || !r.Complete || !r.ReadOnly || r.Error != "none" || r.ReleaseError != "" || !nonzeroHash(hash) || r.MaterialHash != hash || r.ToolSource != tool || r.ToolSource != m.ToolSource || r.ASource != aSource || r.AResources != aResources || r.AResourceFiles != aResourceFiles || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.BudgetExceeded || r.TransactionReleaseUnproven || r.ProductionFreezeCertified {
		return errors.New("original_capture_receipt_not_successful")
	}
	return nil
}
