// Test-only process: actual QS handler/use case, synthetic relationships on disk.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type facts struct{ state string }

func (f facts) GetTestee(_ context.Context, id uint64) (*actorreadmodel.TesteeRow, error) {
	if id != 7 {
		return nil, nil
	}
	profile := uint64(9)
	return &actorreadmodel.TesteeRow{ID: 7, OrgID: 1, ProfileID: &profile}, nil
}
func (f facts) HasActiveProfileLink(_ context.Context, user, profile string) (bool, error) {
	state, err := os.ReadFile(f.state)
	if err != nil {
		return false, err
	}
	return string(state) == "allowed" && user == "parent" && profile == "9", nil
}
func (f facts) AuthorizeAssessment(_ context.Context, actor evaluation.Actor, id uint64) error {
	if actor.TesteeID != 7 || id != 42 {
		return app.ErrAccessDenied
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 5 {
		return fmt.Errorf("expected CA, certificate, key, state file")
	}
	pair, err := tls.LoadX509KeyPair(os.Args[2], os.Args[3])
	if err != nil {
		return err
	}
	ca, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return fmt.Errorf("invalid CA")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert})))
	defer server.Stop()
	f := facts{os.Args[4]}
	(&service.AIWorkflowAccessService{Access: &app.CurrentAccess{Testees: f, Links: f, Assessments: f}}).RegisterService(server)
	fmt.Println(listener.Addr().(*net.TCPAddr).Port)
	return server.Serve(listener)
}
