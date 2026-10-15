package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureParentOwnerMustRemainEqualEvenWithSameDirectoryIdentity(t *testing.T) {
	path := filepath.Join(privateDir(t), "material.json")
	original, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = original.close() })
	changed := original
	changed.owner++
	if !os.SameFile(original.info, changed.info) {
		t.Fatal("owner witness must retain actual directory identity")
	}
	if checkCaptureParent(path, &changed) == nil || checkCaptureParent(path, &captureParentIdentity{}) == nil {
		t.Fatal("changed or unknown parent owner/identity accepted")
	}
}

func TestCaptureRechecksOriginalParentBeforeAndAfterPersistence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		after   bool
		replace bool
	}{
		{"ReplacedBeforeWrite", false, true},
		{"IneligibleBeforeWrite", false, false},
		{"ReplacedAfterReadback", true, true},
		{"IneligibleAfterReadback", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := privateDir(t)
			dir := filepath.Join(base, "capture")
			if e := os.Mkdir(dir, 0700); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(dir, "material.json")
			original, e := captureParent(path)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = original.close() })
			change := func() {
				if tc.replace {
					if os.Rename(dir, filepath.Join(base, "retained-original")) != nil || os.Mkdir(dir, 0700) != nil {
						t.Fatal("replace owned capture directory")
					}
				} else if os.Chmod(dir, 0777) != nil {
					t.Fatal("change owned parent eligibility")
				}
			}
			if !tc.after {
				change()
			}
			called := false
			r := receipt{}
			raw := []byte("fixed original material")
			e = finishCapture(context.Background(), &r, path, &original, raw, func(parent *captureParentIdentity, p string, value []byte) error {
				called = true
				if e := writeCaptureExclusive(parent, p, value); e != nil {
					return e
				}
				change()
				return nil
			})
			if e == nil || called != tc.after || r.Complete || r.MaterialHash != "" {
				t.Fatal("parent drift wrote early or certified material", e, called, r)
			}
			if tc.after {
				retained := path
				if tc.replace {
					retained = filepath.Join(base, "retained-original", "material.json")
				}
				got, e := os.ReadFile(retained)
				if e != nil || !bytes.Equal(got, raw) {
					t.Fatal("uncertified original material was removed or changed")
				}
			} else if _, e := os.Lstat(path); !os.IsNotExist(e) {
				t.Fatal("rejected pre-write parent still created material")
			}
		})
	}
}

func TestCaptureCertifiesMaterialOnlyWithUnchangedEligibleParent(t *testing.T) {
	path := filepath.Join(privateDir(t), "material.json")
	parent, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = parent.close() })
	raw := []byte("fixed original material")
	r := receipt{}
	if e := finishCapture(context.Background(), &r, path, &parent, raw, writeCaptureExclusive); e != nil || r.MaterialHash != fingerprint(raw) || r.Complete || parent.directory != nil {
		t.Fatal("unchanged durable capture rejected or prematurely completed", e, r)
	}
	if got, e := readPrivate(path, r.MaterialHash, maxMaterialBytes); e != nil || !bytes.Equal(got, raw) {
		t.Fatal("certified material did not retain original bytes")
	}
}

func TestCaptureRetainsOriginalDirectoryAcrossTransientPathSwap(t *testing.T) {
	for _, heldFD := range []bool{false, true} {
		name := "OldAbsoluteWriterLosesCanonicalMaterial"
		if heldFD {
			name = "HeldParentPersistsCanonicalMaterial"
		}
		t.Run(name, func(t *testing.T) {
			base := privateDir(t)
			dir := filepath.Join(base, "capture")
			if e := os.Mkdir(dir, 0700); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(dir, "material.json")
			parent, e := captureParent(path)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = parent.close() })
			retained := filepath.Join(base, "retained-original")
			substitute := filepath.Join(base, "substitute")
			raw := []byte("fixed original material")
			r := receipt{}
			e = finishCapture(context.Background(), &r, path, &parent, raw, func(original *captureParentIdentity, p string, value []byte) error {
				if os.Rename(dir, retained) != nil || os.Mkdir(dir, 0700) != nil {
					t.Fatal("swap original parent away")
				}
				var writeErr error
				if heldFD {
					writeErr = writeCaptureExclusive(original, p, value)
				} else {
					// Reproduce the prior absolute-path writer's gap while both
					// outer identity checks observe the genuine original parent.
					writeErr = writeExclusive(p, value)
				}
				if os.Rename(dir, substitute) != nil || os.Rename(retained, dir) != nil {
					t.Fatal("restore original canonical parent")
				}
				return writeErr
			})
			if e != nil || r.MaterialHash != fingerprint(raw) || r.Complete || parent.directory != nil {
				t.Fatal("controlled transient replacement did not reach certification", e, r)
			}
			canonical, canonicalErr := os.ReadFile(path)
			misdirected, misplacedErr := os.ReadFile(filepath.Join(substitute, "material.json"))
			if heldFD {
				if canonicalErr != nil || !bytes.Equal(canonical, raw) || !os.IsNotExist(misplacedErr) {
					t.Fatal("held-FD capture was misdirected or lost")
				}
			} else if !os.IsNotExist(canonicalErr) || misplacedErr != nil || !bytes.Equal(misdirected, raw) {
				t.Fatal("legacy absolute-path weakness was not actually reproduced")
			}
		})
	}
}

func TestCaptureClosedParentCannotCertifyMaterial(t *testing.T) {
	path := filepath.Join(privateDir(t), "material.json")
	parent, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = parent.close() })
	r := receipt{}
	e = finishCapture(context.Background(), &r, path, &parent, []byte("fixed"), func(original *captureParentIdentity, p string, value []byte) error {
		if e := writeCaptureExclusive(original, p, value); e != nil {
			return e
		}
		return original.directory.Close()
	})
	if e == nil || r.MaterialHash != "" || r.Complete || parent.close() == nil {
		t.Fatal("closed descriptor certified capture or concealed failed release", e, r)
	}
}

type captureReleasedContext struct {
	context.Context
	parent *captureParentIdentity
}

func (ctx captureReleasedContext) Err() error {
	if ctx.parent.directory == nil {
		return context.DeadlineExceeded
	}
	return ctx.Context.Err()
}

func TestCaptureChecksBudgetAfterReleasingOriginalParent(t *testing.T) {
	path := filepath.Join(privateDir(t), "material.json")
	parent, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = parent.close() })
	r := receipt{}
	ctx := captureReleasedContext{Context: context.Background(), parent: &parent}
	e = finishCapture(ctx, &r, path, &parent, []byte("fixed"), writeCaptureExclusive)
	if e == nil || e.Error() != "total_budget_exceeded" || parent.directory != nil || r.MaterialHash != "" || r.Complete {
		t.Fatal("release time escaped the action budget", e, r)
	}
}

func TestCaptureHeldParentNeverOverwritesExistingOrSymlinkEntry(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		name := "ExistingFile"
		if symlink {
			name = "Symlink"
		}
		t.Run(name, func(t *testing.T) {
			dir := privateDir(t)
			path := filepath.Join(dir, "material.json")
			retained := filepath.Join(dir, "retained.json")
			raw := []byte("retained original bytes")
			if e := os.WriteFile(retained, raw, 0600); e != nil {
				t.Fatal(e)
			}
			if symlink {
				if e := os.Symlink(retained, path); e != nil {
					t.Fatal(e)
				}
			} else if e := os.WriteFile(path, raw, 0600); e != nil {
				t.Fatal(e)
			}
			parent, e := captureParent(path)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = parent.close() })
			r := receipt{}
			if e := finishCapture(context.Background(), &r, path, &parent, []byte("replacement"), writeCaptureExclusive); e == nil || r.MaterialHash != "" || r.Complete {
				t.Fatal("existing or symlink entry was replaced or certified")
			}
			if got, e := os.ReadFile(path); e != nil || !bytes.Equal(got, raw) {
				t.Fatal("original entry bytes changed")
			}
			if got, e := os.ReadFile(retained); e != nil || !bytes.Equal(got, raw) {
				t.Fatal("unrelated retained bytes changed")
			}
		})
	}
}
