//go:build integration && (m4_07_old_chain || m4_07_new_chain)

package transaction

import (
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// m407TCPFaultProxy disconnects only clients routed through it. Tests choose
// whether those clients are NSQ producers or MySQL connections.
type m407TCPFaultProxy struct {
	listener  net.Listener
	upstream  string
	mu        sync.Mutex
	available bool
	active    map[*m407NSQProxyPair]struct{}
}

type m407NSQProxyPair struct {
	downstream net.Conn
	upstream   net.Conn
}

func newM407TCPFaultProxy(t *testing.T, upstream string) *m407TCPFaultProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &m407TCPFaultProxy{listener: listener, upstream: upstream, available: true, active: make(map[*m407NSQProxyPair]struct{})}
	go proxy.accept()
	t.Cleanup(proxy.Close)
	return proxy
}

func (p *m407TCPFaultProxy) Address() string { return p.listener.Addr().String() }

func (p *m407TCPFaultProxy) SetAvailable(available bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.available = available
	if !available {
		for pair := range p.active {
			_ = pair.downstream.Close()
			_ = pair.upstream.Close()
		}
	}
}

func (p *m407TCPFaultProxy) Close() {
	p.SetAvailable(false)
	_ = p.listener.Close()
}

func (p *m407TCPFaultProxy) accept() {
	for {
		downstream, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		available := p.available
		p.mu.Unlock()
		if !available {
			_ = downstream.Close()
			continue
		}
		upstream, err := net.DialTimeout("tcp", p.upstream, time.Second)
		if err != nil {
			_ = downstream.Close()
			continue
		}
		pair := &m407NSQProxyPair{downstream: downstream, upstream: upstream}
		p.mu.Lock()
		if !p.available {
			p.mu.Unlock()
			_ = downstream.Close()
			_ = upstream.Close()
			continue
		}
		p.active[pair] = struct{}{}
		p.mu.Unlock()
		go p.forward(pair)
	}
}

func (p *m407TCPFaultProxy) forward(pair *m407NSQProxyPair) {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(pair.upstream, pair.downstream)
		close(done)
	}()
	_, _ = io.Copy(pair.downstream, pair.upstream)
	_ = pair.downstream.Close()
	_ = pair.upstream.Close()
	<-done
	p.mu.Lock()
	delete(p.active, pair)
	p.mu.Unlock()
}

func m407NSQOutageAfter(t *testing.T, count int) int {
	t.Helper()
	raw := os.Getenv("RM_QS_M407_NSQ_OUTAGE_AFTER")
	if raw == "" || raw == "0" {
		return 0
	}
	after, err := strconv.Atoi(raw)
	if err != nil || after < 1 || after >= count {
		t.Fatalf("RM_QS_M407_NSQ_OUTAGE_AFTER must be between 1 and count-1; got %q", raw)
	}
	return after
}

func m407MySQLOutageAfter(t *testing.T, count int) int {
	t.Helper()
	raw := os.Getenv("RM_QS_M407_MYSQL_OUTAGE_AFTER")
	if raw == "" || raw == "0" {
		return 0
	}
	after, err := strconv.Atoi(raw)
	if err != nil || after < 1 || after >= count {
		t.Fatalf("RM_QS_M407_MYSQL_OUTAGE_AFTER must be between 1 and count-1; got %q", raw)
	}
	return after
}

func m407MongoOutageAfter(t *testing.T, count int) int {
	t.Helper()
	raw := os.Getenv("RM_QS_M407_MONGO_OUTAGE_AFTER")
	if raw == "" || raw == "0" {
		return 0
	}
	after, err := strconv.Atoi(raw)
	if err != nil || after < 1 || after >= count {
		t.Fatalf("RM_QS_M407_MONGO_OUTAGE_AFTER must be between 1 and count-1; got %q", raw)
	}
	return after
}
