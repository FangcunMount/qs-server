package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureParentOwnerMustRemainEqualEvenWithSameDirectoryIdentity(t *testing.T) {
	path := filepath.Join(privateTestDir(t), "material.json")
	original, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	changed := original
	changed.owner++
	if !os.SameFile(original.info, changed.info) {
		t.Fatal("owner witness must retain actual directory identity")
	}
	if checkCaptureParent(path, changed) == nil || checkCaptureParent(path, captureParentIdentity{}) == nil {
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
		{"ReplacedAfterPersistence", true, true},
		{"IneligibleAfterPersistence", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := privateTestDir(t)
			dir := filepath.Join(base, "capture")
			if e := os.Mkdir(dir, 0700); e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(dir, "material.json")
			original, e := captureParent(path)
			if e != nil {
				t.Fatal(e)
			}
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
			e = finishCapture(context.Background(), &r, path, original, raw, func(p string, value []byte) error {
				called = true
				if e := writeExclusive(p, value); e != nil {
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
	path := filepath.Join(privateTestDir(t), "material.json")
	parent, e := captureParent(path)
	if e != nil {
		t.Fatal(e)
	}
	raw := []byte("fixed original material")
	r := receipt{}
	if e := finishCapture(context.Background(), &r, path, parent, raw, writeExclusive); e != nil || r.MaterialHash != fingerprint(raw) || r.Complete {
		t.Fatal("unchanged durable capture rejected or prematurely completed", e, r)
	}
	if got, e := readPrivate(path, r.MaterialHash, maxMaterialBytes); e != nil || !bytes.Equal(got, raw) {
		t.Fatal("certified material did not retain original bytes")
	}
}

func TestCaptureRequiresOriginalBytesReadbackBeforeCertification(t *testing.T) {
	for _, fault := range []string{"missing_file", "different_bytes"} {
		t.Run(fault, func(t *testing.T) {
			path := filepath.Join(privateTestDir(t), "material.json")
			parent, e := captureParent(path)
			if e != nil {
				t.Fatal(e)
			}
			r := receipt{}
			e = finishCapture(context.Background(), &r, path, parent, []byte("first original material"), func(p string, _ []byte) error {
				if fault == "different_bytes" {
					return writeExclusive(p, []byte("different bytes"))
				}
				return nil
			})
			if e == nil || r.MaterialHash != "" || r.Complete {
				t.Fatal("uncorroborated capture bytes certified", e, r)
			}
		})
	}
}
