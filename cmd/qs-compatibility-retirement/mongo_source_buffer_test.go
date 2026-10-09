package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func mongoSpoolDocuments(t testing.TB, count int) []bson.Raw {
	t.Helper()
	docs := make([]bson.Raw, count)
	for i := range docs {
		var id primitive.ObjectID
		binary.BigEndian.PutUint64(id[4:], uint64(i+1))
		raw, e := bson.Marshal(bson.D{{Key: "_id", Value: id}, {Key: "payload", Value: bytes.Repeat([]byte{byte(i)}, 1536)}, {Key: "status", Value: "published"}})
		if e != nil {
			t.Fatal(e)
		}
		docs[i] = raw
	}
	return docs
}

func writeMongoSpoolFrame(w io.Writer, h hash.Hash, raw bson.Raw) error {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(raw)))
	if _, e := w.Write(size[:]); e != nil {
		return e
	}
	if _, e := w.Write(raw); e != nil {
		return e
	}
	frame(h, raw, false)
	return nil
}

func openMongoSpoolFile(t testing.TB, dir string) *os.File {
	t.Helper()
	f, e := os.OpenFile(filepath.Join(dir, "source.bsonframes"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestMongoSourceBufferPreservesRawBytesAndDurablePageCheckpoints(t *testing.T) {
	// The last short logical page and the boundary at an exact 1000 rows use
	// the same byte framing, digest, BSON token and durability sequence.
	docs := mongoSpoolDocuments(t, 2001)
	dir := t.TempDir()
	f := openMongoSpoolFile(t, dir)
	buffer := bufio.NewWriterSize(f, mongoSourceBufferBytes)
	sink, e := sourceWriterTo(f, buffer, productionLimits().MaxBytes)
	if e != nil {
		t.Fatal(e)
	}
	baseline := openMongoSpoolFile(t, t.TempDir())
	baselineSink, e := sourceWriter(baseline, productionLimits().MaxBytes)
	if e != nil {
		t.Fatal(e)
	}
	h, baselineHash := sha256.New(), sha256.New()
	s := snapshot{}
	for page, start := 1, 0; start < len(docs); page, start = page+1, start+1000 {
		end := min(start+1000, len(docs))
		for _, raw := range docs[start:end] {
			if e = writeMongoSpoolFrame(sink, h, raw); e != nil {
				t.Fatal(e)
			}
			if e = writeMongoSpoolFrame(baselineSink, baselineHash, raw); e != nil {
				t.Fatal(e)
			}
			s.Records++
			s.Bytes += uint64(len(raw))
		}
		token, _, tokenErr := mongoToken(docs[end-1])
		if tokenErr != nil {
			t.Fatal(tokenErr)
		}
		if e = mongoPageCheckpoint(dir, 1, page, token, &s, h, f, buffer); e != nil {
			t.Fatal(e)
		}
		if buffer.Buffered() != 0 || s.Pages != uint64(page) {
			t.Fatal("page checkpoint preceded source flush")
		}
		actual, readErr := os.ReadFile(f.Name())
		expected, baselineErr := os.ReadFile(baseline.Name())
		if readErr != nil || baselineErr != nil || !bytes.Equal(actual, expected) || !bytes.Equal(h.Sum(nil), baselineHash.Sum(nil)) {
			t.Fatal("buffered source changed original BSON bytes or fingerprint")
		}
		checkpointPath := filepath.Join(dir, "mongodb-domain_event_outbox-pass-1-page-"+pageNumber(page)+".checkpoint.json")
		raw, readErr := os.ReadFile(checkpointPath)
		var checkpointBody struct {
			Cursor     string `json:"cursor_token"`
			Records    uint64 `json:"records"`
			Bytes      uint64 `json:"source_bytes"`
			PrefixHash string `json:"prefix_hash"`
		}
		if readErr != nil || json.Unmarshal(raw, &checkpointBody) != nil || checkpointBody.Cursor != token || checkpointBody.Records != s.Records || checkpointBody.Bytes != s.Bytes || checkpointBody.PrefixHash != hex.EncodeToString(h.Sum(nil)) {
			t.Fatal("durable checkpoint changed source identity")
		}
	}
}

func pageNumber(page int) string {
	var raw [6]byte
	for i := len(raw) - 1; i >= 0; i-- {
		raw[i] = byte('0' + page%10)
		page /= 10
	}
	return string(raw[:])
}

func assertNoMongoCheckpoint(t *testing.T, dir string, s snapshot) {
	t.Helper()
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 1 || entries[0].Name() != "source.bsonframes" || s.Pages != 0 {
		t.Fatal("failed source durability advanced a page checkpoint")
	}
}

func TestMongoSourceBufferFlushFailureDoesNotAdvanceCheckpoint(t *testing.T) {
	dir := t.TempDir()
	created := openMongoSpoolFile(t, dir)
	if e := created.Close(); e != nil {
		t.Fatal(e)
	}
	// A real read-only file descriptor accepts buffered bytes but rejects the
	// subsequent kernel write, including when this test executes as root.
	f, e := os.Open(created.Name())
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = f.Close() }()
	buffer := bufio.NewWriterSize(f, mongoSourceBufferBytes)
	sink, e := sourceWriterTo(f, buffer, productionLimits().MaxBytes)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.New()
	if e = writeMongoSpoolFrame(sink, h, mongoSpoolDocuments(t, 1)[0]); e != nil {
		t.Fatal("test did not retain bytes until actual flush")
	}
	s := snapshot{Records: 1}
	if e = mongoPageCheckpoint(dir, 1, 1, "", &s, h, f, buffer); e == nil || e.Error() != "private_output_failed" {
		t.Fatal("actual flush failure was accepted")
	}
	assertNoMongoCheckpoint(t, dir, s)
	raw, e := os.ReadFile(created.Name())
	if e != nil || len(raw) != 0 {
		t.Fatal("failed flush created source bytes")
	}
}

func TestMongoSourceBufferSyncFailureDoesNotAdvanceCheckpoint(t *testing.T) {
	dir := t.TempDir()
	f := openMongoSpoolFile(t, dir)
	buffer := bufio.NewWriterSize(f, mongoSourceBufferBytes)
	sink, e := sourceWriterTo(f, buffer, productionLimits().MaxBytes)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.New()
	if e = writeMongoSpoolFrame(sink, h, mongoSpoolDocuments(t, 1)[0]); e != nil || buffer.Flush() != nil || f.Close() != nil {
		t.Fatal("could not prepare actual sync failure")
	}
	s := snapshot{Records: 1}
	if e = mongoPageCheckpoint(dir, 1, 1, "", &s, h, f, buffer); e == nil || e.Error() != "private_output_failed" {
		t.Fatal("actual sync failure was accepted")
	}
	assertNoMongoCheckpoint(t, dir, s)
	raw, e := os.ReadFile(f.Name())
	if e != nil || len(raw) == 0 {
		t.Fatal("unknown source durability discarded actual written bytes")
	}
}

func TestMongoSourceBufferRejectsOutputBoundBeforeBufferAdmission(t *testing.T) {
	for _, size := range []int{7, 2 * mongoSourceBufferBytes} {
		dir := t.TempDir()
		f := openMongoSpoolFile(t, dir)
		buffer := bufio.NewWriterSize(f, mongoSourceBufferBytes)
		sink, e := sourceWriterTo(f, buffer, 8)
		if e != nil {
			t.Fatal(e)
		}
		if n, e := sink.Write(make([]byte, 8)); n != 8 || e != nil {
			t.Fatal("exact source frame bound changed")
		}
		if n, e := sink.Write(make([]byte, size)); n != 0 || !errors.Is(e, errSourceFileBound) || buffer.Buffered() != 8 || outputError(e).Error() != "target_output_byte_bound_exceeded" {
			t.Fatal("buffer admitted bytes beyond source frame bound")
		}
		assertNoMongoCheckpoint(t, dir, snapshot{})
		st, e := f.Stat()
		if e != nil || st.Size() != 0 {
			t.Fatal("out-of-bound write bypassed buffering")
		}
	}
}

func TestMongoSourceBufferSecondPassCheckpointNeedsNoSourceFile(t *testing.T) {
	dir := t.TempDir()
	h := sha256.New()
	s := snapshot{Records: 1000, Bytes: 10000}
	if e := mongoPageCheckpoint(dir, 2, 1, "", &s, h, nil, nil); e != nil || s.Pages != 1 {
		t.Fatal("second pass acquired a source file dependency")
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 1 || entries[0].Name() != "mongodb-domain_event_outbox-pass-2-page-000001.checkpoint.json" {
		t.Fatal("second pass created source body material")
	}
}

type countedMongoSpoolWriter struct {
	dst    io.Writer
	writes int
}

func (w *countedMongoSpoolWriter) Write(raw []byte) (int, error) {
	w.writes++
	return w.dst.Write(raw)
}

// This is a local regular-file benchmark of source syscalls and identical
// durability barriers. It measures no MongoDB command or production budget.
func BenchmarkMongoSourceSpool(b *testing.B) {
	docs := mongoSpoolDocuments(b, 2000)
	for _, buffered := range []bool{false, true} {
		name := "unbuffered"
		if buffered {
			name = "buffered"
		}
		b.Run(name, func(b *testing.B) {
			var writes int
			var sourceBytes int64
			for _, raw := range docs {
				sourceBytes += int64(len(raw) + 8)
			}
			b.SetBytes(sourceBytes)
			for iteration := 0; iteration < b.N; iteration++ {
				dir := b.TempDir()
				f := openMongoSpoolFile(b, dir)
				counter := &countedMongoSpoolWriter{dst: f}
				var buffer *bufio.Writer
				var dst io.Writer = counter
				if buffered {
					buffer = bufio.NewWriterSize(counter, mongoSourceBufferBytes)
					dst = buffer
				}
				sink, e := sourceWriterTo(f, dst, productionLimits().MaxBytes)
				if e != nil {
					b.Fatal(e)
				}
				h := sha256.New()
				s := snapshot{}
				for page, start := 1, 0; start < len(docs); page, start = page+1, start+1000 {
					for _, raw := range docs[start : start+1000] {
						if e = writeMongoSpoolFrame(sink, h, raw); e != nil {
							b.Fatal(e)
						}
						s.Records++
						s.Bytes += uint64(len(raw))
					}
					if buffered {
						e = mongoPageCheckpoint(dir, 1, page, "", &s, h, f, buffer)
					} else {
						if f.Sync() != nil {
							b.Fatal("baseline source sync failed")
						}
						s.Pages++
						e = checkpoint(dir, "mongodb-domain_event_outbox", 1, page, "", s.Records, s.Bytes, h)
					}
					if e != nil {
						b.Fatal(e)
					}
				}
				if f.Close() != nil {
					b.Fatal("source close failed")
				}
				writes += counter.writes
			}
			b.ReportMetric(float64(writes)/float64(b.N), "source-writes/op")
		})
	}
}
