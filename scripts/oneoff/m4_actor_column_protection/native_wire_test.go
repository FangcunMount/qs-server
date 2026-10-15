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

// Only these isolated tests wrap the owned direct TCP connection. All requests
// are forwarded unmodified; a selected actual server OK packet is then dropped.
// No SQL body, account, password or row value is emitted or retained.
type actorACKLossConn struct {
	net.Conn
	mu       sync.Mutex
	step     string
	armed    bool
	drop     bool
	observed *atomic.Bool
}

func (c *actorACKLossConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	if len(p) > 5 && int(p[0])|int(p[1])<<8|int(p[2])<<16 == len(p)-4 {
		// COM_QUERY or COM_STMT_PREPARE: preparing a value batch arms only
		// its following COMMIT, never a read-only snapshot's earlier COMMIT.
		if p[4] == 3 || p[4] == 22 {
			s := strings.ToUpper(strings.TrimSpace(string(p[5:])))
			if c.step == "first_roles_batch_commit_ack" && strings.HasPrefix(s, "UPDATE `OPERATORS` SET `EFFECTIVE_ROLES`=") {
				c.armed = true
			}
			if p[4] == 3 && ((c.step == "roles_add_ack" && strings.HasPrefix(s, "ALTER TABLE `OPERATORS` ADD COLUMN `EFFECTIVE_ROLES`")) || (c.step == "tags_add_ack" && strings.HasPrefix(s, "ALTER TABLE `TESTEE` ADD COLUMN `TAGS`")) || (c.armed && s == "COMMIT")) {
				c.drop = true
			}
		}
	}
	c.mu.Unlock()
	return c.Conn.Write(p)
}

func (c *actorACKLossConn) Read(p []byte) (int, error) {
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
		return 0, errors.New("owned_ack_packet_limit")
	}
	body := make([]byte, size)
	if _, e := io.ReadFull(c.Conn, body); e != nil {
		return 0, e
	}
	if body[0] == 0 {
		c.observed.Store(true)
	}
	return 0, io.EOF
}

func (h *actorNative) dropRealRestoreACK(step string) (*atomic.Bool, func()) {
	h.t.Helper()
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil || cfg.Net != "tcp" || cfg.TLSConfig != "" {
		h.t.Fatal("owned direct unencrypted TCP fixture required for bounded ACK observation")
	}
	observed := &atomic.Bool{}
	network := h.name + "_ack"
	mysql.RegisterDialContext(network, func(ctx context.Context, address string) (net.Conn, error) {
		c, e := (&net.Dialer{}).DialContext(ctx, "tcp", address)
		if e != nil {
			return nil, e
		}
		return &actorACKLossConn{Conn: c, step: step, observed: observed}, nil
	})
	cfg.Net = network
	h.dsn = cfg.FormatDSN()
	undo := func() { mysql.DeregisterDialContext(network) }
	h.t.Cleanup(undo)
	return observed, undo
}

func TestActorRestoreACKWireObservationIsBoundedAndSelective(t *testing.T) {
	for _, tc := range []struct {
		name, step, query string
		command           byte
		serverError       bool
	}{
		{"roles_add", "roles_add_ack", "ALTER TABLE `operators` ADD COLUMN `effective_roles` JSON NULL AFTER `roles`", 3, false},
		{"tags_add", "tags_add_ack", "ALTER TABLE `testee` ADD COLUMN `tags` JSON DEFAULT NULL", 3, false},
		{"batch_commit", "first_roles_batch_commit_ack", "UPDATE `operators` SET `effective_roles`=CASE WHEN id=? THEN ? END", 22, false},
		{"server_error_is_not_ack", "roles_add_ack", "ALTER TABLE `operators` ADD COLUMN `effective_roles` JSON NULL AFTER `roles`", 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer func() { _ = client.Close(); _ = server.Close() }()
			if client.SetDeadline(time.Now().Add(5*time.Second)) != nil || server.SetDeadline(time.Now().Add(5*time.Second)) != nil {
				t.Fatal("owned pipe deadline unavailable")
			}
			observed := &atomic.Bool{}
			conn := &actorACKLossConn{Conn: client, step: tc.step, observed: observed}
			packet := func(command byte, query string) []byte {
				body := append([]byte{command}, []byte(query)...)
				return append([]byte{byte(len(body)), byte(len(body) >> 8), byte(len(body) >> 16), 0}, body...)
			}
			requests := [][]byte{packet(3, "COMMIT"), packet(tc.command, tc.query)}
			if tc.command == 22 {
				requests = append(requests, packet(3, "COMMIT"))
			}
			// A COMMIT preceding any target mutation belongs to a read-only
			// snapshot and must pass through. A prepare response also passes.
			response := []byte{1, 0, 0, 1, 0}
			done := make(chan error, 1)
			go func() {
				for n, expected := range requests {
					actual := make([]byte, len(expected))
					if _, e := io.ReadFull(server, actual); e != nil {
						done <- e
						return
					}
					if !bytes.Equal(actual, expected) {
						done <- errors.New("owned_request_was_modified")
						return
					}
					ack := append([]byte(nil), response...)
					if tc.serverError && n == len(requests)-1 {
						ack[4] = 0xff
					}
					if _, e := server.Write(ack); e != nil {
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
				reply := make([]byte, len(response))
				_, e := io.ReadFull(conn, reply)
				if n == len(requests)-1 {
					if !errors.Is(e, io.EOF) || observed.Load() == tc.serverError {
						t.Fatal("target ACK missing or server error misclassified", e)
					}
				} else if e != nil || !bytes.Equal(reply, response) || observed.Load() {
					t.Fatal("non-target response was intercepted", e)
				}
			}
			if e := <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
