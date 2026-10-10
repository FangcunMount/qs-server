package compatibilityretirementbackup

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestHostHistoricalSourcesRejectsZeroCopiedClosedAndSerialization(t *testing.T) {
	for _, s := range []*HostHistoricalSources{nil, {}} {
		if _, e := s.Binding(t.Context()); e == nil {
			t.Fatal("zero binding trusted")
		}
		if _, e := s.Copies(t.Context()); e == nil {
			t.Fatal("zero source files trusted")
		}
		if e := s.Verify(t.Context()); e == nil {
			t.Fatal("zero source archive trusted")
		}
	}
	if _, e := json.Marshal(&HostHistoricalSources{}); e == nil {
		t.Fatal("physical FD lease serialized")
	}
	if s, e := OpenHostHistoricalSources(t.Context(), &Archive{}, Approval{}); e == nil || s != nil {
		t.Fatal("zero archive opened")
	}
}

func TestHistoricalFDReadbackRejectsReplacementMetadataHardlinkAndCancelledRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "source")
	raw := []byte("private-fixture-bytes")
	if os.WriteFile(p, raw, 0600) != nil {
		t.Fatal("fixture file failed")
	}
	f, e := os.Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if f.Close() != nil {
			t.Error("close")
		}
	}()
	original, e := f.Stat()
	if e != nil {
		t.Fatal(e)
	}
	st, e := os.Lstat(p)
	if e != nil || !historicalFileSame(original, st) {
		t.Fatal("same original FD rejected")
	}
	if os.Chmod(p, 0640) != nil {
		t.Fatal("chmod")
	}
	st, _ = os.Lstat(p)
	if historicalFileSame(original, st) {
		t.Fatal("changed mode accepted")
	}
	if os.Chmod(p, 0600) != nil || os.Link(p, filepath.Join(dir, "alias")) != nil {
		t.Fatal("hardlink")
	}
	st, _ = f.Stat()
	if historicalFileSame(original, st) {
		t.Fatal("hardlink accepted")
	}
	if os.Remove(filepath.Join(dir, "alias")) != nil || os.Rename(p, p+".original") != nil || os.WriteFile(p, raw, 0600) != nil {
		t.Fatal("replacement")
	}
	st, _ = os.Lstat(p)
	if historicalFileSame(original, st) {
		t.Fatal("same bytes replacement accepted")
	}
	ctx, c := context.WithCancel(t.Context())
	c()
	var out bytes.Buffer
	if _, e = io.Copy(&out, historicalCancelableReader{ctx, bytes.NewReader(raw)}); e == nil || out.Len() != 0 {
		t.Fatal("cancelled read reached EOF as success")
	}
}
