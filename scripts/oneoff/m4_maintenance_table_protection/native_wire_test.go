//go:build integration

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Only owned test TCP connections are wrapped. Requests are unchanged. A
// selected real server OK packet may be consumed without delivery to the client.
// That proves server ACK observation, not local Tx.Rollback/server abort.
type maintenanceACKConn struct {
	net.Conn
	mu                                      sync.Mutex
	step                                    string
	armed, drop, watchRollback              bool
	pending                                 []byte
	observed, rollbackObserved, prepareOnce *atomic.Bool
	onPrepare                               func() error
}

func nativeACKStep(step, query string, command byte) (arm, drop, lock bool) {
	s := strings.ToUpper(strings.TrimSpace(query))
	if command != 3 && command != 22 {
		return
	}
	for i, spec := range scope {
		if step == "create_"+string(rune('0'+i))+"_ack_loss" && command == 3 && strings.HasPrefix(s, "CREATE TABLE "+strings.ToUpper(spec.name)+" (") {
			drop = true
		}
	}
	if strings.HasPrefix(s, "INSERT INTO `TESTEE_STORE_MIGRATION_ITEMS`") {
		arm = step == "items_batch_commit_ack_loss"
		lock = step == "items_prepare_lock_marker"
	}
	if step == "ledger_commit_ack_loss" && command == 3 && s == "DELETE FROM SCHEMA_MIGRATIONS" {
		arm = true
	}
	return
}

func (c *maintenanceACKConn) Write(p []byte) (int, error) {
	if len(p) > 5 && int(p[0])|int(p[1])<<8|int(p[2])<<16 == len(p)-4 && (p[4] == 3 || p[4] == 22) {
		query := string(p[5:])
		arm, drop, lock := nativeACKStep(c.step, query, p[4])
		if lock && c.onPrepare != nil && c.prepareOnce.CompareAndSwap(false, true) {
			if c.onPrepare() != nil {
				return 0, errors.New("owned_prepare_lock_failed")
			}
		}
		c.mu.Lock()
		if arm {
			c.armed = true
		}
		if !c.observed.Load() && (drop || c.armed && p[4] == 3 && strings.EqualFold(strings.TrimSpace(query), "COMMIT")) {
			c.drop = true
		}
		if c.step == "items_prepare_lock_marker" && p[4] == 3 && strings.EqualFold(strings.TrimSpace(query), "ROLLBACK") {
			c.watchRollback = true
		}
		c.mu.Unlock()
	}
	return c.Conn.Write(p)
}

func (c *maintenanceACKConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		c.mu.Unlock()
		return n, nil
	}
	drop, watch := c.drop, c.watchRollback
	c.drop, c.watchRollback = false, false
	c.mu.Unlock()
	if !drop && !watch {
		return c.Conn.Read(p)
	}
	var header [4]byte
	if _, e := io.ReadFull(c.Conn, header[:]); e != nil {
		return 0, e
	}
	size := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if size < 1 || size > 65536 {
		return 0, errors.New("owned_ack_packet_limit")
	}
	body := make([]byte, size)
	if _, e := io.ReadFull(c.Conn, body); e != nil {
		return 0, e
	}
	if drop {
		if body[0] == 0 {
			c.observed.Store(true)
		}
		return 0, io.EOF
	}
	if body[0] == 0 {
		c.rollbackObserved.Store(true)
	}
	packet := append(header[:], body...)
	n := copy(p, packet)
	c.mu.Lock()
	c.pending = append(c.pending, packet[n:]...)
	c.mu.Unlock()
	return n, nil
}

func (h *maintenanceNative) useACKTransport(step string, onPrepare func() error) (*atomic.Bool, *atomic.Bool, func()) {
	h.t.Helper()
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil || cfg.Net != "tcp" || cfg.TLSConfig != "" {
		h.t.Fatal("owned direct unencrypted TCP fixture required for bounded ACK observation")
	}
	before := h.dsn
	observed, rollbackObserved, prepared := &atomic.Bool{}, &atomic.Bool{}, &atomic.Bool{}
	network := h.name + "_ack"
	mysql.RegisterDialContext(network, func(ctx context.Context, address string) (net.Conn, error) {
		conn, e := (&net.Dialer{}).DialContext(ctx, "tcp", address)
		if e != nil {
			return nil, e
		}
		return &maintenanceACKConn{Conn: conn, step: step, observed: observed, rollbackObserved: rollbackObserved, prepareOnce: prepared, onPrepare: onPrepare}, nil
	})
	cfg.Net = network
	h.dsn = cfg.FormatDSN()
	var once sync.Once
	undo := func() { once.Do(func() { h.dsn = before; mysql.DeregisterDialContext(network) }) }
	h.t.Cleanup(undo)
	return observed, rollbackObserved, undo
}

func TestMaintenanceRestoreWireObservationIsBoundedAndSelective(t *testing.T) {
	for _, tc := range []struct {
		name, step, query              string
		command                        byte
		prepare, serverError, rollback bool
	}{
		{"create0", "create_0_ack_loss", "CREATE TABLE testee_store_migration_reversions (id bigint)", 3, false, false, false},
		{"create4", "create_4_ack_loss", "CREATE TABLE operator_retirement_manifests (id bigint)", 3, false, false, false},
		{"batch", "items_batch_commit_ack_loss", "INSERT INTO `testee_store_migration_items` (testee_id) VALUES (?)", 22, true, false, false},
		{"ledger", "ledger_commit_ack_loss", "DELETE FROM schema_migrations", 3, true, false, false},
		{"server_error", "create_4_ack_loss", "CREATE TABLE operator_retirement_manifests (id bigint)", 3, false, true, false},
		{"actual_rollback_ACK", "items_prepare_lock_marker", "ROLLBACK", 3, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer func() { _ = client.Close(); _ = server.Close() }()
			if client.SetDeadline(time.Now().Add(5*time.Second)) != nil || server.SetDeadline(time.Now().Add(5*time.Second)) != nil {
				t.Fatal("bounded pipe deadline unavailable")
			}
			observed, rolled := &atomic.Bool{}, &atomic.Bool{}
			conn := &maintenanceACKConn{Conn: client, step: tc.step, observed: observed, rollbackObserved: rolled, prepareOnce: &atomic.Bool{}}
			packet := func(command byte, query string) []byte {
				body := append([]byte{command}, []byte(query)...)
				return append([]byte{byte(len(body)), byte(len(body) >> 8), byte(len(body) >> 16), 0}, body...)
			}
			requests := [][]byte{packet(3, "COMMIT"), packet(tc.command, tc.query)}
			if tc.prepare {
				requests = append(requests, packet(3, "COMMIT"))
			}
			done := make(chan error, 1)
			go func() {
				for n, request := range requests {
					actual := make([]byte, len(request))
					if _, e := io.ReadFull(server, actual); e != nil {
						done <- e
						return
					}
					if !bytes.Equal(actual, request) {
						done <- errors.New("owned_request_modified")
						return
					}
					response := []byte{1, 0, 0, 1, 0}
					if tc.serverError && n == len(requests)-1 {
						response[4] = 0xff
					}
					if _, e := server.Write(response); e != nil {
						done <- e
						return
					}
				}
				done <- nil
			}()
			for n, request := range requests {
				if _, e := conn.Write(request); e != nil {
					t.Fatal(e)
				}
				response := make([]byte, 5)
				_, e := io.ReadFull(conn, response)
				last := n == len(requests)-1
				if last && !tc.rollback {
					if !errors.Is(e, io.EOF) || observed.Load() == tc.serverError {
						t.Fatal("selected server OK not proved", e)
					}
				} else if e != nil || response[4] != 0 {
					t.Fatal("non-target response changed", e)
				}
			}
			if tc.rollback && (!rolled.Load() || observed.Load()) {
				t.Fatal("actual ROLLBACK OK not distinguished from dropped COMMIT")
			}
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
