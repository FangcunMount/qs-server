package evaluation

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
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	mysqlFailure "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluationfailure"
	mysqlSubmit "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluationsubmit"
	mysqlEventTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	legacy "github.com/FangcunMount/qs-server/internal/testutil/legacymysqloutbox"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
	driver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type transactionRoutes struct{}

func (transactionRoutes) GetTopicForEvent(string) (string, bool) {
	return "qs.evaluation.lifecycle", true
}

type inertPostCommit struct{}

func (inertPostCommit) AfterCommit(context.Context, []event.DomainEvent, time.Time) {}
func TestTransactionSelectionRejectsIncompleteOrDifferentPrimary(t *testing.T) {
	_, err := normalizeDeps(Deps{MySQLDB: &gorm.DB{}, EventTransactionOwner: &mysqlEventTx.Committer{}})
	require.Error(t, err)
	_, err = normalizeDeps(Deps{MySQLDB: &gorm.DB{}, TransactionRoutes: transactionRoutes{}})
	require.Error(t, err)
	_, err = normalizeDeps(Deps{MySQLDB: &gorm.DB{}, EventTransactionOwner: &mysqlEventTx.Committer{}, TransactionRoutes: transactionRoutes{}})
	require.Error(t, err)
}
func TestTransactionModuleConstructsAllPortsWithoutIOOrOwnerLifecycle(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	db, err := gorm.Open(driver.New(driver.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
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
	physical, err := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, "qs.mysql", "m3-1")
	require.NoError(t, err)
	routes, err := json.Marshal(map[string]any{"version": 2, "routes": []map[string]string{{"topic": topic, "physical_topic": physical, "message_type": "TRANSACTION", "owner": "qs.mysql", "route_revision": "m3-1"}}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "routes.json"), routes, 0600))
	o := options.NewMessagingOptions()
	o.Provider = "rocketmq"
	closed := false
	o.RocketMQ = &options.RocketMQOptions{Endpoint: "127.0.0.1:1", CredentialsFile: filepath.Join(dir, "credentials.json"), CAFile: filepath.Join(dir, "ca.crt"), RoutesFile: filepath.Join(dir, "routes.json"), PublishEnabled: &closed, Transactions: map[string]options.TransactionOptions{"qs.mysql": {StoreID: "module-original-primary", WriterEpoch: "m3-1", RouteRevision: "m3-1"}}}
	owner, err := mysqlEventTx.New(db, o, transactionRoutes{}, []string{topic})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, owner.Runtime().Close(ctx))
	})
	stager, err := legacy.NewStager(transactionRoutes{}, "fixture")
	require.NoError(t, err)
	deps := Deps{MySQLDB: db, EventTransactionOwner: owner, TransactionRoutes: transactionRoutes{}, OutboxProfile: appEventing.ProfileBinding{Stager: stager, PostCommit: inertPostCommit{}}}
	normalized, err := normalizeDeps(deps)
	require.NoError(t, err)
	infra, err := newEvaluationInfra(normalized)
	require.NoError(t, err)
	_, ok := infra.transactionSubmitter.(*mysqlSubmit.Submitter)
	require.True(t, ok)
	_, ok = infra.transactionFailure.(*mysqlFailure.Committer)
	require.True(t, ok)
	require.False(t, owner.Runtime().Stats().Ready)
	module, err := New(deps)
	require.NoError(t, err)
	require.NotNil(t, module.IntakeService)
	require.NoError(t, module.Cleanup())
	require.NoError(t, owner.Runtime().SetAdmission(false), "module cleanup must not close the host owner")
	require.False(t, owner.Runtime().Stats().Ready, "module construction/cleanup must not manage the shared runtime")

	// The default final host constructs no publication Outbox at all. Existing
	// fixture-supplied legacy ports above must not be required or retained.
	require.Nil(t, infra.assessmentOutboxStore)
	require.Nil(t, infra.postCommit)
	deps.OutboxProfile = appEventing.ProfileBinding{}
	finalModule, err := New(deps)
	require.NoError(t, err, "final transaction host still required legacy Outbox wiring")
	require.NotNil(t, finalModule.IntakeService)
	require.NoError(t, finalModule.Cleanup())
	require.NoError(t, mock.ExpectationsWereMet(), "constructor or cleanup performed database IO")
}
