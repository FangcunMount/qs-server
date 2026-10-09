package compatibilityretirementbackup

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
)

// These values come only from the already verified physical Archive. They are
// read constraints, never historical closure, writer-fence or DROP authority.
type HostHistoricalBinding struct {
	SourceSHA, OperationID, SQLIdentitySHA256, MongoIdentitySHA256 string
	SQLHead, MongoHead                                             uint64
}

// HostHistoricalSources owns only the four exact registered file descriptors.
// Each Copies call lends independent readers; it owns no database or epoch.
type HostHistoricalSources struct {
	self     *HostHistoricalSources
	archive  *Archive
	expected Approval
	files    [4]*os.File
	closed   bool
}

func (*HostHistoricalSources) MarshalJSON() ([]byte, error) { return nil, ErrSerialization }
func (*HostHistoricalSources) String() string {
	return "opaque temporary archive historical readers; no closure authority"
}

func OpenHostHistoricalSources(ctx context.Context, a *Archive, expected Approval) (out *HostHistoricalSources, result error) {
	if e := VerifyHostArchiveBinding(ctx, a, expected); e != nil {
		return nil, e
	}
	out = &HostHistoricalSources{archive: a, expected: expected}
	out.self = out
	defer func() {
		if result != nil {
			_ = out.Close()
			out = nil
		}
	}()
	for i, asset := range a.data.Assets {
		out.files[i], result = openPrivateFile(filepath.Join(a.dir, asset.Filename))
		if result != nil {
			return out, result
		}
	}
	result = out.Verify(ctx)
	return out, result
}

func (s *HostHistoricalSources) Verify(ctx context.Context) error {
	if s == nil || s.self != s || s.closed || s.archive == nil {
		return ErrPrivate
	}
	if e := VerifyHostArchiveBinding(ctx, s.archive, s.expected); e != nil {
		return e
	}
	for i, f := range s.files {
		if f == nil {
			return ErrPrivate
		}
		before, e := f.Stat()
		named, ne := os.Lstat(filepath.Join(s.archive.dir, sourceNames[i]))
		asset := s.archive.data.Assets[i]
		if e != nil || ne != nil || !historicalFileSame(before, named) || before.Size() != asset.Bytes || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || before.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
			return ErrPrivate
		}
		h := newCountHash()
		_, e = io.Copy(h, historicalContextReader{ctx, io.NewSectionReader(f, 0, asset.Bytes+1)})
		after, ae := f.Stat()
		live, le := os.Lstat(filepath.Join(s.archive.dir, sourceNames[i]))
		if e != nil || ae != nil || le != nil || ctx.Err() != nil || !historicalFileSame(before, after) || !historicalFileSame(before, live) || h.count != asset.Bytes || h.digest() != asset.SHA256 {
			return ErrSource
		}
	}
	return nil
}

func historicalFileSame(a, b os.FileInfo) bool {
	if a == nil || b == nil || !os.SameFile(a, b) || a.Mode() != b.Mode() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && x.Uid == y.Uid && x.Gid == y.Gid && x.Nlink == 1 && y.Nlink == 1
}

type historicalContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r historicalContextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.reader.Read(p)
}

func (s *HostHistoricalSources) Binding(ctx context.Context) (HostHistoricalBinding, error) {
	if s == nil || s.self != s || s.closed || ctx == nil || ctx.Err() != nil {
		return HostHistoricalBinding{}, ErrPrivate
	}
	b := s.archive.data.Inventory.Bindings
	return HostHistoricalBinding{s.expected.SourceSHA, s.expected.OperationID, b["mysql"].IdentityHash, b["mongodb"].IdentityHash, b["mysql"].Version, b["mongodb"].Version}, nil
}

func (s *HostHistoricalSources) Copies(ctx context.Context) ([]retirement.SourceCopyInput, error) {
	if s == nil || s.self != s || s.closed || ctx == nil || ctx.Err() != nil {
		return nil, ErrPrivate
	}
	out := make([]retirement.SourceCopyInput, 4)
	for i, f := range s.files {
		v := s.archive.data.Inventory.Targets[i]
		out[i] = retirement.SourceCopyInput{Input: io.NewSectionReader(f, 0, s.archive.data.Assets[i].Bytes+1), Expected: retirement.SourceCopyExpectation{Boundary: v.Boundary, DataHash: v.DataHash, Records: v.Records, Bytes: v.Bytes}}
	}
	return out, nil
}

func (s *HostHistoricalSources) Close() error {
	if s == nil {
		return nil
	}
	if s.self != s {
		return ErrPrivate
	}
	if s.closed {
		return nil
	}
	s.closed = true
	var result error
	for i, f := range s.files {
		if f != nil && f.Close() != nil {
			result = ErrPrivate
		}
		s.files[i] = nil
	}
	return result // Closing never deletes bodies or performs database recovery.
}
