package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func aiHostUnitPrivateDir(t *testing.T) string {
	t.Helper()
	directory, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(directory, 0700); e != nil {
		t.Fatal(e)
	}
	return directory
}
func aiHostUnitFlags(t *testing.T, mode string) (map[string]string, []string) {
	t.Helper()
	op := filepath.Join(aiHostUnitPrivateDir(t), "backups", "qs-server", "compatibility-retirement", "801-1")
	if e := os.MkdirAll(op, 0700); e != nil {
		t.Fatal(e)
	}
	f := map[string]string{"ai-host-mode": mode, "request": filepath.Join(op, "history.request.json"), "request-sha256": strings.Repeat("a", 64), "operation": "801-1", "run": "802-2", "output": filepath.Join(op, "ai-host-"+mode+"-802-2"), "operation-directory": op, "ai-input": filepath.Join(op, "ai-host-input.json"), "ai-input-sha256": strings.Repeat("b", 64)}
	var args []string
	for _, k := range []string{"ai-host-mode", "request", "request-sha256", "operation", "run", "output", "operation-directory", "ai-input", "ai-input-sha256"} {
		args = append(args, "--"+k, f[k])
	}
	return f, args
}
func TestAIHostCLIExactModesAndArguments(t *testing.T) {
	for _, mode := range []string{"bounds", "verify"} {
		t.Run(mode, func(t *testing.T) {
			_, args := aiHostUnitFlags(t, mode)
			if _, e := parseAIHostFlags(args); e != nil {
				t.Fatal(e)
			}
		})
	}
	_, args := aiHostUnitFlags(t, "bounds")
	for _, change := range []struct {
		name  string
		at    int
		value string
	}{{"unknownmode", 1, "write"}, {"uncanonicalrun", 9, "0802-2"}, {"zerorun", 9, "0-2"}, {"zeroattempt", 9, "802-0"}, {"sourcepathescape", 3, "/tmp/request.json"}, {"outputalias", 11, "/tmp/output"}, {"falsehash", 17, "true"}, {"importqualification", 14, "--qualification"}} {
		t.Run(change.name, func(t *testing.T) {
			bad := append([]string(nil), args...)
			bad[change.at] = change.value
			if _, e := parseAIHostFlags(bad); e == nil {
				t.Fatal("accepted incompatible/authority argument")
			}
		})
	}
	if _, e := parseAIHostFlags(append(args, "--ai-host-mode", "bounds")); e == nil {
		t.Fatal("accepted repeated mode")
	}
	if _, e := parseFlags(args); e == nil {
		t.Fatal("AI mode changed default readonly flag contract")
	}
}
func TestAIHostDescriptorBindingAndNoImportedAuthority(t *testing.T) {
	old := sourceSHA
	sourceSHA = strings.Repeat("c", 40)
	t.Cleanup(func() { sourceSHA = old })
	f, _ := aiHostUnitFlags(t, "bounds")
	dir := aiHostUnitPrivateDir(t)
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	v := aiHostDescriptor{FormatVersion: 1, Kind: "readonly_ai_external_host_input", SourceSHA: sourceSHA, OperationID: f["operation"], ActualRunID: f["run"], Mode: "bounds", RequestSHA256: f["request-sha256"], RuntimeSourceSHA: strings.Repeat("d", 40), ImageID: "sha256:" + strings.Repeat("e", 64), ContainerID: strings.Repeat("f", 64), RuntimeBindingSHA256: strings.Repeat("1", 64), AssetsDirectory: dir}
	if e := validateAIHostDescriptor(v, f); e != nil {
		t.Fatal(e)
	}
	for _, change := range []struct {
		name string
		fn   func(*aiHostDescriptor)
	}{
		{"differentrun", func(v *aiHostDescriptor) { v.ActualRunID = "800-1" }},
		{"differentrequest", func(v *aiHostDescriptor) { v.RequestSHA256 = strings.Repeat("2", 64) }},
		{"onepriorfield", func(v *aiHostDescriptor) { v.ExpectedAIHead = "0040_module_table_names" }},
		{"boundsimport", func(v *aiHostDescriptor) {
			v.AIBounds = &fileBinding{Path: filepath.Join(f["operation-directory"], "unapproved.json"), SHA256: strings.Repeat("a", 64)}
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			bad := v
			change.fn(&bad)
			if validateAIHostDescriptor(bad, f) == nil {
				t.Fatal("descriptor binding accepted")
			}
		})
	}
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	var value aiHostDescriptor
	if strictDecode(raw, &value) != nil {
		t.Fatal("valid descriptor cannot decode")
	}
	for _, extra := range []string{`"complete":true`, `"qualification":{}`, `"peer_connection":{"password":"secret"}`, `"ai_bounds":null`, `"peer_bounds":null`, `"protection":null`} {
		bad := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(","+extra+"}")...)
		if strictDecode(bad, &value) == nil {
			t.Fatal("accepted imported authority/connection")
		}
	}
}
func TestAIHostCLIMissingPrivateAssetFailsBeforeConnection(t *testing.T) {
	_, args := aiHostUnitFlags(t, "bounds")
	r, e := runAIHostCLI(context.Background(), args)
	if e == nil || safeCategory(e) != "history_ai_host_private_input_rejected" || r.DiagnosticReadComplete || r.Complete || r.IndependentApproval || r.WholeWriterFence || r.CASAuthority || r.RetirementWritten || r.DropReady {
		t.Fatalf("missing input did not fail closed: %s", safeCategory(e))
	}
	flags, pe := parseAIHostFlags(args)
	if pe != nil {
		t.Fatal(pe)
	}
	raw, re := os.ReadFile(filepath.Join(flags["output"], "ai-host.readiness.json"))
	if re != nil {
		t.Fatal(re)
	}
	var persisted aiHostReport
	if json.Unmarshal(raw, &persisted) != nil || persisted.ErrorCategory != r.ErrorCategory || persisted.DiagnosticReadComplete {
		t.Fatal("failed private diagnostic mismatch")
	}
	before, be := os.Stat(filepath.Join(flags["output"], "ai-host.readiness.json"))
	if be != nil {
		t.Fatal(be)
	}
	if _, e = runAIHostCLI(context.Background(), args); e == nil || safeCategory(e) != "history_ai_host_output_exists_or_unavailable" {
		t.Fatal("adopted previous attempt")
	}
	after, ae := os.Stat(filepath.Join(flags["output"], "ai-host.readiness.json"))
	if ae != nil || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("overwrote previous diagnostic")
	}
}
func TestAIHostPrivateInputAndOperationLockOwnership(t *testing.T) {
	flags, _ := aiHostUnitFlags(t, "bounds")
	path := flags["ai-input"]
	raw := []byte("private-test")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	a, e := readAIHostAsset(fileBinding{path, rawHash(raw)}, 1024)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := a.file.Close(); e != nil {
			t.Error(e)
		}
	})
	if a.recheck() != nil {
		t.Fatal("stable input rejected")
	}
	if e := os.WriteFile(path, []byte("changed-test"), 0600); e != nil {
		t.Fatal(e)
	}
	if a.recheck() == nil {
		t.Fatal("changed source accepted")
	}
	link := filepath.Join(flags["operation-directory"], "input-link.json")
	if e := os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if b, e := readAIHostAsset(fileBinding{link, rawHash(raw)}, 1024); e == nil {
		_ = b.file.Close()
		t.Fatal("symlink accepted")
	}
	lock, e := lockAIHostOperation(flags["operation-directory"])
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := lock.Close(); e != nil {
			t.Error(e)
		}
	})
	other, e := lockAIHostOperation(flags["operation-directory"])
	if e == nil {
		_ = other.Close()
		t.Fatal("concurrent lock accepted")
	}
}
