//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package process

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// m4NSQFaultProxy forwards the candidate's actual go-nsq connection to the
// invocation-owned broker. An outage closes established connections and
// refuses new ones; restoring it permits a fresh producer connection.
type m4NSQFaultProxy struct {
	listener      net.Listener
	upstream      string
	mu            sync.Mutex
	available     bool
	active        map[*m4NSQProxyPair]struct{}
	responseDelay time.Duration
}

type m4NSQProxyPair struct {
	downstream net.Conn
	upstream   net.Conn
}

func newM4NSQFaultProxy(t *testing.T, upstream string) *m4NSQFaultProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &m4NSQFaultProxy{listener: listener, upstream: upstream, available: true, active: make(map[*m4NSQProxyPair]struct{})}
	go proxy.accept()
	t.Cleanup(proxy.Close)
	return proxy
}

func (p *m4NSQFaultProxy) Address() string { return p.listener.Addr().String() }

// SetResponseDelay makes the disposable broker confirmation path slower so a
// process proof can observe fairness while a genuine pending backlog exists.
func (p *m4NSQFaultProxy) SetResponseDelay(delay time.Duration) {
	p.mu.Lock()
	p.responseDelay = delay
	p.mu.Unlock()
}

func (p *m4NSQFaultProxy) SetAvailable(available bool) {
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

func (p *m4NSQFaultProxy) Close() {
	p.SetAvailable(false)
	_ = p.listener.Close()
}

func (p *m4NSQFaultProxy) accept() {
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
		pair := &m4NSQProxyPair{downstream: downstream, upstream: upstream}
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

func (p *m4NSQFaultProxy) forward(pair *m4NSQProxyPair) {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(pair.upstream, pair.downstream)
		close(done)
	}()
	_, _ = io.Copy(&m4NSQDelayedWriter{proxy: p, target: pair.downstream}, pair.upstream)
	_ = pair.downstream.Close()
	_ = pair.upstream.Close()
	<-done
	p.mu.Lock()
	delete(p.active, pair)
	p.mu.Unlock()
}

type m4NSQDelayedWriter struct {
	proxy  *m4NSQFaultProxy
	target net.Conn
}

func (w *m4NSQDelayedWriter) Write(b []byte) (int, error) {
	w.proxy.mu.Lock()
	delay := w.proxy.responseDelay
	w.proxy.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return w.target.Write(b)
}
