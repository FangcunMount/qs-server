//go:build integration

package aibridge

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	jose "github.com/go-jose/go-jose/v4"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestMQPayloadMTLSTrustAndExactStoredReference(t *testing.T) {
	dsn := os.Getenv("QS_AI_MQ_TEST_DSN")
	if dsn == "" {
		t.Fatal("required disposable QS MQ MySQL DSN is missing")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	observation := func(kind string) uint64 {
		t.Helper()
		var count uint64
		if err := db.QueryRow("SELECT recorded_count FROM ai_messaging_observations WHERE kind=?", kind).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	referenceBefore, workloadBefore := observation("payload_serve_reference_mismatch"), observation("payload_serve_workload_denied")
	id := uuid.NewString()
	scope := app.OperationScope{OrganizationID: "1", SubjectID: "42", ResourceID: id}
	defer func() {
		for _, table := range []string{"ai_messaging_operations", "ai_messaging_outbox", "ai_messaging_aggregates"} {
			if _, err := db.Exec("DELETE FROM "+table+" WHERE aggregate_key=?", id); err != nil {
				t.Error(err)
			}
		}
	}()
	newKey := func(id string) jose.JSONWebKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return jose.JSONWebKey{Key: k, KeyID: id}
	}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: id, Actor: &pb.Actor{OrgId: "1", SubjectId: "42"}, TesteeId: "7", AssessmentIds: []string{"9"}, Goal: "fixture", Evidence: []*pb.EvidenceItem{{Facts: []*pb.Fact{{Ref: "standard_report", Value: strings.Repeat("🙂", 32768)}}}}}}}
	recipient := newKey("ai-crypt")
	prepared, err := app.ProtectMessaging(pb.MessagingKind_START, id, id, "", "1", "", body, newKey("qs-sign"), recipient.Public())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Envelope.GetPayloadReference() == nil {
		t.Fatal("128 KiB unicode body was not referenced")
	}
	messages := store.NewMessagingStore()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = messages.StageOperation(context.Background(), tx, prepared.Envelope, prepared.Body, scope, func() (*app.PreparedMessaging, error) { return prepared, nil }); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// Real TLS certificates/handshake, rather than a manufactured peer context.
	caKey := newKey("ca").Key.(*ecdsa.PrivateKey)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "isolated MQ CA"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	leaf := func(cn string, serial int64, server bool) tls.Certificate {
		k := newKey(cn).Key.(*ecdsa.PrivateKey)
		template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: cn}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		if server {
			template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, ca, &k.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{leaf("qs-apiserver.svc", 2, true)}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert})))
	(&MessagePayloads{Reader: &store.MessagingReader{DB: db, Store: messages}}).RegisterService(server)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close(); <-done })
	client := func(cn string, serial int64) pb.MessagePayloadsClient {
		conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{leaf(cn, serial, false)}, RootCAs: pool})), grpc.WithDisableRetry(), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(app.MaxMessagingBody+2048)))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return pb.NewMessagePayloadsClient(conn)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ref := prepared.Envelope.GetPayloadReference()
	authorized := client("qs-ai.svc", 3)
	got, err := authorized.Get(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got.Reference, ref) || !bytes.Equal(got.Body, prepared.Body) {
		t.Fatal("payload reference changed original bytes")
	}
	for _, change := range []func(*pb.MessagePayloadReference){func(r *pb.MessagePayloadReference) { r.OrganizationId = "2" }, func(r *pb.MessagePayloadReference) { r.BodySha256 = strings.Repeat("a", 64) }, func(r *pb.MessagePayloadReference) { r.BodyLength++ }} {
		bad := proto.Clone(ref).(*pb.MessagePayloadReference)
		change(bad)
		if _, err := authorized.Get(ctx, bad); status.Code(err) != codes.NotFound {
			t.Fatalf("cross-scope/corrupt reference accepted: %v", err)
		}
	}
	if _, err := client("other-workload.svc", 4).Get(ctx, ref); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("wrong workload accepted: %v", err)
	}
	if observation("payload_serve_reference_mismatch") != referenceBefore+3 || observation("payload_serve_workload_denied") != workloadBefore+1 {
		t.Fatal("real mTLS rejection did not persist bounded technical facts")
	}
	if err = db.Ping(); err != nil {
		t.Fatal("borrowed host pool closed")
	}
}
