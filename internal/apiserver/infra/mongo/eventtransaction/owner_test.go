package eventtransaction

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
	"strings"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func ownerFixture(t *testing.T) (*Owner, *options.MessagingOptions, *mongo.Database, transaction.Execution, *evidence.EventEvidenceV2) {
	t.Helper()
	// NewClient stays disconnected. Any constructor query/DDL would fail.
	client, err := mongo.NewClient() //nolint:staticcheck // An inert fixture; Connect would start monitoring/network work.
	require.NoError(t, err)
	db := client.Database("original_primary")
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ca.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(`{"username":"fixture","password":"fixture-only"}`), 0600))
	topics := []string{"answers.lifecycle", "reports.lifecycle"}
	routes := make([]map[string]string, 0, len(topics))
	for _, topic := range topics {
		physical, routeErr := transport.PhysicalTopicV2(topic, transport.TransactionMessageType, ownerName, routeRevision)
		require.NoError(t, routeErr)
		routes = append(routes, map[string]string{"topic": topic, "physical_topic": physical, "message_type": "TRANSACTION", "owner": ownerName, "route_revision": routeRevision})
	}
	raw, err := json.Marshal(map[string]any{"version": 2, "routes": routes})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "routes.json"), raw, 0600))
	disabled := false
	o := options.NewMessagingOptions()
	o.Provider = "rocketmq"
	o.RocketMQ = &options.RocketMQOptions{Endpoint: "127.0.0.1:1", CredentialsFile: filepath.Join(dir, "credentials.json"), CAFile: filepath.Join(dir, "ca.crt"), RoutesFile: filepath.Join(dir, "routes.json"), PublishEnabled: &disabled, Transactions: map[string]options.TransactionOptions{ownerName: {StoreID: "original-primary", WriterEpoch: "m3-1", RouteRevision: routeRevision}}}
	owner, err := New(db, o, topics)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, owner.Runtime().Close(ctx))
	})
	at := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	m, err := message.New(message.Input{ID: "report-event", Producer: "qs-server", Destination: topics[1], EventType: catalog.InterpretationReportGenerated, SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{}`)})
	require.NoError(t, err)
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: ownerName, Scope: "org:1", OperationID: "report-operation", IdempotencyKey: "report/1", Branches: []transaction.BranchInput{{ID: "generated", Message: m, Wire: m.Input().Payload, PhysicalTopic: routes[1]["physical_topic"], RouteRevision: routeRevision}}})
	require.NoError(t, err)
	e := transaction.Execution{Plan: plan, Fence: 1, WriterEpoch: "m3-1", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}
	ref, err := plan.Reference("generated", e.Fence, e.WriterEpoch)
	require.NoError(t, err)
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(m), ref, strings.Repeat("b", 64), at)
	require.NoError(t, err)
	return owner, o, db, e, proof
}

// SharedOwnerFixture is exported only in the test binary for cross-adapter
// ownership tests; it is not part of the production package API.
var SharedOwnerFixture = ownerFixture

func TestSharedOwnerConstructorIsInertAndKeepsOriginalDatabaseIdentity(t *testing.T) {
	o, cfg, db, e, _ := ownerFixture(t)
	require.False(t, o.Runtime().Stats().Ready)
	require.True(t, o.HasTopic("answers.lifecycle"))
	require.True(t, o.HasTopic("reports.lifecycle"))
	require.True(t, o.UsesDatabase(db.Client().Database(db.Name())))
	require.False(t, o.UsesDatabase(db.Client().Database("different_primary")))
	other, err := mongo.NewClient() //nolint:staticcheck // Disconnected identity fixture, no I/O.
	require.NoError(t, err)
	require.False(t, o.UsesDatabase(other.Database(db.Name())))
	_, err = o.Reserve(t.Context(), transaction.RequestFor(e.Plan), e.Plan)
	require.ErrorIs(t, err, messagingruntime.ErrTransactionAdmissionClosed)
	_, err = New(db, cfg, []string{"reports.lifecycle", "reports.lifecycle"})
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = New(db, cfg, []string{"unregistered.topic"})
	require.Error(t, err, "configured topics must resolve through the fixed v2 route manifest")
	_, err = New(nil, cfg, []string{"reports.lifecycle"})
	require.ErrorIs(t, err, transaction.ErrBinding)
	_, err = New(&mongo.Database{}, cfg, []string{"reports.lifecycle"})
	require.ErrorIs(t, err, transaction.ErrBinding)
}

func TestOriginalGuardRejectsMissingSessionAndClosedOrWrongDatabaseContext(t *testing.T) {
	o, _, db, e, proof := ownerFixture(t)
	called := false
	err := o.WithinOriginal(t.Context(), e, func(context.Context) error { called = true; return nil })
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	require.False(t, called)
	require.ErrorIs(t, RequireOriginal(t.Context(), db, proof), transaction.ErrTransactionRequired)
	root := &originalRoot{owner: o, execution: e}
	ctx := context.WithValue(t.Context(), rootKey{}, root)
	require.ErrorIs(t, RequireOriginal(ctx, db.Client().Database("wrong_database"), proof), transaction.ErrBinding)
	root.closed.Store(true)
	require.ErrorIs(t, RequireOriginal(ctx, db, proof), transaction.ErrTransactionRequired)
}

func TestOriginalReferenceBindsExactBranchMetadataFenceAndWire(t *testing.T) {
	_, _, _, e, proof := ownerFixture(t)
	require.NoError(t, matchReference(e, proof))
	for _, kind := range []string{"event-type", "producer", "occurred-at", "fence", "wire"} {
		t.Run(kind, func(t *testing.T) {
			bad := proof.Clone()
			switch kind {
			case "event-type":
				bad.Reference.EventType = catalog.InterpretationReportFailed
			case "producer":
				bad.Reference.Producer = "different-producer"
			case "occurred-at":
				bad.Reference.OccurredAt = bad.RecordedAt.Add(time.Second).Format(time.RFC3339Nano)
			default:
				ref, err := transaction.DecodeReference(bad.TransactionReference)
				require.NoError(t, err)
				if kind == "fence" {
					ref.Fence++
				} else {
					ref.WireHash[0] ^= 1
				}
				bad.TransactionReference, err = transaction.EncodeReference(ref)
				require.NoError(t, err)
			}
			require.Error(t, matchReference(e, bad))
		})
	}
}
