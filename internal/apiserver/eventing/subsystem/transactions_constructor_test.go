package subsystem

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
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/mongo/options"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type transactionNormalWire struct{ calls int }

func (w *transactionNormalWire) PublishWire(context.Context, string, []byte) error {
	w.calls++
	return nil
}

func TestTransactionSubsystemBorrowsBothOriginalStoresWithoutStartingOrInstalling(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	client, err := mongo.NewClient(mongoOptions.Client().ApplyURI("mongodb://127.0.0.1:1")) //nolint:staticcheck // SA1019: Deliberately disconnected to prove no constructor I/O.
	require.NoError(t, err)
	mongoDB := client.Database("m3_constructor_only")
	directory := t.TempDir()
	write := func(name string, body []byte) string {
		t.Helper()
		path := filepath.Join(directory, name)
		require.NoError(t, os.WriteFile(path, body, 0600))
		return path
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	require.NoError(t, err)
	root := write("ca.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	files := map[string]string{}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		body, err := json.Marshal(map[string]string{"username": owner, "password": "constructor-only-fixture"})
		require.NoError(t, err)
		files[owner] = write(owner+".json", body)
	}
	var routes []transport.RouteBinding
	for owner, topics := range map[string][]string{"qs.mysql": {"qs.evaluation.lifecycle", "qs.plan.task", "qs.ai.commands.v1", "qs.ai.acks.v1", "qs.mysql.idle-topic"}, "qs.mongo": {"qs.evaluation.lifecycle"}} {
		for _, topic := range topics {
			physical, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, owner, "m3-1")
			require.NoError(t, err)
			routes = append(routes, transport.RouteBinding{Topic: topic, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: owner, RouteRevision: "m3-1"})
		}
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "routes": routes})
	require.NoError(t, err)
	o := options.NewMessagingOptions()
	o.Enabled = true
	o.Provider = "rocketmq"
	o.RocketMQ = &options.RocketMQOptions{Endpoint: "127.0.0.1:1", CAFile: root, CredentialsFile: files["qs.mysql"], RoutesFile: write("routes.json", raw), Transactions: map[string]options.TransactionOptions{}}
	for owner, file := range files {
		o.RocketMQ.Transactions[owner] = options.TransactionOptions{CredentialsFile: file, StoreID: owner + "-original-primary", WriterEpoch: "m3-1", RouteRevision: "m3-1"}
	}
	wire := &transactionNormalWire{}
	opts := Options{MySQLDB: db, MongoDB: mongoDB, Catalog: loadCatalog(t), WirePublisher: wire, PublisherMode: eventruntime.PublishModeMQ}
	s, err := NewWithTransactionOwners(opts, o)
	require.NoError(t, err)
	require.True(t, s.HasTransactionOwners())
	require.True(t, s.MySQLTransactionOwner().UsesPrimary(db))
	require.True(t, s.MongoTransactionOwner().UsesDatabase(mongoDB))
	for _, topic := range []string{"qs.evaluation.lifecycle", "qs.plan.task", "qs.ai.commands.v1", "qs.ai.acks.v1"} {
		require.True(t, s.MySQLTransactionOwner().HasTopic(topic))
	}
	require.True(t, s.MySQLTransactionOwner().HasTopic("qs.mysql.idle-topic"), "idle owner failed to register all configured transaction topics")
	require.False(t, s.MySQLTransactionOwner().Runtime().Stats().Ready)
	require.False(t, s.MongoTransactionOwner().Runtime().Stats().Ready)
	require.Empty(t, s.profiles, "transaction mode must not construct a publication Outbox/Relay")
	require.Nil(t, s.Profile(eventcatalog.OutboxProfileAssessmentMySQL).Stager)
	require.Nil(t, s.Profile(eventcatalog.OutboxProfileMongoDomain).Stager)
	require.NoError(t, mock.ExpectationsWereMet(), "constructor performed database I/O")
	require.NoError(t, s.Publisher().Publish(t.Context(), event.New(eventcatalog.TaskOpened, "fixture", "1", map[string]string{"id": "1"})))
	require.Error(t, s.Publisher().Publish(t.Context(), event.New(eventcatalog.TaskOpenedReminderRequested, "fixture", "1", map[string]string{"id": "1"})))
	require.Equal(t, 1, wire.calls, "durable same-topic event reached NORMAL client")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, s.CloseContext(ctx))
	require.NoError(t, s.Wait(ctx))
	mock.ExpectExec("SELECT 1").WillReturnResult(sqlmock.NewResult(0, 1))
	_, err = pool.ExecContext(ctx, "SELECT 1")
	require.NoError(t, err, "subsystem closed the borrowed original pool")
	require.NoError(t, mock.ExpectationsWereMet())
	delete(o.RocketMQ.Transactions, "qs.mongo")
	_, err = NewWithTransactionOwners(opts, o)
	require.Error(t, err, "missing owner selected legacy publication")
}
