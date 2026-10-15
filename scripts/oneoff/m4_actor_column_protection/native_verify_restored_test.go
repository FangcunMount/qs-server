//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// This new selector is separate from the original21; compiling it is not a
// substitute for its real MySQL/Mongo execution or CI required-selector wiring.
func TestM4ActorColumnsNativeVerifyRestored(t *testing.T) {
	t.Run("ExactOriginalReadOnly", func(t *testing.T) {
		h := newActorNative(t)
		h.richOriginalValues(0)
		before := h.originalWitness()
		h.capture()
		h.upgradePrefix(2, true)
		args := h.approvedRestoreArgs(h.restoreApproval())
		requireActorRestoreSuccess(t, h.action("restore", args...))
		h.assertOriginalWitness(before)
		files := privateDirectoryBytes(t, h.dir)
		r, _, code := actorTerminalAction(h, t.Context(), "verify-restored", args...)
		requireActorRestoredObservation(t, r, code)
		h.assertOriginalWitness(before)
		if !reflect.DeepEqual(files, privateDirectoryBytes(t, h.dir)) {
			t.Fatal("observation modified original material/authority/receipt")
		}
	})
	t.Run("ReadOnlySQLAccount", func(t *testing.T) {
		h := newActorNative(t)
		h.capture()
		h.upgradePrefix(2, true)
		args := h.approvedRestoreArgs(h.restoreApproval())
		requireActorRestoreSuccess(t, h.action("restore", args...))
		before := h.originalWitness()
		user := "m4cro_" + fmt.Sprint(time.Now().UnixNano())
		if _, e := h.root.ExecContext(t.Context(), "CREATE USER '"+user+"'@'%' IDENTIFIED BY 'owned-isolated-readonly'"); e != nil {
			t.Fatal("owned read-only SQL account unavailable")
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
			defer cancel()
			if _, e := h.root.ExecContext(ctx, "DROP USER '"+user+"'@'%'"); e != nil {
				t.Error("owned read-only SQL account cleanup failed")
			}
		}()
		if _, e := h.root.ExecContext(t.Context(), "GRANT SELECT ON `"+h.name+"`.* TO '"+user+"'@'%'"); e != nil {
			t.Fatal("owned SELECT-only grant unavailable")
		}
		direct := h.dsn
		cfg, e := mysql.ParseDSN(direct)
		if e != nil {
			t.Fatal(e)
		}
		cfg.User, cfg.Passwd = user, "owned-isolated-readonly"
		h.dsn = cfg.FormatDSN()
		r, _, exit := actorTerminalAction(h, t.Context(), "verify-restored", args...)
		h.dsn = direct
		requireActorRestoredObservation(t, r, exit)
		h.assertOriginalWitness(before)
	})
	t.Run("RetainedFactDriftRejected", func(t *testing.T) {
		h := newActorNative(t)
		h.capture()
		h.upgradePrefix(2, true)
		args := h.approvedRestoreArgs(h.restoreApproval())
		requireActorRestoreSuccess(t, h.action("restore", args...))
		if _, e := h.db.ExecContext(t.Context(), "UPDATE testee SET name='owned-drift' WHERE id=0"); e != nil {
			t.Fatal("owned retained fact change unavailable")
		}
		changed := h.originalWitness()
		r, _, exit := actorTerminalAction(h, t.Context(), "verify-restored", args...)
		if exit == 0 || r.Complete || !r.ReadOnly || r.ApprovalConsumed || r.Error != "restored_original_facts_rejected" || !r.RecoveryGateReleased || r.RecoveryGateReleaseUnproven || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed {
			t.Fatal("changed retained fact accepted", r)
		}
		h.assertOriginalWitness(changed)
	})
	t.Run("CommittedLedgerACKUnknown", func(t *testing.T) {
		h := newActorNative(t)
		h.richOriginalValues(0)
		before := h.originalWitness()
		h.capture()
		h.upgradePrefix(2, true)
		args := h.approvedRestoreArgs(h.restoreApproval())
		direct := h.dsn
		seen, undo := actorTerminalACKProxy(h, "ledger_commit")
		failed, raw, code := actorTerminalAction(h, t.Context(), "restore", args...)
		undo()
		h.dsn = direct
		if code == 0 || failed.Complete || failed.Error != "ledger_commit_outcome_unknown" || !failed.ApprovalConsumed || !seen.Load() || !failed.RecoveryGateAcquired || failed.RecoveryGateReleased || !failed.RecoveryGateReleaseUnproven || failed.ReleaseError != "recovery_gate_release_unproven" || !failed.SQLPoolClosed || !failed.MongoClosed {
			t.Fatal("actual final COMMIT ACK loss was not preserved", failed)
		}
		failedPath := filepath.Join(h.dir, "original-unknown-restore-stdout.json")
		if e := writeExclusive(failedPath, raw); e != nil {
			t.Fatal(e)
		}
		oldHash := fingerprint(raw)
		h.assertOriginalWitness(before)
		r, _, exit := actorTerminalAction(h, t.Context(), "verify-restored", args...)
		requireActorRestoredObservation(t, r, exit)
		again, e := readPrivate(failedPath, oldHash)
		if e != nil || !bytes.Equal(again, raw) {
			t.Fatal("fresh observation rewrote original unknown receipt")
		}
		h.assertOriginalWitness(before)
		replay, _, exit := actorTerminalAction(h, t.Context(), "restore", args...)
		if exit == 0 || replay.Complete || replay.ApprovalConsumed {
			t.Fatal("readonly verification revived the consumed restore")
		}
	})
	t.Run("DifferentCapturesShareSQLGate", func(t *testing.T) {
		h := newActorNative(t)
		h.capture()
		secondDir := privateRecoveryDir(t)
		secondPath := filepath.Join(secondDir, "other-capture.json")
		second := h.action("capture", "--capture-file="+secondPath)
		if !second.Complete {
			t.Fatal(second.Error)
		}
		ev, eh := freezeCaptureExecution(t, secondDir, "other", h.lastStdout, h.lastExit)
		raw, e := readPrivate(secondPath, second.MaterialHash)
		var other material
		if e != nil || strictDecode(raw, &other) != nil {
			t.Fatal("second legal capture unavailable")
		}
		h.upgradePrefix(2, true)
		// A gate held for the first capture must block a different legal directory.
		conn, e := h.db.Conn(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = conn.Close() }()
		g, e := acquireRecoveryGate(t.Context(), conn, h.binding)
		if e != nil {
			t.Fatal(e)
		}
		savedM, savedPath, savedHash, savedE, savedEH := h.original, h.materialPath, h.materialHash, h.captureEvidence, h.captureEvidenceHash
		h.original, h.materialPath, h.materialHash, h.captureEvidence, h.captureEvidenceHash = other, secondPath, second.MaterialHash, ev, eh
		args := h.approvedRestoreArgs(h.restoreApproval())
		blocked, _, code := actorTerminalAction(h, t.Context(), "restore", args...)
		if code == 0 || blocked.Complete || blocked.ApprovalConsumed || blocked.Error != "recovery_gate_busy_or_unavailable" {
			t.Fatal("different capture bypassed SQL-domain gate", blocked)
		}
		if e := g.release(); e != nil {
			t.Fatal(e)
		}
		h.original, h.materialPath, h.materialHash, h.captureEvidence, h.captureEvidenceHash = savedM, savedPath, savedHash, savedE, savedEH
		requireActorRestoreSuccess(t, h.action("restore", h.approvedRestoreArgs(h.restoreApproval())...))
	})
	t.Run("ActiveRestoreMDLBlocksVerifier", func(t *testing.T) {
		h := newActorNative(t)
		h.capture()
		h.upgradePrefix(2, true)
		args := h.approvedRestoreArgs(h.restoreApproval())
		blocker, e := h.db.BeginTx(t.Context(), nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e := blocker.ExecContext(t.Context(), "SELECT id FROM operators LIMIT 1"); e != nil {
			_ = blocker.Rollback()
			t.Fatal(e)
		}
		defer func() { _ = blocker.Rollback() }()
		type outcome struct {
			r    receipt
			code int
		}
		done := make(chan outcome, 1)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		go func() { r, _, code := actorTerminalAction(h, ctx, "restore", args...); done <- outcome{r, code} }()
		waitUntilActorMDL(t, h, ctx)
		// The original operation is already consumed, but its server session still
		// owns the gate while its actual ADD waits for another session's MDL.
		blocked, _, code := actorTerminalAction(h, t.Context(), "verify-restored", args...)
		if code == 0 || blocked.Complete || blocked.ApprovalConsumed || blocked.Error != "recovery_gate_busy_or_unavailable" {
			t.Fatal("active restore was mistaken for a finished executor", blocked)
		}
		if e := blocker.Rollback(); e != nil {
			t.Fatal(e)
		}
		select {
		case result := <-done:
			if result.code != 0 {
				t.Fatal("original restore failed after MDL released", result.r)
			}
			requireActorRestoreSuccess(t, result.r)
		case <-ctx.Done():
			t.Fatal("owned restore did not finish")
		}
		r, _, exit := actorTerminalAction(h, t.Context(), "verify-restored", args...)
		requireActorRestoredObservation(t, r, exit)
	})
	t.Run("GateOwnershipLossFailsClosed", func(t *testing.T) {
		h := newActorNative(t)
		conn, e := h.db.Conn(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = conn.Close() }()
		g, e := acquireRecoveryGate(t.Context(), conn, h.binding)
		if e != nil {
			t.Fatal(e)
		}
		var answer int
		if e := conn.QueryRowContext(t.Context(), "SELECT RELEASE_LOCK(?)", g.key).Scan(&answer); e != nil || answer != 1 {
			t.Fatal("owned server unlock unavailable")
		}
		if g.require(t.Context()) == nil || g.release() == nil || g.released || !g.releaseUnproven {
			t.Fatal("lost server ownership authorized observation")
		}
	})
	t.Run("ReleaseACKUnknownNeverComplete", func(t *testing.T) {
		h := newActorNative(t)
		h.capture()
		h.upgradePrefix(2, true)
		args := h.approvedRestoreArgs(h.restoreApproval())
		requireActorRestoreSuccess(t, h.action("restore", args...))
		before := h.originalWitness()
		direct := h.dsn
		seen, undo := actorTerminalACKProxy(h, "release_prepare")
		r, _, code := actorTerminalAction(h, t.Context(), "verify-restored", args...)
		undo()
		h.dsn = direct
		if code == 0 || r.Complete || !r.ReadOnly || r.ApprovalConsumed || !seen.Load() || !r.RecoveryGateAcquired || r.RecoveryGateReleased || !r.RecoveryGateReleaseUnproven || r.ReleaseError != "recovery_gate_release_unproven" || !r.SQLPoolClosed || !r.MongoClosed {
			t.Fatal("unknown RELEASE_LOCK response signed a completed observation", r)
		}
		h.assertOriginalWitness(before)
	})
}

func actorTerminalAction(h *actorNative, ctx context.Context, action string, extras ...string) (receipt, []byte, int) {
	args := []string{"--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=2m", "--capture-evidence-file=" + h.captureEvidence, "--capture-evidence-sha256=" + h.captureEvidenceHash}
	args = append(args, extras...)
	var stdout bytes.Buffer
	exit := serve(ctx, args, func(key string) string {
		switch key {
		case "QS_M4_C_OBJECT_MYSQL_DSN":
			return h.dsn
		case "QS_M4_C_OBJECT_MONGO_URI":
			return h.uri
		}
		return ""
	}, h.tool, &stdout)
	raw := append([]byte(nil), stdout.Bytes()...)
	var r receipt
	if strictDecode(raw, &r) != nil || (exit == 0) != r.Complete {
		h.t.Error("actual terminal CLI stdout/status inconsistent")
	}
	return r, raw, exit
}

func requireActorRestoredObservation(t *testing.T, r receipt, exit int) {
	t.Helper()
	if exit != 0 || !r.Complete || r.Action != "verify-restored" || !r.ReadOnly || r.ApprovalConsumed || r.Error != "none" || r.ReleaseError != "" || !r.SQLSnapshot || !r.MandatoryEOF || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || !r.RecoveryGateAcquired || !r.RecoveryGateReleased || r.RecoveryGateReleaseUnproven || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR {
		t.Fatal("read-only final observation or release unproved", r)
	}
}

func waitUntilActorMDL(t *testing.T, h *actorNative, ctx context.Context) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		if e := h.root.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.processlist WHERE DB=? AND STATE='Waiting for table metadata lock'", h.name).Scan(&count); e != nil {
			t.Fatal("owned MDL wait observation unavailable")
		}
		if count == 1 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("restore did not reach actual MDL wait")
		case <-ticker.C:
		}
	}
}

// Keep this fault seam independent of the original wire helper: only a final
// ledger mutation arms its following COMMIT; all original requests pass intact.
type actorTerminalLossConn struct {
	net.Conn
	mu          sync.Mutex
	mode        string
	armed, drop bool
	seen        *atomic.Bool
}

func (c *actorTerminalLossConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	if len(p) > 5 && int(p[0])|int(p[1])<<8|int(p[2])<<16 == len(p)-4 && (p[4] == 3 || p[4] == 22) {
		s := strings.ToUpper(strings.TrimSpace(string(p[5:])))
		if c.mode == "ledger_commit" && s == "DELETE FROM SCHEMA_MIGRATIONS" {
			c.armed = true
		}
		if c.mode == "ledger_commit" && c.armed && p[4] == 3 && s == "COMMIT" || c.mode == "release_prepare" && p[4] == 22 && s == "SELECT RELEASE_LOCK(?)" {
			c.drop = true
		}
	}
	c.mu.Unlock()
	return c.Conn.Write(p)
}
func (c *actorTerminalLossConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	drop := c.drop
	c.drop = false
	c.mu.Unlock()
	if !drop {
		return c.Conn.Read(p)
	}
	var header [4]byte
	if _, e := io.ReadFull(c.Conn, header[:]); e != nil {
		return 0, e
	}
	size := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if size < 1 || size > 65536 {
		return 0, fmt.Errorf("owned_terminal_ack_packet_limit")
	}
	body := make([]byte, size)
	if _, e := io.ReadFull(c.Conn, body); e != nil {
		return 0, e
	}
	if body[0] == 0 {
		c.seen.Store(true)
	}
	return 0, io.EOF
}
func actorTerminalACKProxy(h *actorNative, mode string) (*atomic.Bool, func()) {
	h.t.Helper()
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil || cfg.Net != "tcp" || cfg.TLSConfig != "" {
		h.t.Fatal("owned direct plaintext TCP required")
	}
	seen := &atomic.Bool{}
	network := h.name + "_terminal_ack"
	mysql.RegisterDialContext(network, func(ctx context.Context, addr string) (net.Conn, error) {
		conn, e := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if e != nil {
			return nil, e
		}
		return &actorTerminalLossConn{Conn: conn, mode: mode, seen: seen}, nil
	})
	cfg.Net = network
	h.dsn = cfg.FormatDSN()
	undo := func() { mysql.DeregisterDialContext(network) }
	h.t.Cleanup(undo)
	return seen, undo
}

func TestActorTerminalACKObservationIsSelective(t *testing.T) {
	for _, tc := range []struct {
		name, mode, query            string
		command                      byte
		armed, wantDrop, serverError bool
	}{
		{"ledger_commit", "ledger_commit", "COMMIT", 3, true, true, false},
		{"earlier_readonly_commit", "ledger_commit", "COMMIT", 3, false, false, false},
		{"value_batch_commit", "ledger_commit", "UPDATE `operators` SET `effective_roles`=CAST(? AS JSON)", 22, false, false, false},
		{"release_prepare", "release_prepare", "SELECT RELEASE_LOCK(?)", 22, false, true, false},
		{"server_error_not_success", "ledger_commit", "COMMIT", 3, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer func() { _ = client.Close(); _ = server.Close() }()
			if client.SetDeadline(time.Now().Add(queryLimit)) != nil || server.SetDeadline(time.Now().Add(queryLimit)) != nil {
				t.Fatal("owned bounded pipe unavailable")
			}
			seen := &atomic.Bool{}
			c := &actorTerminalLossConn{Conn: client, mode: tc.mode, armed: tc.armed, seen: seen}
			body := append([]byte{tc.command}, []byte(tc.query)...)
			request := append([]byte{byte(len(body)), byte(len(body) >> 8), byte(len(body) >> 16), 0}, body...)
			response := []byte{1, 0, 0, 1, 0}
			if tc.serverError {
				response[4] = 0xff
			}
			done := make(chan error, 1)
			go func() {
				got := make([]byte, len(request))
				if _, e := io.ReadFull(server, got); e != nil {
					done <- e
					return
				}
				if !bytes.Equal(got, request) {
					done <- fmt.Errorf("owned_request_modified")
					return
				}
				_, e := server.Write(response)
				done <- e
			}()
			if _, e := c.Write(request); e != nil {
				t.Fatal(e)
			}
			reply := make([]byte, len(response))
			_, e := io.ReadFull(c, reply)
			if tc.wantDrop {
				if e != io.EOF || seen.Load() == tc.serverError {
					t.Fatal("target ACK classification incorrect", e)
				}
			} else if e != nil || seen.Load() || !bytes.Equal(reply, response) {
				t.Fatal("nonterminal response intercepted", e)
			}
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
