package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// This is a C-local, cooperative server-session mutex. It is deliberately not
// keyed by material/operation: two captures of the same database must contend.
// GET_LOCK survives DDL implicit COMMIT. A caller may not replace it with a
// filesystem probe or infer server termination from a local Conn.Close.
type recoveryGate struct {
	conn            *sql.Conn
	key             string
	acquired        bool
	released        bool
	releaseRequired bool
	releaseUnproven bool
}

func recoveryGateKey(b binding) (string, error) {
	if !validBinding(b) {
		return "", errors.New("recovery_gate_binding_rejected")
	}
	raw, _ := json.Marshal([]string{"m4_c_sql_recovery_gate_v1", b.SQLIdentity, b.SQLName})
	return fingerprint(raw), nil // full SHA256, within MySQL's64-character limit
}

func acquireRecoveryGate(ctx context.Context, conn *sql.Conn, b binding) (*recoveryGate, error) {
	key, e := recoveryGateKey(b)
	if e != nil || conn == nil {
		return nil, errors.New("recovery_gate_binding_rejected")
	}
	g := &recoveryGate{conn: conn, key: key, releaseRequired: true, releaseUnproven: true}
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var answer sql.NullInt64
	if conn.QueryRowContext(q, "SELECT GET_LOCK(?,0)", key).Scan(&answer) != nil {
		return g, errors.New("recovery_gate_acquire_unknown")
	}
	if !answer.Valid || answer.Int64 != 1 {
		// An actual server0/NULL is a failed acquisition, not permission to write.
		g.releaseRequired, g.releaseUnproven = false, false
		return g, errors.New("recovery_gate_busy_or_unavailable")
	}
	g.acquired = true
	return g, nil
}

func (g *recoveryGate) require(ctx context.Context) error {
	if g == nil || !g.acquired || g.released {
		return errors.New("recovery_gate_not_held")
	}
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	var owned sql.NullBool
	if g.conn.QueryRowContext(q, "SELECT IS_USED_LOCK(?)=CONNECTION_ID()", g.key).Scan(&owned) != nil || !owned.Valid || !owned.Bool {
		return errors.New("recovery_gate_ownership_unknown")
	}
	return nil
}

func (g *recoveryGate) release() error {
	if g == nil || !g.releaseRequired {
		return nil
	}
	// Cleanup is separately bounded. Only actual RELEASE_LOCK=1 certifies release;
	// local pool/connection closure cannot substitute for this server response.
	q, cancel := context.WithTimeout(context.Background(), queryLimit)
	defer cancel()
	var answer sql.NullInt64
	if g.conn.QueryRowContext(q, "SELECT RELEASE_LOCK(?)", g.key).Scan(&answer) != nil || !answer.Valid || answer.Int64 != 1 {
		g.releaseUnproven = true
		return errors.New("recovery_gate_release_unproven")
	}
	g.released, g.releaseRequired, g.releaseUnproven = true, false, false
	return nil
}
