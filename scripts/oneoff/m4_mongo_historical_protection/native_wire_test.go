//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// This bounded loopback fixture never changes a request or fabricates a server
// response. It can withhold one insert request or every actual commit reply.
// Credentials and wire payloads stay in RAM; only transaction digests/counts
// are retained in the test output.
type nativeWireProxy struct {
	listener                                                                                               net.Listener
	backend, namespace, mode                                                                               string
	mu                                                                                                     sync.Mutex
	conns                                                                                                  map[net.Conn]bool
	closed                                                                                                 bool
	wg                                                                                                     sync.WaitGroup
	withheldInserts, commitRequests, successfulCommitReplies, droppedCommitReplies, successfulAbortReplies int
	transactionDigests                                                                                     []string
}

func readNativeFrame(c net.Conn) ([]byte, error) {
	var header [16]byte
	if _, e := io.ReadFull(c, header[:]); e != nil {
		return nil, e
	}
	n := int(binary.LittleEndian.Uint32(header[:4]))
	if n < 16 || n > 1<<20 {
		return nil, errors.New("owned_wire_frame_size_rejected")
	}
	raw := make([]byte, n)
	copy(raw, header[:])
	_, e := io.ReadFull(c, raw[16:])
	return raw, e
}
func nativeFrameDocument(raw []byte) bson.Raw {
	if len(raw) < 26 || binary.LittleEndian.Uint32(raw[12:16]) != 2013 || raw[20] != 0 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(raw[21:25]))
	if n < 5 || 21+n > len(raw) {
		return nil
	}
	doc := bson.Raw(raw[21 : 21+n])
	if doc.Validate() != nil {
		return nil
	}
	return doc
}
func nativeServerOK(raw []byte) bool {
	d := nativeFrameDocument(raw)
	v := d.Lookup("ok")
	if n, ok := v.DoubleOK(); ok {
		return n == 1
	}
	if n, ok := v.Int32OK(); ok {
		return n == 1
	}
	if n, ok := v.Int64OK(); ok {
		return n == 1
	}
	return false
}
func nativeDatabaseName(doc bson.Raw) string {
	name, _ := doc.Lookup("$db").StringValueOK()
	return name
}

func nativeTransactionDigest(doc bson.Raw) string {
	lsid, ok := doc.Lookup("lsid").DocumentOK()
	n, nok := doc.Lookup("txnNumber").Int64OK()
	if !ok || !nok {
		return ""
	}
	raw := append([]byte(nil), lsid...)
	var v [8]byte
	binary.LittleEndian.PutUint64(v[:], uint64(n))
	return fingerprint(append(raw, v[:]...))
}
func newNativeWireProxy(t *testing.T, h *nativeFixture, mode string) *nativeWireProxy {
	t.Helper()
	u, e := url.Parse(h.uri)
	if e != nil || u.Query().Get("directConnection") != "true" || u.Query().Get("compressors") != "" {
		t.Fatal("fixed uncompressed direct owned fixture route required")
	}
	host, _, e := net.SplitHostPort(u.Host)
	if e != nil || (host != "127.0.0.1" && host != "localhost") {
		t.Fatal("wire fixture backend must be owned loopback")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := &nativeWireProxy{listener: listener, backend: u.Host, namespace: h.name, mode: mode, conns: map[net.Conn]bool{}}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			front, e := listener.Accept()
			if e != nil {
				return
			}
			q, c := context.WithTimeout(context.Background(), queryLimit)
			back, e := (&net.Dialer{}).DialContext(q, "tcp", p.backend)
			c()
			if e != nil {
				_ = front.Close()
				continue
			}
			_ = front.SetDeadline(time.Now().Add(2 * time.Minute))
			_ = back.SetDeadline(time.Now().Add(2 * time.Minute))
			p.mu.Lock()
			if p.closed {
				p.mu.Unlock()
				_ = front.Close()
				_ = back.Close()
				return
			}
			p.conns[front] = true
			p.conns[back] = true
			p.mu.Unlock()
			p.bridge(front, back)
		}
	}()
	u.Host = listener.Addr().String()
	h.uri = u.String()
	t.Cleanup(func() {
		if !p.close() {
			t.Error("owned wire listener/connections/goroutines release unproven")
		}
	})
	return p
}
func (p *nativeWireProxy) bridge(front, back net.Conn) {
	var once sync.Once
	finish := func() {
		once.Do(func() {
			_ = front.Close()
			_ = back.Close()
			p.mu.Lock()
			delete(p.conns, front)
			delete(p.conns, back)
			p.mu.Unlock()
		})
	}
	pending := map[uint32]string{}
	var pendingMu sync.Mutex
	p.wg.Add(2)
	go func() {
		defer p.wg.Done()
		defer finish()
		for {
			frame, e := readNativeFrame(front)
			if e != nil {
				return
			}
			doc := nativeFrameDocument(frame)
			kind := ""
			if len(doc) > 0 {
				txn := nativeTransactionDigest(doc)
				if doc.Lookup("commitTransaction").Type != 0 && txn != "" {
					kind = "commit"
					p.mu.Lock()
					p.commitRequests++
					p.transactionDigests = append(p.transactionDigests, txn)
					p.mu.Unlock()
				}
				if doc.Lookup("abortTransaction").Type != 0 && txn != "" {
					kind = "abort"
				}
				if name, ok := doc.Lookup("insert").StringValueOK(); ok && name == "schema_migrations" && txn != "" && nativeDatabaseName(doc) == p.namespace && p.mode == "insert_request_loss" {
					p.mu.Lock()
					p.withheldInserts++
					p.mu.Unlock()
					return
				}
			}
			if kind != "" {
				pendingMu.Lock()
				pending[binary.LittleEndian.Uint32(frame[4:8])] = kind
				pendingMu.Unlock()
			}
			if _, e := io.Copy(back, bytes.NewReader(frame)); e != nil {
				return
			}
		}
	}()
	go func() {
		defer p.wg.Done()
		defer finish()
		for {
			frame, e := readNativeFrame(back)
			if e != nil {
				return
			}
			responseTo := binary.LittleEndian.Uint32(frame[8:12])
			pendingMu.Lock()
			kind := pending[responseTo]
			delete(pending, responseTo)
			pendingMu.Unlock()
			if kind == "commit" {
				p.mu.Lock()
				if nativeServerOK(frame) {
					p.successfulCommitReplies++
				}
				drop := p.mode == "commit_reply_loss"
				if drop {
					p.droppedCommitReplies++
				}
				p.mu.Unlock()
				if drop {
					return
				}
			}
			if kind == "abort" && nativeServerOK(frame) {
				p.mu.Lock()
				p.successfulAbortReplies++
				p.mu.Unlock()
			}
			if _, e := io.Copy(front, bytes.NewReader(frame)); e != nil {
				return
			}
		}
	}()
}
func (p *nativeWireProxy) close() bool {
	p.mu.Lock()
	p.closed = true
	for c := range p.conns {
		_ = c.Close()
	}
	p.mu.Unlock()
	_ = p.listener.Close()
	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()
	select {
	case <-done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.conns) == 0
	case <-time.After(queryLimit):
		return false
	}
}
func TestM4MongoObjectNativeInterruptedLedgerTransaction(t *testing.T) {
	h := newNative(t)
	h.capture()
	h.upgrade(2, false)
	docs := h.frozenDocuments()
	metadata := h.retainedMetadata()
	head, uuid := h.head()
	sql := h.sqlLedger()
	args, _, _ := h.approval()
	p := newNativeWireProxy(t, h, "insert_request_loss")
	r := h.action("restore", args...)
	requirePhysicalReleased(t, r)
	if r.Complete || r.Error != "original_ledger_bson_insert_failed" || r.LedgerCommitIssued || !r.LedgerAbortCalled || !r.MongoTransactionReleaseUnproven {
		t.Fatal("interrupted transaction improperly certified", r)
	}
	after, afterUUID := h.head()
	if !bytes.Equal(head, after) || !bytes.Equal(uuid, afterUUID) || h.sqlLedger() != sql {
		t.Fatal("failed transaction exposed partial ledger replay")
	}
	assertDocuments(t, docs, h.frozenDocuments())
	assertMetadata(t, metadata, h.retainedMetadata())
	h.assertOriginalTargetSpecs()
	p.mu.Lock()
	withheld, aborts, commits := p.withheldInserts, p.successfulAbortReplies, p.commitRequests
	p.mu.Unlock()
	if withheld != 1 || aborts < 1 || commits != 0 {
		t.Fatal("actual withheld insert/server abort was not observed", withheld, aborts, commits)
	}
	t.Log("actual transaction insert request withheld after DELETE; upstream abort acknowledged; public release remains unproven rather than inferred from SDK nil")
}
func TestM4MongoObjectNativeCommitResponseLossUnknown(t *testing.T) {
	h := newNative(t)
	h.capture()
	h.upgrade(2, false)
	docs := h.frozenDocuments()
	metadata := h.retainedMetadata()
	_, uuid := h.head()
	sql := h.sqlLedger()
	args, _, _ := h.approval()
	p := newNativeWireProxy(t, h, "commit_reply_loss")
	r := h.action("restore", args...)
	requirePhysicalReleased(t, r)
	if r.Complete || r.Error != "ledger_commit_failed_or_unknown" || !r.LedgerCommitIssued || r.LedgerCommitAcknowledged || !r.LedgerCommitOutcomeUnknown || !r.MongoTransactionReleaseUnproven {
		t.Fatal("committed reply loss did not remain unknown", r)
	}
	after, afterUUID := h.head()
	if !bytes.Equal(after, h.original.Ledger.Document) || !bytes.Equal(uuid, afterUUID) || h.sqlLedger() != sql {
		t.Fatal("actual committed original BSON/provider40 UUID missing")
	}
	assertDocuments(t, docs, h.frozenDocuments())
	assertMetadata(t, metadata, h.retainedMetadata())
	h.assertOriginalTargetSpecs()
	p.mu.Lock()
	requests, acks, dropped := p.commitRequests, p.successfulCommitReplies, p.droppedCommitReplies
	digests := append([]string(nil), p.transactionDigests...)
	p.mu.Unlock()
	if requests < 1 || requests > 2 || acks != requests || dropped != requests || len(digests) != requests {
		t.Fatal("actual upstream success/all reply loss not observed", requests, acks, dropped)
	}
	for _, digest := range digests {
		if digest == "" || digest != digests[0] {
			t.Fatal("driver changed lsid/txnNumber or reran a new transaction")
		}
	}
	t.Logf("upstream successful COMMIT replies=%d; every reply dropped; same lsid/txnNumber digest=%s; one application restore/commit call, no transaction rerun", acks, digests[0])
}

func TestNativeWireMessageBoundsAndTransactionDigest(t *testing.T) {
	raw, _ := bson.Marshal(bson.D{{Key: "commitTransaction", Value: int32(1)}, {Key: "lsid", Value: bson.D{{Key: "id", Value: bson.RawValue{Type: 5, Value: append([]byte{16, 0, 0, 0, 4}, bytes.Repeat([]byte{7}, 16)...)}}}}, {Key: "txnNumber", Value: int64(17)}, {Key: "autocommit", Value: false}})
	frame := make([]byte, 21+len(raw))
	binary.LittleEndian.PutUint32(frame[:4], uint32(len(frame)))
	binary.LittleEndian.PutUint32(frame[12:16], 2013)
	copy(frame[21:], raw)
	doc := nativeFrameDocument(frame)
	if !bytes.Equal(doc, raw) || nativeTransactionDigest(doc) == "" {
		t.Fatal("wire fixture rewrote complete command/session BSON")
	}
	changed := append([]byte(nil), frame...)
	binary.LittleEndian.PutUint32(changed[12:16], 2012)
	if nativeFrameDocument(changed) != nil || nativeFrameDocument(frame[:len(frame)-1]) != nil {
		t.Fatal("unsupported compressed/truncated wire frame accepted")
	}
	front, back := net.Pipe()
	defer func() { _ = front.Close(); _ = back.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		header := make([]byte, 16)
		binary.LittleEndian.PutUint32(header[:4], (1<<20)+1)
		_, _ = back.Write(header)
	}()
	if _, e := readNativeFrame(front); e == nil {
		t.Fatal("oversized wire response allocated/read")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("in-memory wire helper did not release")
	}
}
