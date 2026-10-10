package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are actual private directories owned by the current test UID. They
// exercise only filesystem primitives; they do not issue acceptance, a Window,
// a remote-zero proof, Docker authority or a full production material catalog.
func materialTestDirectory(t *testing.T, path string) *lifecycleMaterialDirectory {
	t.Helper()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := openLifecycleMaterialDirectory(path, uint32(os.Getuid()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.close(); err != nil {
			t.Error(err)
		}
	})
	return d
}
func materialTestFile(t *testing.T, d *lifecycleMaterialDirectory, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(d.path, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.register(name, digestRaw([]byte(body)), uint32(os.Getuid()), 0600); err != nil {
		t.Fatal(err)
	}
}
func materialTestBinding() lifecycleMaterialBinding {
	return lifecycleMaterialBinding{strings.Repeat("a", 40), strings.Repeat("b", 40), "123-1", "124-1", "125-1", strings.Repeat("c", 64), strings.Repeat("d", 64), strings.Repeat("e", 64)}
}
func materialTestBatch(t *testing.T) *lifecycleBatchMaterials {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	c := &lifecycleBatchMaterials{binding: materialTestBinding(), directories: []*lifecycleMaterialDirectory{materialTestDirectory(t, filepath.Join(root, "sources"))}, archive: materialTestDirectory(t, filepath.Join(root, "archive")), journal: materialTestDirectory(t, filepath.Join(root, "receipts"))}
	c.self = c
	t.Cleanup(func() {
		if err := c.close(); err != nil {
			t.Error(err)
		}
	})
	return c
}
func TestMaterialDirectoryDeletesOnlyExactRegisteredBodies(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	d := materialTestDirectory(t, filepath.Join(root, "current-batch"))
	child := materialTestDirectory(t, filepath.Join(d.path, "body-copy"))
	materialTestFile(t, d, "source.bsonframes", "original private body")
	materialTestFile(t, child, "copy.ndjson", "temporary recovery body")
	if err := d.registerChild(child); err != nil {
		t.Fatal(err)
	}
	receipt := []byte(`{"body_sha256":"immutable-summary"}`)
	if err := os.WriteFile(filepath.Join(d.path, "receipt.private.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.retainReceipt("receipt.private.json", digestRaw(receipt), uint32(os.Getuid())); err != nil {
		t.Fatal(err)
	}
	ordinary := filepath.Join(root, "ordinary-backup")
	prior := filepath.Join(root, "prior-cleanup-archive")
	for _, path := range []string{ordinary, prior} {
		if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.checkComplete(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(child.path); !os.IsNotExist(err) {
		t.Fatal("owned empty child remains")
	}
	if raw, err := os.ReadFile(filepath.Join(d.path, "receipt.private.json")); err != nil || string(raw) != string(receipt) {
		t.Fatal("receipt altered")
	}
	for _, path := range []string{ordinary, prior} {
		if raw, err := os.ReadFile(path); err != nil || string(raw) != "preserve" {
			t.Fatal("nonbatch material changed")
		}
	}
	if err := d.close(); err != nil {
		t.Fatal(err)
	}
	for _, v := range d.files {
		if v.file != nil {
			t.Fatal("raw FD survived cleanup")
		}
	}
}
func TestMaterialDirectoryRejectsChangesBeforeAnyDeletion(t *testing.T) {
	for _, kind := range []string{"foreign", "foreign-child", "same-inode-bytes", "replacement-inode", "hardlink", "mode", "directory-replacement", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			d := materialTestDirectory(t, filepath.Join(root, "batch"))
			materialTestFile(t, d, "a-source", "known body")
			child := materialTestDirectory(t, filepath.Join(d.path, "nested"))
			materialTestFile(t, child, "b-copy", "copy body")
			if err := d.registerChild(child); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(d.path, "a-source")
			switch kind {
			case "foreign":
				if err := os.WriteFile(filepath.Join(d.path, "foreign"), []byte("unregistered"), 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign-child":
				if err := os.WriteFile(filepath.Join(child.path, "foreign"), []byte("unregistered"), 0600); err != nil {
					t.Fatal(err)
				}
			case "same-inode-bytes":
				if err := os.WriteFile(path, []byte("other body"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replacement-inode":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("known body"), 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(root, "linked")); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0640); err != nil {
					t.Fatal(err)
				}
			case "directory-replacement":
				if err := os.Rename(d.path, d.path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(d.path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("known body"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			}
			if err := d.purge(context.Background()); err == nil {
				t.Fatal("changed/foreign material accepted")
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatal("preflight removed a source")
			}
			childPath := child.path
			if kind == "directory-replacement" {
				childPath = filepath.Join(d.path+".original", "nested")
			}
			if _, err := os.Lstat(filepath.Join(childPath, "b-copy")); err != nil {
				t.Fatal("preflight removed another source")
			}
		})
	}
}
func TestMaterialRegistrationRejectsAliasesWrongHashAndUnsafeMetadata(t *testing.T) {
	for _, kind := range []string{"parent", "absolute", "wrong-hash", "wrong-uid", "group-writable", "symlink", "hardlink", "duplicate", "closed-fd"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			d := materialTestDirectory(t, filepath.Join(root, "batch"))
			path := filepath.Join(d.path, "body")
			if err := os.WriteFile(path, []byte("body"), 0600); err != nil {
				t.Fatal(err)
			}
			name, hash, uid, mode := "body", digestRaw([]byte("body")), uint32(os.Getuid()), os.FileMode(0600)
			switch kind {
			case "parent":
				name = "../body"
			case "absolute":
				name = path
			case "wrong-hash":
				hash = digestRaw([]byte("different"))
			case "wrong-uid":
				uid++
			case "group-writable":
				mode = 0620
			case "symlink":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".old", path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".other"); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				if err := d.register(name, hash, uid, mode); err != nil {
					t.Fatal(err)
				}
			case "closed-fd":
				if err := d.close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := d.register(name, hash, uid, mode); err == nil {
				t.Fatal("unsafe registration accepted")
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatal("registration performed deletion")
			}
		})
	}
}
func TestMaterialBatchRequiresArchiveZeroAndClosesAllSourceFDs(t *testing.T) {
	c := materialTestBatch(t)
	materialTestFile(t, c.directories[0], "source", "source body")
	materialTestFile(t, c.archive, "archive.body", "backup body")
	originalFD := c.directories[0].files["source"].file
	archiveFD := c.archive.files["archive.body"].file
	if err := c.purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.zeroVerified {
		t.Fatal("purge alone became completed zero verification")
	}
	if _, err := os.Lstat(filepath.Join(c.archive.path, "archive.body")); err != nil {
		t.Fatal("host deleted external archive")
	}
	// Simulate only the separate archive helper's actual filesystem unlink. This
	// does not issue production acceptance or exercise any Docker/DB path.
	if err := os.Remove(filepath.Join(c.archive.path, "archive.body")); err != nil {
		t.Fatal(err)
	}
	if err := c.verifyZero(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !c.zeroVerified {
		t.Fatal("actual zero and FD closure did not complete the native phase")
	}
	for _, fd := range []*os.File{originalFD, archiveFD} {
		if _, err := fd.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatal("unlinked source FD still holds content")
		}
	}
	raw, err := os.ReadFile(filepath.Join(c.journal.path, "material-purge.intent.private.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"source body", "backup body"} {
		if strings.Contains(string(raw), body) {
			t.Fatal("receipt leaked body")
		}
	}
	var receipt map[string]any
	if json.Unmarshal(raw, &receipt) != nil || receipt["binding_sha256"] != c.binding.digest() || receipt["drop_authority"] != false {
		t.Fatal("receipt binding invalid")
	}
}
func TestMaterialZeroNeverTreatsArchiveAbsenceAsWholeScope(t *testing.T) {
	for _, kind := range []string{"not-purged", "raw-archive-remains", "foreign-after-purge", "closed-before-zero"} {
		t.Run(kind, func(t *testing.T) {
			c := materialTestBatch(t)
			materialTestFile(t, c.directories[0], "source", "source body")
			if kind == "raw-archive-remains" {
				materialTestFile(t, c.archive, "archive.body", "backup body")
			}
			if kind != "not-purged" {
				if err := c.purge(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "foreign-after-purge" {
				if err := os.WriteFile(filepath.Join(c.directories[0].path, "foreign"), []byte("unregistered"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "closed-before-zero" {
				if err := c.close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := c.verifyZero(context.Background()); err == nil {
				t.Fatal("incomplete scope appeared zero")
			}
			if c.zeroVerified {
				t.Fatal("failed zero verification became final completion")
			}
		})
	}
}
func TestMaterialExclusiveIntentUnknownAndCancellationRetainBodies(t *testing.T) {
	c := materialTestBatch(t)
	materialTestFile(t, c.directories[0], "source", "source body")
	materialTestFile(t, c.journal, "material-purge.intent.private.json", "prior intent")
	if err := c.purge(context.Background()); err == nil || !c.unknown || !c.started {
		t.Fatal("exclusive intent failure was not retained unknown")
	}
	if err := c.purge(context.Background()); err == nil {
		t.Fatal("unknown mutation retried")
	}
	if raw, err := os.ReadFile(filepath.Join(c.directories[0].path, "source")); err != nil || string(raw) != "source body" {
		t.Fatal("unknown deleted body")
	}
	if raw, err := os.ReadFile(filepath.Join(c.journal.path, "material-purge.intent.private.json")); err != nil || string(raw) != "prior intent" {
		t.Fatal("old intent overwritten")
	}
	c2 := materialTestBatch(t)
	materialTestFile(t, c2.directories[0], "source", "retain")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c2.purge(ctx); err == nil {
		t.Fatal("cancelled purge allowed")
	}
	if _, err := os.Lstat(filepath.Join(c2.directories[0].path, "source")); err != nil {
		t.Fatal("cancelled purge removed source")
	}
}
func TestMaterialHostRejectsJSONReceiptsAndMissingRealAcceptance(t *testing.T) {
	for _, raw := range []string{`{}`, `{"success":true,"acceptance_complete":true,"purge_complete":true}`, `{"self":{},"host":{},"catalog":{"complete":true}}`} {
		var saved lifecycleAcceptedMaterials
		if err := json.Unmarshal([]byte(raw), &saved); err != nil { //nolint:staticcheck // SA9005 is intentional: JSON has no access to the private capability fields.
			t.Fatal(err)
		}
		h := &lifecycleFixedHost{acceptedMaterials: &saved}
		if err := h.PurgeTemporaryCopies(context.Background(), lifecycleRequest{}); err == nil {
			t.Fatal("saved receipt minted purge")
		}
		if err := h.VerifyTemporaryMaterialsZero(context.Background(), lifecycleRequest{}); err == nil {
			t.Fatal("saved receipt minted zero")
		}
	}
	h := &lifecycleFixedHost{}
	for _, f := range []func() error{
		func() error { return h.PurgeTemporaryCopies(context.Background(), lifecycleRequest{}) },
		func() error { return h.VerifyTemporaryMaterialsZero(context.Background(), lifecycleRequest{}) },
		func() error { return h.VerifyAcceptance(context.Background(), lifecycleRequest{}, nil) },
		func() error { return h.CheckWholeWriterFence(context.Background(), lifecycleRequest{}) },
		func() error { return lifecycleEffectsPreflight(context.Background()) },
	} {
		if f() == nil {
			t.Fatal("missing real producer granted effects")
		}
	}
}
func TestMaterialEngineRegistrationRejectsUnownedAndNonterminalBeforeNativeCalls(t *testing.T) {
	for _, e := range []*lifecycleOwnedEngine{nil, {}, {ID: strings.Repeat("a", 64)}, {ID: strings.Repeat("a", 64), wireClosed: true, Labels: map[string]string{}}, {ID: strings.Repeat("a", 64), wireClosed: true, wires: []*lifecycleWireConn{{done: make(chan struct{})}}}} {
		if _, err := registerLifecycleEnginePurge(context.Background(), materialTestBinding(), e); err == nil {
			t.Fatal("unowned/nonterminal engine adopted")
		}
	}
}
func TestMaterialBindingChangesOriginalProducerRunAndEveryDigest(t *testing.T) {
	a := materialTestBinding()
	base := a.digest()
	for _, change := range []func(*lifecycleMaterialBinding){func(x *lifecycleMaterialBinding) { x.original = "x" }, func(x *lifecycleMaterialBinding) { x.tool = "x" }, func(x *lifecycleMaterialBinding) { x.operation = "x" }, func(x *lifecycleMaterialBinding) { x.originalRun = "x" }, func(x *lifecycleMaterialBinding) { x.actualRun = "x" }, func(x *lifecycleMaterialBinding) { x.manifest = "x" }, func(x *lifecycleMaterialBinding) { x.archive = "x" }, func(x *lifecycleMaterialBinding) { x.request = "x" }} {
		b := a
		change(&b)
		if b.digest() == base {
			t.Fatal("binding projection omitted a field")
		}
	}
}

func TestMaterialRegisteredExternalArchiveCannotEscapeMandatoryZero(t *testing.T) {
	c := materialTestBatch(t)
	root := materialTestDirectory(t, filepath.Join(filepath.Dir(c.archive.path), "combined-scope"))
	archivePath := filepath.Join(root.path, "archive")
	archive := materialTestDirectory(t, archivePath)
	archive.externalArchive = true
	materialTestFile(t, archive, "archive.body", "raw restore material")
	materialTestFile(t, root, "source", "raw original material")
	if err := root.registerChild(archive); err != nil {
		t.Fatal(err)
	}
	c.directories = []*lifecycleMaterialDirectory{root}
	c.archive = archive
	if err := c.purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(archive.path, "archive.body")); err != nil || string(raw) != "raw restore material" {
		t.Fatal("host adopted archive purge responsibility")
	}
	if err := c.verifyZero(context.Background()); err == nil || !c.unknown {
		t.Fatal("external archive exemption minted whole zero")
	}
	// Unknown is sticky: external deletion afterward does not complete this run.
	if err := os.Remove(filepath.Join(archive.path, "archive.body")); err != nil {
		t.Fatal(err)
	}
	if err := c.verifyZero(context.Background()); err == nil {
		t.Fatal("unknown zero observation was replayed")
	}
}
func TestMaterialEmptySourceStillHasRealIdentityAndFingerprint(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	d := materialTestDirectory(t, filepath.Join(root, "empty"))
	materialTestFile(t, d, "empty.bsonframes", "")
	if err := d.purge(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(d.path, "empty.bsonframes")); !os.IsNotExist(err) {
		t.Fatal("empty source remained")
	}
}

func TestMaterialHostCannotReplaceActualPreparationOwner(t *testing.T) {
	original := &lifecyclePreparationOwner{}
	for _, host := range []*lifecycleFixedHost{nil, {restoreOwner: original}, {owner: original}} {
		if _, err := host.Prepare(context.Background(), lifecycleRequest{}, nil); err == nil {
			t.Fatal("repeat preparation adopted new resources")
		}
		if host != nil && host.restoreOwner != original && host.owner != original {
			t.Fatal("original owner was lost")
		}
	}
}

func TestMaterialNativeSetRejectsMissingDuplicateAndForeignWithoutAdoption(t *testing.T) {
	for _, v := range []struct {
		raw      string
		expected []string
		valid    bool
	}{
		{"", nil, true}, {"first\nsecond\n", []string{"second", "first"}, true},
		{"foreign\n", nil, false}, {"first\nfirst\n", []string{"first", "second"}, false},
		{"first\n", []string{"first", "second"}, false}, {"first\nforeign\n", []string{"first", "second"}, false},
		{"first\n", []string{"first", "first"}, false},
	} {
		if lifecycleMaterialNativeSet([]byte(v.raw), v.expected) != v.valid {
			t.Fatal("native actual set did not match exact registration")
		}
	}
	if err := checkLifecycleRestoreMaterialSet(context.Background(), []*lifecycleEnginePurge{nil}, false); err == nil {
		t.Fatal("unowned inventory accepted")
	}
}
