package aibridge

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type tlsManagementServer struct {
	pb.UnimplementedEvaluationManagementServer
	t *testing.T
}

func (s *tlsManagementServer) Get(ctx context.Context, r *pb.EvaluationQuery) (*pb.EvaluationState, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		s.t.Fatal("missing TLS peer")
	}
	info, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.VerifiedChains) == 0 || info.State.PeerCertificates[0].Subject.CommonName != "qs-apiserver.svc" {
		s.t.Fatal("unverified QS workload")
	}
	return &pb.EvaluationState{RunId: r.RunId, Version: 1, Status: "blocked", ResolutionsJson: "[]"}, nil
}
func TestEvaluationManagementConnectionUsesMutualTLSAndCloses(t *testing.T) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-only-ca"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	caPath := write("ca.pem", caPEM)
	issue := func(name string, serial int64) (string, string, tls.Certificate) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		der, err := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		certPath := write(name+".crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		keyPath := write(name+".key", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
		pair, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			t.Fatal(err)
		}
		return certPath, keyPath, pair
	}
	_, _, serverPair := issue("qs-ai", 2)
	certPath, keyPath, _ := issue("qs-apiserver.svc", 3)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{serverPair}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert})))
	pb.RegisterEvaluationManagementServer(server, &tlsManagementServer{t: t})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	go func() { _ = server.Serve(listener) }()
	client, closer, err := DialEvaluationManagement(listener.Addr().String(), caPath, certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()
	scope := app.EvaluationScope{RunID: "00000000-0000-4000-8000-000000000001", OrganizationID: 1, OperatorUserID: 1}
	value, err := client.GetEvaluation(context.Background(), scope)
	if err != nil || value.Version != 1 {
		t.Fatalf("%+v %v", value, err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetEvaluation(context.Background(), scope); err == nil {
		t.Fatal("closed connection remained usable")
	}
}
