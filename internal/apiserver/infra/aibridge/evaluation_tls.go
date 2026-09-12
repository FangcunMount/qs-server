package aibridge

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func DialEvaluationManagement(address, caFile, certFile, keyFile string) (*EvaluationClient, io.Closer, error) {
	evaluation, _, connection, err := DialGovernanceManagement(address, caFile, certFile, keyFile)
	return evaluation, connection, err
}

func DialGovernanceManagement(address, caFile, certFile, keyFile string) (*EvaluationClient, *PublicationClient, io.Closer, error) {
	evaluation, publication, _, connection, err := DialGovernanceClients(address, caFile, certFile, keyFile)
	return evaluation, publication, connection, err
}

func DialGovernanceClients(address, caFile, certFile, keyFile string) (*EvaluationClient, *PublicationClient, *PromptDraftClient, io.Closer, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("cannot load QS evaluation client certificate")
	}
	roots, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("cannot load QS evaluation CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(roots) {
		return nil, nil, nil, nil, fmt.Errorf("invalid QS evaluation CA")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, RootCAs: pool}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDisableRetry())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return NewEvaluationClient(conn), NewPublicationClient(conn), NewPromptDraftClient(conn), conn, nil
}
