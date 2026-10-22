package process

import (
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"testing"
	"time"

	txplan "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	txport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/worker/container"
	grpcclient "github.com/FangcunMount/qs-server/internal/worker/infra/grpcclient"
	messaging "github.com/FangcunMount/qs-server/internal/worker/integration/messaging"
	"github.com/FangcunMount/qs-server/internal/worker/options"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/stretchr/testify/require"
)

// This is an inert cross-boundary contract, not a Broker/MySQL proof. A canceled
// original store context must be reached only after the real frozen wire and
// pause authority pass their guards; no socket or original business RPC is used.
func TestWorkerPausedFrozenRetryContract(t *testing.T) {
	source, err := catalog.Load("../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	o := options.NewOptions()
	o.RetryGovernance.AutomaticRetryEnabled = false
	manager, err := grpcclient.NewManager(&grpcclient.ManagerConfig{Endpoint: "127.0.0.1:1", Insecure: true, Timeout: time.Millisecond})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	require.NoError(t, manager.RegisterClients())
	c, err := container.NewContainer(o, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, nil, routes)
	require.NoError(t, err)
	c.InitializeRuntimeClients(container.ClientBundle{AnswerSheet: manager.AnswerSheetClient(), Internal: manager.InternalClient(), AssessmentIntake: manager.AssessmentIntakeClient(), EvaluationWorker: manager.EvaluationWorkerClient(), InterpretationAutomation: manager.InterpretationAutomationClient()})
	require.NoError(t, c.Initialize())
	t.Cleanup(c.Cleanup)
	evt := event.New(catalog.EvaluationRetryRequested, "Assessment", "1", eventpayload.EvaluationRequestedData{OrgID: 7, AssessmentID: 1, ExpectedAttempt: 2, AttemptOrigin: "automatic", ModelCode: "fixture", RequestedAt: time.Now().UTC()})
	plan, err := txplan.Freeze(txport.Request{Key: "inert-frozen-wire", InputHash: sha256.Sum256([]byte("input")), Artifact: []byte("fixture"), Events: []event.DomainEvent{evt}}, routes, "qs.mysql", "m3-1")
	require.NoError(t, err)
	wire := plan.Input().Branches[0].Wire
	envelope, recognized, err := legacy.Decode(wire)
	require.NoError(t, err)
	require.True(t, recognized)
	_, pauseErr := c.DispatchEvent(t.Context(), envelope.Metadata["event_type"], envelope.Payload)
	require.ErrorIs(t, pauseErr, eventruntime.ErrAutomaticRetryPaused)
	group, err := rmtransport.ConsumerGroupV2("qs.evaluation.lifecycle", "qs-worker")
	require.NoError(t, err)
	bindings := []rmtransport.RouteBinding{}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		physical, e := rmtransport.PhysicalTopicV2("qs.evaluation.lifecycle", rmtransport.TransactionMessageType, owner, "m3-1")
		require.NoError(t, e)
		bindings = append(bindings, rmtransport.RouteBinding{Topic: "qs.evaluation.lifecycle", Subscription: "qs-worker", PhysicalTopic: physical, ConsumerGroup: group, MessageType: rmtransport.TransactionMessageType, Owner: owner, RouteRevision: "m3-1"})
	}
	o.MySQL.Host = "127.0.0.1:1"
	o.MySQL.Database = "inert_only"
	original, err := messaging.NewMySQLRetryEventHoldStore(o.MySQL, "rocketmq")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, original.Close()) })
	store, err := messaging.NewRocketMQDeferredStore(original, "qs-worker", 8, bindings)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = store.HoldDelivery(ctx, rmtransport.Received{Provider: "rocketmq", ID: envelope.UUID, TransportID: "inert-physical", Topic: "qs.evaluation.lifecycle", Channel: "qs-worker", PhysicalTopic: plan.Input().Branches[0].PhysicalTopic, Wire: wire, Payload: envelope.Payload, Metadata: envelope.Metadata, Attempts: 1}, catalog.EvaluationRetryRequested, pauseErr)
	require.ErrorIs(t, err, context.Canceled, "original root cancellation is reached only after frozen delivery/owner guards")
}
