package messagingruntime

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/options"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
)

func TestTransactionConstructorIsInertAndUsesExactBinding(t *testing.T) {
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"ca.crt":           pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		"credentials.json": []byte("{\"username\":\"fixture\",\"password\":\"fixture-only\"}"),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	const owner = "qs.mysql"
	const topic = "qs.evaluation.lifecycle"
	physical, _ := transport.PhysicalTopicV2(topic, "TRANSACTION", owner, "m3-1")
	raw := []byte("{\"version\":2,\"routes\":[{\"topic\":\"" + topic + "\",\"physical_topic\":\"" + physical + "\",\"message_type\":\"TRANSACTION\",\"owner\":\"" + owner + "\",\"route_revision\":\"m3-1\"}]}")
	if err := os.WriteFile(filepath.Join(dir, "routes.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	binding := transaction.Binding{Owner: owner, StoreID: "fixture-primary", WriterEpoch: "m3-epoch"}
	// A zero DB cannot execute queries. Constructor success proves no DB access.
	pool := new(sql.DB)
	store, err := txmysql.New(pool, binding)
	if err != nil {
		t.Fatal(err)
	}
	disabled := false
	o := options.NewMessagingOptions()
	o.Provider = "rocketmq"
	o.RocketMQ = &options.RocketMQOptions{Endpoint: "127.0.0.1:1", CredentialsFile: filepath.Join(dir, "credentials.json"), CAFile: filepath.Join(dir, "ca.crt"), RoutesFile: filepath.Join(dir, "routes.json"), PublishEnabled: &disabled, Transactions: map[string]options.TransactionOptions{owner: {StoreID: binding.StoreID, WriterEpoch: binding.WriterEpoch, RouteRevision: "m3-1"}}}
	native, err := NewSQLNativeLimits(pool, binding, "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := NewTransactionalSender(o, owner, store, []string{topic}, native)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Stats().Ready {
		t.Fatal("constructor started runtime")
	}
	if _, err := runtime.Reserve(context.Background(), transaction.Request{}, transaction.FrozenPlan{}); err != ErrTransactionAdmissionClosed {
		t.Fatal("closed gate reached DB", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	wrong, _ := txmysql.New(new(sql.DB), transaction.Binding{Owner: owner, StoreID: "wrong-primary", WriterEpoch: binding.WriterEpoch})
	if _, err := NewTransactionalSender(o, owner, wrong, []string{topic}, native); err == nil {
		t.Fatal("wrong primary store accepted")
	}
}
