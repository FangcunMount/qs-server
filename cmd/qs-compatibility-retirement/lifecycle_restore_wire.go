package main

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Fixed helper executed from the approved, read-only binary mount in the
// fresh network-none engine. Only actual local native wire bytes cross stdio.
// No JSON receipt, exported proof or imported readiness bit is involved.
func runLifecycleRestoreWire(kind string) error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return lifecycleError("restore_wire_root_required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var network, address string
	switch kind {
	case "mysql":
		network, address = "unix", "/var/run/mysqld/mysqld.sock"
	case "mongodb":
		network, address = "tcp", "127.0.0.1:27017"
	default:
		return lifecycleError("restore_wire_kind_rejected")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return lifecycleError("restore_wire_local_engine_unavailable")
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(600 * time.Second))
	go func() { _, _ = io.Copy(conn, os.Stdin); _ = conn.Close() }()
	_, err = io.Copy(os.Stdout, conn)
	_ = conn.Close()
	if err != nil {
		return lifecycleError("restore_wire_stream_failed")
	}
	return nil
}

type restoreWireAddress string

func (a restoreWireAddress) Network() string { return "docker-exec-native" }
func (a restoreWireAddress) String() string  { return string(a) }

// Each driver's net.Conn is one real Docker exec process and one real engine
// socket. Closing/reaching a deadline cancels that process, closes both pipes,
// and reaps it. Driver socket deadlines can only shorten the live I/O lifetime.
type lifecycleWireConn struct {
	active                      sync.WaitGroup
	closed                      bool
	read                        io.ReadCloser
	write                       io.WriteCloser
	command                     *exec.Cmd
	cancel                      context.CancelFunc
	done                        chan struct{}
	once                        sync.Once
	mu                          sync.Mutex
	readDeadline, writeDeadline time.Time
	readChanged, writeChanged   chan struct{}
	readMu, writeMu             sync.Mutex
	expired                     bool
}

func openLifecycleWireConn(ctx context.Context, docker, cid, kind string) (*lifecycleWireConn, error) {
	if ctx == nil || ctx.Err() != nil || !hashRE.MatchString(cid) || (kind != "mysql" && kind != "mongodb") {
		return nil, lifecycleError("restore_wire_binding_rejected")
	}
	child, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(child, docker, "--host", "unix:///run/docker.sock", "exec", "--interactive", cid, "/tool/restore-native", "--restore-wire", kind)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	// Server/exec diagnostics may contain original namespace names; discard them.
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, lifecycleError("restore_wire_pipe_failed")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		_ = in.Close()
		return nil, lifecycleError("restore_wire_pipe_failed")
	}
	if cmd.Start() != nil {
		cancel()
		_ = in.Close()
		_ = out.Close()
		return nil, lifecycleError("restore_wire_exec_failed")
	}
	c := &lifecycleWireConn{read: out, write: in, command: cmd, cancel: cancel, done: make(chan struct{}), readChanged: make(chan struct{}), writeChanged: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(c.done) }()
	go func() {
		select {
		case <-child.Done():
			_ = c.Close()
		case <-c.done:
		}
	}()
	return c, nil
}

// Pipe streams cannot set kernel socket deadlines. Each active I/O waits on
// its own direction's deadline/change notification. A timeout closes/reaps the
// real exec and joins the blocked I/O before returning the caller's buffer.
// An expired idle write deadline never interrupts an active read (and vice versa).
type lifecycleWireIO struct {
	n   int
	err error
}

func (c *lifecycleWireConn) transfer(read bool, p []byte) (int, error) {
	lock := &c.writeMu
	if read {
		lock = &c.readMu
	}
	lock.Lock()
	defer lock.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, net.ErrClosed
	}
	c.active.Add(1)
	deadline := c.writeDeadline
	if read {
		deadline = c.readDeadline
	}
	c.mu.Unlock()
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		c.active.Done()
		c.mu.Lock()
		c.expired = true
		c.mu.Unlock()
		_ = c.Close()
		return 0, c.deadlineError(os.ErrDeadlineExceeded)
	}
	completed := make(chan lifecycleWireIO, 1)
	go func() {
		defer c.active.Done()
		var n int
		var e error
		if read {
			n, e = c.read.Read(p)
		} else {
			n, e = c.write.Write(p)
		}
		completed <- lifecycleWireIO{n, e}
	}()
	for {
		c.mu.Lock()
		deadline, changed := c.writeDeadline, c.writeChanged
		if read {
			deadline, changed = c.readDeadline, c.readChanged
		}
		c.mu.Unlock()
		var timer *time.Timer
		var timeout <-chan time.Time
		if !deadline.IsZero() {
			timer = time.NewTimer(time.Until(deadline))
			timeout = timer.C
		}
		select {
		case result := <-completed:
			if timer != nil {
				timer.Stop()
			}
			return result.n, c.deadlineError(result.err)
		case <-changed:
			if timer != nil {
				timer.Stop()
			}
		case <-timeout:
			c.mu.Lock()
			c.expired = true
			c.mu.Unlock()
			_ = c.Close()
			result := <-completed
			return result.n, c.deadlineError(os.ErrDeadlineExceeded)
		}
	}
}
func (c *lifecycleWireConn) Read(p []byte) (int, error)  { return c.transfer(true, p) }
func (c *lifecycleWireConn) Write(p []byte) (int, error) { return c.transfer(false, p) }
func (c *lifecycleWireConn) deadlineError(e error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e != nil && c.expired {
		return &net.OpError{Op: "native-wire", Net: "docker-exec", Err: os.ErrDeadlineExceeded}
	}
	return e
}
func (c *lifecycleWireConn) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		c.cancel()
		_ = c.write.Close()
		_ = c.read.Close()
		c.active.Wait()
	})
	<-c.done
	return nil
}
func (c *lifecycleWireConn) LocalAddr() net.Addr { return restoreWireAddress("approved-host") }
func (c *lifecycleWireConn) RemoteAddr() net.Addr {
	return restoreWireAddress("owned-engine-local-socket")
}
func (c *lifecycleWireConn) SetDeadline(t time.Time) error      { return c.setDeadline(&t, &t) }
func (c *lifecycleWireConn) SetReadDeadline(t time.Time) error  { return c.setDeadline(&t, nil) }
func (c *lifecycleWireConn) SetWriteDeadline(t time.Time) error { return c.setDeadline(nil, &t) }
func (c *lifecycleWireConn) setDeadline(r, w *time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expired {
		return os.ErrDeadlineExceeded
	}
	if r != nil {
		c.readDeadline = *r
		close(c.readChanged)
		c.readChanged = make(chan struct{})
	}
	if w != nil {
		c.writeDeadline = *w
		close(c.writeChanged)
		c.writeChanged = make(chan struct{})
	}
	return nil
}

var _ net.Conn = (*lifecycleWireConn)(nil)
