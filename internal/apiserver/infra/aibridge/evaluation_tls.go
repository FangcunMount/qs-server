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

type GovernanceClients struct {
	Payloads       *MessagingPayloadClient
	Flows          *FlowClient
	Runtime        *RuntimeClient
	SemanticDrafts *SemanticDraftClient
	Solutions      *SolutionClient
	Quotas         *QuotaClient
	Commands       *Client
	Participants   *ParticipantClient
	Assets         *AssetCatalogClient
	Evaluation     *EvaluationClient
	Publications   *PublicationClient
	PromptDrafts   *PromptDraftClient
	Suites         *SuiteClient
	Profiles       *ProfileClient
	Connection     io.Closer
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
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDisableRetry(), grpc.WithChainUnaryInterceptor(correlateRPC))
	if err != nil {
		return nil, err
	}
	return &GovernanceClients{Payloads: NewMessagingPayloadClient(conn), Flows: NewFlowClient(conn), Runtime: NewRuntimeClient(conn), SemanticDrafts: NewSemanticDraftClient(conn), Quotas: NewQuotaClient(conn), Solutions: NewSolutionClient(conn), Commands: New(conn), Participants: NewParticipantClient(conn), Assets: NewAssetCatalogClient(conn), Evaluation: NewEvaluationClient(conn), Publications: NewPublicationClient(conn), PromptDrafts: NewPromptDraftClient(conn), Suites: NewSuiteClient(conn), Profiles: NewProfileClient(conn), Connection: conn}, nil
}
