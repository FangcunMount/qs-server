// Disposable cryptographic/configuration fixtures for the required native image
// preflight. This testdata program is not copied into any runtime image.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	binding "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/aimessagingbinding"
	options "github.com/FangcunMount/qs-server/internal/apiserver/options"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/FangcunMount/reliable-messaging/transport"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/spf13/viper"
)

func check(err error) {
	if err != nil {
		panic("disposable fixture unavailable")
	}
}
func write(path string, raw []byte) { check(os.WriteFile(path, raw, 0600)) }
func encode(path string, v any)     { raw, err := json.Marshal(v); check(err); write(path, raw) }

func main() {
	if len(os.Args) != 2 {
		panic("expected owned fixture directory")
	}
	root, err := filepath.Abs(os.Args[1])
	check(err)
	info, err := os.Stat(root)
	check(err)
	if !info.IsDir() {
		panic("fixture directory required")
	}
	entries, err := os.ReadDir(root)
	check(err)
	if len(entries) != 0 {
		panic("owned empty fixture directory required")
	}
	version := filepath.Join(root, "bindings", "versions", "m4-native-fixture")
	check(os.MkdirAll(filepath.Join(version, "rocketmq"), 0700))
	raw, err := os.ReadFile("configs/apiserver.prod.yaml")
	check(err)
	base := filepath.Join(root, "source.yaml")
	write(base, raw)
	cmd := exec.Command("python3", "scripts/cd/render-api-messaging.py", "render", base)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "MESSAGING_PROVIDER=rocketmq", "ROCKETMQ_ENDPOINT=broker.invalid:8081", "ROCKETMQ_INSTANCE_ID=qs-apiserver-1", "ROCKETMQ_MYSQL_STORE_ID=Native-$主", "ROCKETMQ_MYSQL_WRITER_EPOCH=Native-$$-Epoch", "ROCKETMQ_MONGO_STORE_ID=Native-Mongo", "ROCKETMQ_MONGO_WRITER_EPOCH=Native-Mongo-Epoch"}
	check(cmd.Run())
	v := viper.New()
	v.SetConfigFile(base)
	check(v.ReadInConfig())
	o := options.NewOptions()
	check(app.DecodeOptions(v, o))
	sql := o.MessagingOptions.RocketMQ.Transactions["qs.mysql"]
	b := binding.Binding{Provider: "rocketmq", Revision: "m4-native-fixture", Enabled: true, PublishEnabled: o.MessagingOptions.RocketMQ.PublishEnabled, MaxInFlight: 1, Signing: "/run/qs-server-jose/qs.sign.fixture.json", Recipient: "/run/qs-server-jose/ai.encrypt.fixture.json", Decrypt: map[string]string{"qs.encrypt.fixture": "/run/qs-server-jose/qs.encrypt.fixture.json"}, Signers: map[string]string{"ai.sign.fixture": "/run/qs-server-jose/ai.sign.fixture.json"}}
	mq := *o.MessagingOptions.RocketMQ
	mq.Transactions = make(map[string]generic.TransactionOptions)
	mq.Transactions["qs.mysql"] = sql
	b.RocketMQ = &mq
	data, err := json.Marshal(b)
	check(err)
	var object map[string]any
	check(json.Unmarshal(data, &object))
	delete(object, "nsqd")
	encode(filepath.Join(root, "bindings", "current.json"), object)
	for _, role := range []string{"qs.sign", "qs.encrypt", "ai.sign", "ai.encrypt"} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		check(err)
		jwk := jose.JSONWebKey{Key: key, KeyID: role + ".fixture"}
		if role[:2] == "ai" {
			jwk = jwk.Public()
		}
		encode(filepath.Join(version, role+".fixture.json"), jwk)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(err)
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	check(err)
	write(filepath.Join(version, "rocketmq", "ca.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	for _, name := range []string{"credentials.json", "mongo-credentials.json"} {
		encode(filepath.Join(version, "rocketmq", name), map[string]string{"username": name, "password": "disposable-only"})
	}
	var routes []transport.RouteBinding
	for owner, topics := range map[string][]string{"qs.mysql": {"qs.evaluation.lifecycle", "qs.plan.task", "qs.ai.commands.v1", "qs.ai.acks.v1"}, "qs.mongo": {"qs.evaluation.lifecycle"}, "ai.mysql": {"qs.ai.events.v1"}, "iam.mysql": {"iam.authz.version.v2"}} {
		for _, topic := range topics {
			physical, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, "m3-1")
			check(err)
			routes = append(routes, transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: "m3-1"})
		}
	}
	encode(filepath.Join(version, "rocketmq", "routes.json"), map[string]any{"version": 2, "routes": routes})
	for i := range routes {
		if routes[i].Topic == "qs.ai.events.v1" {
			routes[i].Owner = "qs.mysql"
			routes[i].PhysicalTopic, err = transport.PhysicalTopicV2(routes[i].Topic, transport.TransactionMessageType, "qs.mysql", "m3-1")
			check(err)
		}
	}
	encode(filepath.Join(root, "wrong-events.routes.json"), map[string]any{"version": 2, "routes": routes})
}
