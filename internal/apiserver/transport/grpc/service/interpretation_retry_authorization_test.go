package service

import (
	"context"
	"net"
	"sync"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

func TestInterpretationAutomationGRPCPreservesGovernedRetryAuthorization(t *testing.T) {
	probe := &retryAuthorizationProbe{}
	server := grpc.NewServer()
	pb.RegisterInterpretationAutomationServiceServer(server, NewInterpretationAutomationService(probe))
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := pb.NewInterpretationAutomationServiceClient(conn)
	outcomeID := meta.New()
	cases := []struct {
		name string
		md   metadata.MD
		want retrygovernance.Authorization
		ok   bool
	}{
		{
			name: "manual",
			md: metadata.Pairs("x-retry-event-id", "manual-event", "x-retry-expected-attempt", "1",
				"x-retry-origin", "manual", "x-retry-action-request-id", "manual-action", "x-retry-mode", "normal"),
			want: retrygovernance.Authorization{EventID: "manual-event", ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginManual, ActionRequestID: "manual-action", Mode: "normal"}, ok: true,
		},
		{
			name: "force",
			md: metadata.Pairs("x-retry-event-id", "force-event", "x-retry-expected-attempt", "2",
				"x-retry-origin", "force", "x-retry-action-request-id", "force-action", "x-retry-mode", "force"),
			want: retrygovernance.Authorization{EventID: "force-event", ExpectedAttempt: 2, Origin: retrygovernance.AttemptOriginForce, ActionRequestID: "force-action", Mode: "force"}, ok: true,
		},
		{
			name: "invalid attempt",
			md:   metadata.Pairs("x-retry-event-id", "bad-event", "x-retry-expected-attempt", "zero", "x-retry-origin", "manual"),
		},
		{
			name: "invalid origin",
			md:   metadata.Pairs("x-retry-event-id", "bad-event", "x-retry-expected-attempt", "1", "x-retry-origin", "unknown"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := metadata.NewOutgoingContext(t.Context(), tc.md)
			response, err := client.GenerateReportFromOutcome(ctx, &pb.GenerateReportFromOutcomeRequest{OutcomeId: outcomeID.String()})
			if err != nil || response == nil || !response.Success {
				t.Fatalf("gRPC response=%+v err=%v", response, err)
			}
			got, ok := probe.last()
			if ok != tc.ok || (ok && got != tc.want) {
				t.Fatalf("parsed authorization=%+v present=%t, want %+v present=%t", got, ok, tc.want, tc.ok)
			}
		})
	}
}

type retryAuthorizationProbe struct {
	mu                sync.Mutex
	lastAuthorization retrygovernance.Authorization
	present           bool
}

func (p *retryAuthorizationProbe) Generate(ctx context.Context, _ automation.GenerateCommand) (*automation.Result, error) {
	authorization, ok := retrygovernance.AuthorizationFromContext(ctx)
	p.mu.Lock()
	p.lastAuthorization, p.present = authorization, ok
	p.mu.Unlock()
	return &automation.Result{Status: automation.StatusProcessing}, nil
}

func (p *retryAuthorizationProbe) last() (retrygovernance.Authorization, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastAuthorization, p.present
}
