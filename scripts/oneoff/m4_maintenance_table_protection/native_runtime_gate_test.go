//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func (h *maintenanceNative) runtimeArgs(mode, approvalPath, approvalHash string) []string {
	h.t.Helper()
	captured := mustTime(h.original.CapturedAt)
	now := time.Now().UTC()
	v := runtimeIntent{Format: 1, Kind: "m4_d_first_runtime_intent", Operation: "3001-1", ToolSource: h.tool, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, MaterialHash: h.materialHash, CapturePath: h.materialPath, Marker: derivedRuntimeMarker(h.original), Binding: h.binding, Mode: mode, FreezeHash: fingerprint([]byte("owned fixed writers absent")), ResponsibilityHash: fingerprint([]byte("owned synthetic objects only")), WindowStarted: captured.Add(-time.Second).Format(time.RFC3339Nano), Decision: now.Format(time.RFC3339Nano), Deadline: now.Add(5 * time.Minute).Format(time.RFC3339Nano), Expires: now.Add(10 * time.Minute).Format(time.RFC3339Nano), RuntimeFrozen: true, RuntimeNotStarted: true, AllowMarker: true, ApplyNotDispatched: mode == "cancel102"}
	if mode == "restored102" {
		raw, e := readPrivateLimit(approvalPath, approvalHash, 32768)
		var a approval
		if e != nil || strictDecode(raw, &a) != nil {
			h.t.Fatal("actual original recovery approval unavailable")
		}
		v.WindowStarted, v.Deadline, v.Expires = a.WindowStarted, a.Deadline, a.Expires
		v.FreezeHash, v.ResponsibilityHash = a.FreezeHash, a.ResponsibilityHash
	}
	raw, e := json.Marshal(v)
	p := filepath.Join(h.dir, "runtime-intent-"+mode+".json")
	if e != nil || writeExclusive(p, raw) != nil {
		h.t.Fatal("independent owned runtime intent unavailable")
	}
	args := []string{"--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash, "--runtime-intent-file=" + p, "--runtime-intent-sha256=" + fingerprint(raw)}
	if mode == "restored102" {
		args = append(args, "--approval-file="+approvalPath, "--approval-sha256="+approvalHash)
	}
	return args
}

func (h *maintenanceNative) assertRuntimeMarker(r receipt) {
	h.t.Helper()
	h.assertRestoreReleased(r)
	if !r.Complete || r.ReadOnly || !r.MarkerWritten || r.MarkerWriteUnknown || !nonzeroHash(r.MarkerHash) || !r.RuntimeGateAcquired || !r.RuntimeGateReleased || r.ApprovalConsumed || !r.DatabaseBoundaryAdvanced || r.DatabaseBoundaryAdvanceUnknown || r.DatabaseBoundary == nil || r.DatabaseBoundary.Generation != h.original.RuntimeBoundary.Generation+1 {
		h.t.Fatal("actual marker author did not close its held server gate", r)
	}
	if _, e := readPrivateLimit(derivedRuntimeMarker(h.original), r.MarkerHash, maxRuntimeIntentBytes); e != nil {
		h.t.Fatal("actual durable marker bytes not independently readable")
	}
}

// FTWRL runs only on the dedicated owned mysqld. The separate observer must
// prove a real pending metadata lock for this exact CREATE before proceeding.
func (h *maintenanceNative) blockRuntimeDDL() func() {
	h.t.Helper()
	c, e := h.db.Conn(h.t.Context())
	if e != nil {
		h.t.Fatal("owned global read-lock connection unavailable")
	}
	q, cancel := context.WithTimeout(h.t.Context(), queryLimit)
	_, e = c.ExecContext(q, "FLUSH TABLES WITH READ LOCK")
	cancel()
	if e != nil {
		_ = c.Close()
		h.t.Fatal("actual owned global read lock unavailable")
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			q, cancel := context.WithTimeout(context.Background(), queryLimit)
			defer cancel()
			_, e := c.ExecContext(q, "UNLOCK TABLES")
			ce := c.Close()
			if e != nil || ce != nil {
				h.t.Error("actual owned global read lock release unproved")
			}
		})
	}
	h.t.Cleanup(release)
	return release
}

func (h *maintenanceNative) waitRuntimeCREATE(ctx context.Context) {
	h.t.Helper()
	end := time.Now().Add(queryLimit)
	for time.Now().Before(end) && ctx.Err() == nil {
		q, cancel := context.WithTimeout(ctx, time.Second)
		var pending int
		e := h.root.QueryRowContext(q, "SELECT COUNT(DISTINCT th.PROCESSLIST_ID) FROM performance_schema.metadata_locks ml JOIN performance_schema.threads th ON th.THREAD_ID=ml.OWNER_THREAD_ID WHERE ml.LOCK_STATUS='PENDING' AND th.PROCESSLIST_DB=? AND th.PROCESSLIST_INFO LIKE 'CREATE TABLE operator_retirement_manifests (%'", h.name).Scan(&pending)
		cancel()
		if e != nil {
			h.t.Fatal("owned CREATE metadata wait observation unavailable")
		}
		if pending == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond) // polling is not the wait evidence
	}
	h.t.Fatal("actual owned CREATE pending metadata lock not proved")
}

func TestM4MaintenanceTablesNativeFirstRuntimeGate(t *testing.T) {
	t.Run("Target103Admission", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		h.upgradePrefix(len(scope), true)
		modern := h.retainedRaw()
		r := h.action("mark-first-runtime", h.runtimeArgs("target103", "", "")...)
		h.assertRuntimeMarker(r)
		h.assertSQLHead(103, false)
		h.assertRetained(modern)
	})
	t.Run("Original102Admission", func(t *testing.T) {
		for _, mode := range []string{"cancel102", "restored102"} {
			h := newMaintenanceNative(t)
			h.capture()
			before, modern := h.originalRaw(), h.retainedRaw()
			ap, ah := "", ""
			if mode == "restored102" {
				h.upgradePrefix(len(scope), true)
				args, p, hash := h.restoreInput("3002-1")
				ap, ah = p, hash
				r := h.action("restore", args...)
				h.assertRestoreReleased(r)
				if !r.Complete {
					t.Fatal("actual original restoration failed", r)
				}
			}
			h.assertRuntimeMarker(h.action("mark-first-runtime", h.runtimeArgs(mode, ap, ah)...))
			h.assertCaptureRaw(before)
			h.assertRetained(modern)
			h.assertModernReads()
		}
	})
	t.Run("CreateMDLWaitBlocksMarker", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		h.upgradePrefix(len(scope), true)
		args, ap, ah := h.restoreInput("3003-1")
		markerArgs := h.runtimeArgs("restored102", ap, ah)
		unlock := h.blockRuntimeDDL()
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		done := make(chan receipt, 1)
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			done <- h.actionContext(ctx, "restore", args...)
		}()
		defer func() {
			cancel()
			unlock()
			select {
			case <-finished:
			case <-time.After(queryLimit + time.Second):
				t.Error("owned restoration goroutine release unproved")
			}
		}()
		h.waitRuntimeCREATE(ctx)
		blocked := h.action("mark-first-runtime", markerArgs...)
		if blocked.Complete || blocked.Error != "runtime_gate_busy_or_unavailable" || blocked.MarkerWritten || blocked.MarkerWriteUnknown {
			t.Fatal("marker crossed an actual CREATE wait", blocked)
		}
		if _, e := os.Lstat(derivedRuntimeMarker(h.original)); !os.IsNotExist(e) {
			t.Fatal("busy runtime gate wrote a first marker")
		}
		unlock()
		select {
		case r := <-done:
			h.assertRestoreReleased(r)
			if !r.Complete {
				t.Fatal("CREATE/restoration after actual unlock failed", r)
			}
		case <-ctx.Done():
			t.Fatal("owned restoration child did not finish")
		}
		h.assertRuntimeMarker(h.action("mark-first-runtime", markerArgs...))
	})
	t.Run("MarkerBlocksRestoreBeforeConsume", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		h.upgradePrefix(len(scope), true)
		h.assertRuntimeMarker(h.action("mark-first-runtime", h.runtimeArgs("target103", "", "")...))
		args, _, _ := h.restoreInput("3004-1")
		r := h.action("restore", args...)
		if r.Complete || r.ApprovalConsumed || r.SQLConnectionClosed || r.RuntimeGateAcquired || r.Error != "restore_approval_rejected" {
			t.Fatal("restore crossed an actual first marker", r)
		}
		h.assertSQLHead(103, false)
	})
	t.Run("ConnectionLossKeepsServerGate", nativeRuntimeConnectionLoss)
	t.Run("ReleaseUnknownNeverAuthorizesStart", func(t *testing.T) {
		h := newMaintenanceNative(t)
		h.capture()
		args := h.runtimeArgs("cancel102", "", "")
		seen, _, undo := h.useACKTransport("runtime_release_prepare_loss", nil)
		r := h.action("mark-first-runtime", args...)
		undo()
		if r.Complete || !r.MarkerWritten || !r.RuntimeGateReleaseUnproven || r.RuntimeGateReleased || r.Error != "runtime_gate_release_unproven" || !seen.Load() {
			t.Fatal("missing RELEASE proof authorized startup or lost marker", r)
		}
		if _, e := readPrivateLimit(derivedRuntimeMarker(h.original), r.MarkerHash, maxRuntimeIntentBytes); e != nil {
			t.Fatal("failed release deleted or changed the marker")
		}
		// This is a real PREPARE OK loss, not evidence RELEASE_LOCK executed.
		t.Log("actual RELEASE_LOCK PREPARE reply lost; marker retained; no startup command, server release remains unproved")
	})
}

type nativeHeldSQLSocket struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *nativeHeldSQLSocket) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil // test transport keeps the upstream server session genuinely alive
}

// This fixture bypasses actionContext to retain the actual upstream socket.
// Keep its complete consumer input independently testable without a database.
func (h *maintenanceNative) connectionLossRestoreArgs(extras []string) []string {
	base := []string{"--action=restore", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=20s", "--capture-evidence-file=" + h.captureEvidencePath, "--capture-evidence-sha256=" + h.captureEvidenceHash}
	return append(base, extras...)
}

func TestRuntimeConnectionLossFixtureInputReachesEnvironment(t *testing.T) {
	m, _, hash, approvalPath, approvalHash := recoveryInputs(t)
	proofPath, proofHash, _ := unitCaptureEvidence(t, unitCaptureReceipt(m, hash), 0)
	h := maintenanceNative{t: t, name: m.Binding.SQLName, binding: m.Binding, captureEvidencePath: proofPath, captureEvidenceHash: proofHash}
	extras := []string{"--material-file=" + m.CapturePath, "--material-sha256=" + hash, "--approval-file=" + approvalPath, "--approval-sha256=" + approvalHash}
	reads := 0
	r := run(t.Context(), h.connectionLossRestoreArgs(extras), func(string) string {
		reads++
		return "" // stop at the ENV boundary; never open a database connection
	}, testTool)
	if reads != 2 || r.Complete || r.Error != "selected_connection_input_rejected" || r.ApprovalConsumed || r.RuntimeGateAcquired {
		t.Fatal("socket-loss fixture did not reach its intended connection boundary", reads, r)
	}
}

func nativeRuntimeConnectionLoss(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	h.upgradePrefix(len(scope), true)
	modern := h.retainedRaw()
	args, ap, ah := h.restoreInput("3005-1")
	markerArgs := h.runtimeArgs("restored102", ap, ah)
	unlock := h.blockRuntimeDDL()
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil {
		t.Fatal("owned connection unavailable")
	}
	physical := make(chan *nativeHeldSQLSocket, 1)
	network := h.name + "_held_socket"
	mysql.RegisterDialContext(network, func(ctx context.Context, address string) (net.Conn, error) {
		c, e := (&net.Dialer{}).DialContext(ctx, "tcp", address)
		if e != nil {
			return nil, e
		}
		wrapped := &nativeHeldSQLSocket{Conn: c, closed: make(chan struct{})}
		physical <- wrapped
		return wrapped, nil
	})
	defer mysql.DeregisterDialContext(network)
	cfg.Net = network
	dsn := cfg.FormatDSN()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	done := make(chan receipt, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- run(ctx, h.connectionLossRestoreArgs(args), func(key string) string {
			if key == "QS_M4_D_OBJECT_MYSQL_DSN" {
				return dsn
			}
			if key == "QS_M4_D_OBJECT_MONGO_URI" {
				return h.uri
			}
			return ""
		}, h.tool)
	}()
	var wire *nativeHeldSQLSocket
	select {
	case wire = <-physical:
	case r := <-done:
		t.Fatal("owned restore exited before its upstream connection", r.Error)
	case <-ctx.Done():
		select {
		case r := <-done:
			t.Fatal("owned restore exited before its upstream connection", r.Error)
		default:
			t.Fatal("owned upstream connection not created")
		}
	}
	defer func() { _ = wire.Conn.Close() }()
	defer func() {
		cancel()
		unlock()
		_ = wire.Conn.Close()
		select {
		case <-finished:
		case <-time.After(queryLimit + time.Second):
			t.Error("owned cancelled restoration goroutine release unproved")
		}
	}()
	h.waitRuntimeCREATE(ctx)
	cancel()
	select {
	case <-wire.closed:
	case <-time.After(queryLimit):
		t.Fatal("actual client cancellation did not close its local handle")
	}
	blocked := h.action("mark-first-runtime", markerArgs...)
	if blocked.Complete || blocked.Error != "runtime_gate_busy_or_unavailable" || blocked.MarkerWritten {
		t.Fatal("local close was mistaken for an ended server SQL session", blocked)
	}
	unlock()
	select {
	case r := <-done:
		if r.Complete || !r.RuntimeGateReleaseUnproven || r.RuntimeGateReleased || r.Error == "none" {
			t.Fatal("cancelled client upgraded unknown server release", r)
		}
	case <-time.After(queryLimit + time.Second):
		t.Fatal("owned cancelled restore did not finish after MDL unlock")
	}
	// No marker/start is attempted after the unknown result. The retained actual
	// upstream socket is closed only as owned fixture cleanup, not recovery proof.
	if e := wire.Conn.Close(); e != nil && !errors.Is(e, net.ErrClosed) {
		t.Fatal("owned upstream socket cleanup unproved")
	}
	h.assertRetained(modern)
}
