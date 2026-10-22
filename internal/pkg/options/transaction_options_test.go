package options

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/transport"
)

func TestTransactionOwnerCredentialsAreExplicitDistinctAndDoNotMutateBase(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, raw []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	baseFile := write("base.json", []byte(`{"username":"base-reader","password":"fixture-only"}`))
	mysqlFile := write("mysql.json", []byte(`{"username":"mysql-writer","password":"mysql-fixture-only"}`))
	mongoFile := write("mongo.json", []byte(`{"username":"mongo-writer","password":"mongo-fixture-only"}`))
	ca := write("ca.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	const topic = "qs.evaluation.lifecycle"
	routes := []transport.RouteBinding{}
	bindings := map[string]TransactionOptions{}
	for owner, file := range map[string]string{"qs.mysql": mysqlFile, "qs.mongo": mongoFile} {
		physical, _ := transport.PhysicalTopicV2(topic, "TRANSACTION", owner, "m3-1")
		routes = append(routes, transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: "TRANSACTION", Owner: owner, RouteRevision: "m3-1"})
		bindings[owner] = TransactionOptions{StoreID: owner + "-primary", WriterEpoch: "m3-1", RouteRevision: "m3-1", CredentialsFile: file}
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "routes": routes})
	if err != nil {
		t.Fatal(err)
	}
	o := &RocketMQOptions{Endpoint: "127.0.0.1:1", CredentialsFile: baseFile, CAFile: ca, RoutesFile: write("routes.json", raw), Transactions: bindings}
	for owner, expected := range map[string]string{"qs.mysql": "mysql-writer", "qs.mongo": "mongo-writer"} {
		config, err := o.TransactionConfig(owner, []string{topic})
		if err != nil || config.AccessKey != expected || config.Owner != owner || len(config.Routes) != 1 {
			t.Fatalf("owner credential binding failed for %s: %v", owner, err)
		}
	}
	base, err := o.Load([]string{topic}, 1, time.Second)
	if err != nil || base.AccessKey != "base-reader" || o.CredentialsFile != baseFile {
		t.Fatal("base reader credentials changed", err)
	}
	mongoBinding := bindings["qs.mongo"]
	for _, file := range []string{"", mysqlFile, filepath.Join(dir, ".", "mysql.json")} {
		mongoBinding.CredentialsFile = file
		bindings["qs.mongo"] = mongoBinding
		if o.Validate() == nil {
			t.Fatal("shared/implicit multi-owner credentials accepted")
		}
	}
	mongoBinding.CredentialsFile = write("alias.json", []byte(`{"username":"mysql-writer","password":"other-fixture-only"}`))
	bindings["qs.mongo"] = mongoBinding
	if _, err := o.TransactionConfig("qs.mysql", []string{topic}); err == nil {
		t.Fatal("same principal through different files accepted")
	}
	mongoBinding.CredentialsFile = filepath.Join(dir, "missing.json")
	bindings["qs.mongo"] = mongoBinding
	if _, err := o.TransactionConfig("qs.mysql", []string{topic}); err == nil {
		t.Fatal("missing peer-owner credential file accepted")
	}
	delete(bindings, "qs.mongo")
	mysqlBinding := bindings["qs.mysql"]
	mysqlBinding.CredentialsFile = ""
	bindings["qs.mysql"] = mysqlBinding
	single, err := o.TransactionConfig("qs.mysql", []string{topic})
	if err != nil || single.AccessKey != "base-reader" {
		t.Fatal("existing explicit single-owner base binding regressed", err)
	}
}

func TestRouteManifestVersionAndTypeCannotBeMixed(t *testing.T) {
	const topic = "iam.authz.version.v2"
	physical, err := transport.PhysicalTopicV2(topic, "TRANSACTION", "iam.mysql", "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	v2 := transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: "TRANSACTION", Owner: "iam.mysql", RouteRevision: "m3-1"}
	legacy := transport.RouteBinding{Topic: topic, PhysicalTopic: transport.PhysicalTopic(topic)}
	for _, tc := range []struct {
		name    string
		version int
		route   transport.RouteBinding
		valid   bool
	}{
		{"v2", 2, v2, true}, {"legacy", 1, legacy, true},
		{"v2-missing-type", 2, legacy, false}, {"v1-hidden-v2", 1, v2, false}, {"unknown-version", 3, v2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(struct {
				Version int                      `json:"version"`
				Routes  []transport.RouteBinding `json:"routes"`
			}{tc.version, []transport.RouteBinding{tc.route}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = decodeRouteManifest(raw)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}

func TestTransactionConfigRequiresExactOwnerRevisionAndAllTopics(t *testing.T) {
	const owner = "qs.mysql"
	opts := TransactionOptions{StoreID: "primary-test", WriterEpoch: "m3-epoch", RouteRevision: "m3-1"}
	limits, err := opts.ProtocolLimits(owner)
	if err != nil {
		t.Fatal(err)
	}
	if limits.Sends != 4 || limits.CheckWorkers != 2 || limits.CheckQueue != 16 {
		t.Fatalf("unexpected budget: %+v", limits)
	}
	topic := "qs.evaluation.lifecycle"
	physical, _ := transport.PhysicalTopicV2(topic, "TRANSACTION", owner, "m3-1")
	routes := []transport.RouteBinding{{Topic: topic, PhysicalTopic: physical, MessageType: "TRANSACTION", Owner: owner, RouteRevision: "m3-1"}}
	got, err := transactionRoutes(routes, owner, opts, []string{topic})
	if err != nil || len(got) != 1 {
		t.Fatalf("valid routes=%v, err=%v", got, err)
	}
	if _, err = transactionRoutes(routes, owner, opts, []string{topic, "absent"}); err == nil {
		t.Fatal("missing topic accepted")
	}
	routes[0].Owner = "wrong.mysql"
	if _, err = transactionRoutes(routes, owner, opts, []string{topic}); err == nil {
		t.Fatal("wrong owner accepted")
	}
	opts.WriterEpoch = ""
	if _, err = opts.ProtocolLimits(owner); err == nil {
		t.Fatal("missing binding accepted")
	}
	opts.WriterEpoch = "m3-epoch"
	opts.Limits = &TransactionLimits{Sends: 1, Branches: 17, CheckWorkers: 1, CheckQueue: 1}
	if _, err = opts.ProtocolLimits(owner); err == nil {
		t.Fatal("invalid budget accepted")
	}
}
