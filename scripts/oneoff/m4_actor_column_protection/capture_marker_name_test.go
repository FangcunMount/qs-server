package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureCannotOccupyReservedRuntimeMarker(t *testing.T) {
	m := fixtureMaterial(t)
	dir := privateRecoveryDir(t)
	path := filepath.Join(dir, "first-runtime")
	b := m.Binding
	args := []string{"--action=capture", "--mysql-database=" + b.SQLName, "--mongo-database=" + b.MongoName, "--mysql-identity-sha256=" + b.SQLIdentity, "--mongo-identity-sha256=" + b.MongoIdentity, "--mongo-ledger-uuid-sha256=" + b.MongoUUID, "--capture-file=" + path}
	r := run(t.Context(), args, func(string) string { t.Fatal("reserved name reached ENV"); return "" }, fixtureSource)
	if r.Complete || r.Error != "capture_reserved_runtime_marker_rejected" || r.SQLConnectionClosed || r.SQLPoolClosed || r.MongoClosed {
		t.Fatal("capture consumed its later marker", r)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("reserved name created a file")
	}
	m.CapturePath = path
	if validateMaterial(m, fixtureSource, b) == nil {
		t.Fatal("hand-bound material retained the reserved marker path")
	}
	// The adjacent ordinary filename still passes the real held-parent writer.
	path = filepath.Join(dir, "first-capture.json")
	parent, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := parent.close(); e != nil {
			t.Error(e)
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), queryLimit)
	defer cancel()
	raw := []byte("owned original capture writer bytes")
	r = receipt{}
	if e := finishCapture(ctx, &r, path, &parent, raw, writeCaptureExclusive); e != nil || r.MaterialHash != fingerprint(raw) {
		t.Fatal("ordinary capture writer changed", e)
	}
	if got, e := readPrivate(path, r.MaterialHash); e != nil || string(got) != string(raw) {
		t.Fatal("ordinary exact bytes not retained", e)
	}
}
