package interpretation

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
	mongoEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	mongoOpts "go.mongodb.org/mongo-driver/mongo/options"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type reportTransactionRoutes struct{}

func (reportTransactionRoutes) GetTopicForEvent(string) (string, bool) {
	return "qs.evaluation.lifecycle", true
}
func TestFinalTransactionReportModuleRequiresNoLegacyOutboxOrDatabaseIO(t *testing.T) {
	client, err := mongo.NewClient(mongoOpts.Client().ApplyURI("mongodb://127.0.0.1:1")) //nolint:staticcheck // SA1019: Deliberately unconnected to prove constructor performs no database I/O.
	require.NoError(t, err)
	db := client.Database("m3_report_inert")
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	mysqlDB, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)

	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(`{"username":"fixture","password":"fixture-only"}`), 0600))
	const topic = "qs.evaluation.lifecycle"
	physical, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, "qs.mongo", "m3-1")
	require.NoError(t, err)
	routes, err := json.Marshal(map[string]any{"version": 2, "routes": []map[string]string{{"topic": topic, "physical_topic": physical, "message_type": "TRANSACTION", "owner": "qs.mongo", "route_revision": "m3-1"}}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "routes.json"), routes, 0600))
	o := options.NewMessagingOptions()
	o.Provider = "rocketmq"
	closed := false
	o.RocketMQ = &options.RocketMQOptions{Endpoint: "127.0.0.1:1", CredentialsFile: filepath.Join(dir, "credentials.json"), CAFile: filepath.Join(dir, "ca.crt"), RoutesFile: filepath.Join(dir, "routes.json"), PublishEnabled: &closed, Transactions: map[string]options.TransactionOptions{"qs.mongo": {StoreID: "module-original-mongo", WriterEpoch: "m3-1", RouteRevision: "m3-1"}}}
	owner, err := mongoEventTx.New(db, o, []string{topic})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, owner.Runtime().Close(ctx))
	})
	module, err := New(Deps{RunLeaseDuration: time.Minute, MySQLDB: mysqlDB, MongoDB: db, EventTransactionOwner: owner, TransactionRoutes: reportTransactionRoutes{}})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(), "report constructor performed MySQL I/O")
	require.NotNil(t, module.executionExecutor)
	require.Nil(t, module.eventStager, "final report module retained publication Outbox")
	require.Equal(t, owner, module.TransactionOwner())
	require.NoError(t, module.Cleanup())
	require.False(t, owner.Runtime().Stats().Ready)
	require.NoError(t, owner.Runtime().SetAdmission(false), "module cleanup closed shared owner")
}
