package compatibilityretirementbackup

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestResumeJournalAncestorSiblingChangesPreserveOriginalDirectory(t *testing.T) {
	dir, _, _, _ := targetResumeFixtureJournal(t)
	dirs, err := openTargetJournalDirectories(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeTargetJournalDirectories(dirs) != nil {
			t.Error("owned directory close failed")
		}
	}()
	sibling := filepath.Join(filepath.Dir(dir), "unrelated-sibling")
	if err = os.Mkdir(sibling, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if os.Remove(sibling) != nil {
			t.Error("owned sibling cleanup failed")
		}
	})
	if err = verifyTargetJournalDirectories(dirs); err != nil {
		t.Fatalf("unrelated ancestor child changed original directory identity: %v", err)
	}
}

func TestResumeJournalLeafChildAndNamedReplacementRemainRejected(t *testing.T) {
	for _, kind := range []string{"leaf-child", "named-replacement"} {
		t.Run(kind, func(t *testing.T) {
			dir, _, _, _ := targetResumeFixtureJournal(t)
			dirs, err := openTargetJournalDirectories(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if closeTargetJournalDirectories(dirs) != nil {
					t.Error("owned directory close failed")
				}
			}()
			switch kind {
			case "leaf-child":
				child := filepath.Join(dir, "unexpected-child")
				if err = os.Mkdir(child, 0700); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if os.Remove(child) != nil {
						t.Error("owned leaf child cleanup failed")
					}
				})
				var current unix.Stat_t
				if unix.Fstat(dirs[len(dirs)-1].fd, &current) != nil {
					t.Fatal("owned leaf metadata unavailable")
				}
				if current.Nlink != dirs[len(dirs)-1].stamp.Nlink && verifyTargetJournalDirectories(dirs) != ErrRecoveryJournal {
					t.Fatal("private journal leaf link change accepted")
				}
				// Filesystem link counts vary. The private listing must always
				// retain the original journal names on every platform.
				names, e := listTargetJournalFiles(dirs[len(dirs)-1].fd)
				if e != nil {
					t.Fatal(e)
				}
				found := false
				for _, name := range names {
					if !targetJournalNameSupported(name) {
						found = true
					}
				}
				if !found {
					t.Fatal("private journal child was hidden")
				}
			case "named-replacement":
				moved := dir + "-moved"
				if err = os.Rename(dir, moved); err != nil {
					t.Fatal(err)
				}
				if err = os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if os.Remove(dir) != nil || os.Rename(moved, dir) != nil {
						t.Error("owned replacement cleanup failed")
					}
				})
				if verifyTargetJournalDirectories(dirs) != ErrRecoveryJournal {
					t.Fatal("replaced journal directory accepted")
				}
			}
		})
	}
}
