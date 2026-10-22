package container

import (
	"context"
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

	"github.com/DATA-DOG/go-sqlmock"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongoTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	mysqlTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type ownerIdentityNormalWire struct{}

func (ownerIdentityNormalWire) PublishWire(context.Context, string, []byte) error { return nil }

// Actual constructors and factory, with disconnected original handles. This
// regression proves composition identity, not native DB or Broker behavior.
func TestRetryFactoryRequiresRegisteredOwnerIdentity(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	client, err := mongo.NewClient(mongooptions.Client().ApplyURI("mongodb://127.0.0.1:1")) //nolint:staticcheck // SA1019: Deliberately disconnected; no constructor network I/O permitted.
	require.NoError(t, err)
	documents := client.Database("owner_identity_inert")
	directory := t.TempDir()
	write := func(name string, body []byte) string {
		t.Helper()
		path := filepath.Join(directory, name)
		require.NoError(t, os.WriteFile(path, body, 0600))
		return path
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certificate := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	require.NoError(t, err)
	roots := write("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	var bindings []transport.RouteBinding
	for owner, topics := range map[string][]string{"qs.mysql": {"qs.evaluation.lifecycle", "qs.plan.task", "qs.ai.commands.v1", "qs.ai.acks.v1"}, "qs.mongo": {"qs.evaluation.lifecycle"}} {
		for _, topic := range topics {
			physical, e := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, "m3-1")
			require.NoError(t, e)
			bindings = append(bindings, transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: "m3-1"})
		}
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "routes": bindings})
	require.NoError(t, err)
	conf := options.NewMessagingOptions()
	conf.Enabled = true
	conf.Provider = "rocketmq"
	conf.RocketMQ = &options.RocketMQOptions{Endpoint: "127.0.0.1:1", CAFile: roots, RoutesFile: write("routes.json", raw), Transactions: map[string]options.TransactionOptions{}}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		body, e := json.Marshal(map[string]string{"username": owner, "password": "inert-owner-identity"})
		require.NoError(t, e)
		path := write(owner+".json", body)
		conf.RocketMQ.Transactions[owner] = options.TransactionOptions{CredentialsFile: path, StoreID: owner + "-original", WriterEpoch: "m3-1", RouteRevision: "m3-1"}
		if owner == "qs.mysql" {
			conf.RocketMQ.CredentialsFile = path
		}
	}
	source, err := catalog.Load("../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	events, err := eventsubsystem.NewWithTransactionOwners(eventsubsystem.Options{MySQLDB: db, MongoDB: documents, Catalog: routes, WirePublisher: ownerIdentityNormalWire{}, PublisherMode: eventruntime.PublishModeMQ}, conf)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, events.CloseContext(context.Background())) })
	duplicateSQL, err := mysqlTx.New(db, conf, routes, []string{"qs.evaluation.lifecycle"})
	require.NoError(t, err)
	duplicateMongo, err := mongoTx.New(documents, conf, []string{"qs.evaluation.lifecycle"})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, duplicateSQL.Runtime().Close(context.Background()))
		require.NoError(t, duplicateMongo.Runtime().Close(context.Background()))
	})
	for _, test := range []struct {
		name     string
		sql      *mysqlTx.Committer
		document *mongoTx.Owner
		valid    bool
	}{
		{"registered", events.MySQLTransactionOwner(), events.MongoTransactionOwner(), true},
		{"duplicate_mysql_runtime", duplicateSQL, events.MongoTransactionOwner(), false},
		{"duplicate_mongo_runtime", events.MySQLTransactionOwner(), duplicateMongo, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := NewContainerWithOptions(db, documents, nil, ContainerOptions{Silent: true, EventSubsystem: events, Transactions: &EventTransactionBindings{MySQL: test.sql, Mongo: test.document, Routes: routes}})
			factory, e := host.BuildRetryDispatchers(true)
			if test.valid {
				require.NoError(t, e)
				require.NotNil(t, factory)
			} else {
				require.Error(t, e, "same DB does not authorize an independent runtime")
				require.Nil(t, factory)
			}
			require.NoError(t, mock.ExpectationsWereMet(), "factory performed database I/O")
		})
	}
}
