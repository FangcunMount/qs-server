package compatibilityretirementbackup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestBackupOpenPrivateFileRegularPreservesBytesAndReadOnlyAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.private.json")
	original := []byte("private local fixture\x00\xff\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := openPrivateFile(path)
	if err != nil {
		t.Fatal("regular private file rejected", err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error("close private fixture", err)
		}
	})
	actual, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatal("regular file bytes changed", err)
	}
	if _, err := file.Write([]byte("mutation")); err == nil {
		t.Fatal("reader descriptor allowed a write")
	}
	visible, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(visible, original) {
		t.Fatal("reader modified source fixture", err)
	}
}

func TestBackupOpenPrivateFileRejectsSymlinkDirectoryAndPublicMode(t *testing.T) {
	dir := t.TempDir()
	private := filepath.Join(dir, "private.json")
	public := filepath.Join(dir, "public.json")
	link := filepath.Join(dir, "link.json")
	if err := os.WriteFile(private, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(public, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(public, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(private, link); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"symlink": link, "directory": dir, "public_mode": public} {
		t.Run(name, func(t *testing.T) {
			file, err := openPrivateFile(path)
			if file != nil {
				if closeErr := file.Close(); closeErr != nil {
					t.Error("close rejected fixture", closeErr)
				}
				t.Fatal("non-private or non-regular file returned a descriptor")
			}
			if !errors.Is(err, ErrPrivate) {
				t.Fatal("rejection changed the fixed private-file error", err)
			}
		})
	}
}

// The child deliberately opens a real FIFO without any writer. Keeping this in
// a separate process makes a blocking-open regression killable by the parent.
func TestBackupOpenPrivateFileFIFOChild(t *testing.T) {
	if os.Getenv("QS_COMPAT_BACKUP_READ_CHILD") != "1" {
		return
	}
	path := os.Getenv("QS_COMPAT_BACKUP_READ_PATH")
	if !filepath.IsAbs(path) {
		t.Fatal("FIFO child fixture path missing")
	}
	file, err := openPrivateFile(path)
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			t.Error("close rejected FIFO", closeErr)
		}
		t.Fatal("FIFO was accepted as a regular private file")
	}
	if !errors.Is(err, ErrPrivate) {
		t.Fatal("FIFO rejection changed the fixed error", err)
	}
}

func TestBackupOpenPrivateFileFIFOWithoutWriterReturnsBeforeDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.private.json")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal("create owned FIFO fixture", err)
	}
	metadata, err := os.Lstat(path)
	if err != nil || metadata.Mode()&os.ModeNamedPipe == 0 {
		t.Fatal("fixture is not an actual FIFO", err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestBackupOpenPrivateFileFIFOChild$", "-test.count=1", "-test.timeout=2s")
	command.Env = []string{"QS_COMPAT_BACKUP_READ_CHILD=1", "QS_COMPAT_BACKUP_READ_PATH=" + path}
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatal("private FIFO read blocked until subprocess deadline")
	}
	if err != nil {
		t.Fatalf("FIFO child did not reject immediately: %v: %s", err, output)
	}
}
