//go:build reliable_messaging

package transaction

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	"github.com/stretchr/testify/require"
)

type proofIntakeGRPCClient struct {
	client pb.AssessmentIntakeServiceClient
}

func (c proofIntakeGRPCClient) EnsureAssessment(ctx context.Context, req *pb.EnsureAssessmentRequest) (*pb.EnsureAssessmentResponse, error) {
	return c.client.EnsureAssessment(ctx, req)
}

func dropFirstFINProxy(t *testing.T, ctx context.Context, target string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxyCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		downstream, e := listener.Accept()
		if e != nil {
			done <- e
			return
		}
		defer downstream.Close()
		upstream, e := net.DialTimeout("tcp", target, time.Second)
		if e != nil {
			done <- e
			return
		}
		defer upstream.Close()
		interrupt := context.AfterFunc(proxyCtx, func() { downstream.Close(); upstream.Close() })
		defer interrupt()
		copied := make(chan struct{})
		go func() { defer close(copied); _, _ = io.Copy(downstream, upstream); downstream.Close() }()
		defer func() { downstream.Close(); upstream.Close(); <-copied }()
		var magic [4]byte
		if _, e = io.ReadFull(downstream, magic[:]); e != nil {
			done <- e
			return
		}
		if _, e = upstream.Write(magic[:]); e != nil {
			done <- e
			return
		}
		reader := bufio.NewReader(downstream)
		dropped := false
		for {
			line, e := reader.ReadString('\n')
			if e != nil {
				if e == io.EOF || proxyCtx.Err() != nil {
					e = nil
				}
				done <- e
				return
			}
			if strings.HasPrefix(line, "FIN ") && !dropped {
				dropped = true
				continue
			}
			if _, e = io.WriteString(upstream, line); e != nil {
				done <- e
				return
			}
			if line == "IDENTIFY\n" || line == "AUTH\n" {
				var size [4]byte
				if _, e = io.ReadFull(reader, size[:]); e != nil {
					done <- e
					return
				}
				n := binary.BigEndian.Uint32(size[:])
				if n > 1024*1024 {
					done <- fmt.Errorf("oversized command")
					return
				}
				body := make([]byte, n)
				if _, e = io.ReadFull(reader, body); e != nil {
					done <- e
					return
				}
				if _, e = upstream.Write(append(size[:], body...)); e != nil {
					done <- e
					return
				}
			}
		}
	}()
	return listener.Addr().String(), func() {
		cancel()
		listener.Close()
		select {
		case e := <-done:
			if e != nil && !errors.Is(e, net.ErrClosed) {
				t.Errorf("FIN proxy: %v", e)
			}
		case <-time.After(5 * time.Second):
			t.Error("FIN proxy failed to drain")
		}
	}
}
