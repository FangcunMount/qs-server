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
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot load QS evaluation client certificate")
	}
	roots, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot load QS evaluation CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(roots) {
		return nil, nil, fmt.Errorf("invalid QS evaluation CA")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, RootCAs: pool}
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(config)), grpc.WithDisableRetry())
	if err != nil {
		return nil, nil, err
	}
	return NewEvaluationClient(conn), conn, nil
}
