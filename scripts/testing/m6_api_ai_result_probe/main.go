// Command m6_api_ai_result_probe exercises the real API image's AI result RPC
// against a disposable database. It never invokes a model or production API.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const (
	requestID = "11111111-1111-4111-8111-111111111111"
	eventID   = "22222222-2222-4222-8222-222222222222"
	sessionID = "33333333-3333-4333-8333-333333333333"
)

func main() {
	mode := flag.String("mode", "", "certgen, stage, send, assert, or reject-conflict")
	directory := flag.String("cert-dir", "/tmp/certs", "disposable certificate directory")
	flag.Parse()
	var err error
	switch *mode {
	case "certgen":
		err = certgen(*directory)
	case "stage", "assert":
		err = databaseOperation(*mode)
	case "send", "reject-conflict":
		err = send(*mode, *directory)
	default:
		err = errors.New("mode required")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func databaseOperation(mode string) error {
	dsn := os.Getenv("QS_AI_BRIDGE_DSN")
	if dsn == "" {
		return errors.New("disposable MySQL DSN required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if mode == "stage" {
		return (&app.Service{Store: &store.Store{DB: db}}).Start(ctx, app.Start{
			RequestID: requestID, Actor: app.Actor{OrgID: "1", SubjectID: "m6-image-proof"},
			TesteeID: "7", AssessmentIDs: []string{"42"}, Goal: "Disposable AI result handoff",
		})
	}
	var count, version int
	var state, bound string
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_bridge_events WHERE event_id=? OR (request_id=? AND version=1)", eventID, requestID).Scan(&count); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, "SELECT version,status,session_id FROM ai_bridge_requests WHERE request_id=?", requestID).Scan(&version, &state, &bound); err != nil {
		return err
	}
	if count != 1 || version != 1 || state != "running" || bound != sessionID {
		return fmt.Errorf("result drift: events=%d version=%d status=%q session_match=%v", count, version, state, bound == sessionID)
	}
	fmt.Println("one original AI result, one version, unchanged projection")
	return nil
}

func send(mode, directory string) error {
	pair, err := tls.LoadX509KeyPair(filepath.Join(directory, "client.pem"), filepath.Join(directory, "client.key"))
	if err != nil {
		return err
	}
	ca, err := os.ReadFile(filepath.Join(directory, "ca.pem"))
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid test CA")
	}
	conn, err := grpc.NewClient("127.0.0.1:9090", grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS13, ServerName: "qs-apiserver.svc", RootCAs: roots,
		Certificates: []tls.Certificate{pair},
	})), grpc.WithDisableRetry())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	event := &pb.StateEvent{
		EventId: eventID, RequestId: requestID, SessionId: sessionID,
		Actor:    &pb.Actor{OrgId: "1", SubjectId: "m6-image-proof"},
		TesteeId: "7", Version: 1, Status: "running",
	}
	if mode == "reject-conflict" {
		event.Status = "blocked"
	}
	ack, err := pb.NewResultsClient(conn).Accept(ctx, event)
	if mode == "reject-conflict" {
		if status.Code(err) != codes.Aborted {
			return fmt.Errorf("changed duplicate must be rejected, got %v", err)
		}
		fmt.Println("changed duplicate rejected")
		return nil
	}
	if err != nil {
		return err
	}
	if ack.GetEventId() != eventID {
		return errors.New("acknowledgement did not preserve original event ID")
	}
	fmt.Println("original AI result acknowledged")
	return nil
}

func certgen(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	now := time.Now()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "m6-disposable-ca"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}
	if err := writePEM(filepath.Join(directory, "ca.pem"), "CERTIFICATE", caDER); err != nil {
		return err
	}
	for index, identity := range []struct {
		name  string
		usage x509.ExtKeyUsage
	}{
		{"qs-apiserver.svc", x509.ExtKeyUsageServerAuth},
		{"qs-ai.svc", x509.ExtKeyUsageClientAuth},
	} {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return err
		}
		certificate := &x509.Certificate{
			SerialNumber: big.NewInt(int64(index + 2)),
			Subject:      pkix.Name{CommonName: identity.name, OrganizationalUnit: []string{"QS"}},
			NotBefore:    now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{identity.usage},
			DNSNames: []string{identity.name},
		}
		der, err := x509.CreateCertificate(rand.Reader, certificate, ca, &key.PublicKey, caKey)
		if err != nil {
			return err
		}
		prefix := "server"
		if index == 1 {
			prefix = "client"
		}
		if err := writePEM(filepath.Join(directory, prefix+".pem"), "CERTIFICATE", der); err != nil {
			return err
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return err
		}
		// These short-lived keys exist only on a disposable isolated runner and
		// must be readable by the non-root API image user.
		if err := writePEM(filepath.Join(directory, prefix+".key"), "PRIVATE KEY", encoded); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(directory, "acl.yaml"), []byte("default_policy: deny\nservices:\n  - service_name: qs-ai.svc\n    enabled: true\n    allowed_methods:\n      - /qsai.workflow.v1.Results/Accept\n"), 0644)
}

func writePEM(path, kind string, data []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data}), 0644)
}
