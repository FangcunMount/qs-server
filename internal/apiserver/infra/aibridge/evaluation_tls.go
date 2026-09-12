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
	clients, err := DialGovernance(address, caFile, certFile, keyFile)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return clients.Evaluation, clients.Publications, clients.PromptDrafts, clients.Connection, nil
}

type GovernanceClients struct {
	Assets       *AssetCatalogClient
	Evaluation   *EvaluationClient
	Publications *PublicationClient
	PromptDrafts *PromptDraftClient
	Suites       *SuiteClient
	Profiles     *ProfileClient
	Connection   io.Closer
}

func DialGovernance(address, caFile, certFile, keyFile string) (*GovernanceClients, error) {
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("cannot load QS evaluation client certificate")
	}
	roots, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("cannot load QS evaluation CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(roots) {
		return nil, fmt.Errorf("invalid QS evaluation CA")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, RootCAs: pool}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDisableRetry())
	if err != nil {
		return nil, err
	}
	return &GovernanceClients{Assets: NewAssetCatalogClient(conn), Evaluation: NewEvaluationClient(conn), Publications: NewPublicationClient(conn), PromptDrafts: NewPromptDraftClient(conn), Suites: NewSuiteClient(conn), Profiles: NewProfileClient(conn), Connection: conn}, nil
}
