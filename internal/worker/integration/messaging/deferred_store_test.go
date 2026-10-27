package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/deliveryevidence"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/stretchr/testify/require"
)

func deferredRoutes(t *testing.T) []rmtransport.RouteBinding {
	t.Helper()
	var routes []rmtransport.RouteBinding
	group, err := rmtransport.ConsumerGroupV2("qs.evaluation.lifecycle", "qs-worker")
	require.NoError(t, err)
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		physical, err := rmtransport.PhysicalTopicV2("qs.evaluation.lifecycle", rmtransport.TransactionMessageType, owner, "m3-1")
		require.NoError(t, err)
		routes = append(routes, rmtransport.RouteBinding{Topic: "qs.evaluation.lifecycle", Subscription: "qs-worker", PhysicalTopic: physical, ConsumerGroup: group, MessageType: rmtransport.TransactionMessageType, Owner: owner, RouteRevision: "m3-1"})
	}
	return routes
}

func deferredFixture(t *testing.T) (*DeferredDeliveryStore, sqlmock.Sqlmock, rmtransport.Received) {
	t.Helper()
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		mock.ExpectClose()
		require.NoError(t, pool.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	routes := deferredRoutes(t)
	store, err := NewRocketMQDeferredStore(&mysqlRetryEventHoldStore{db: pool, provider: "rocketmq", policy: retrygovernance.DefaultOutboxPolicy}, "qs-worker", 8, routes)
	require.NoError(t, err)
	payload := []byte(`{"id":"event-1","eventType":"evaluation.retry.requested","data":{"org_id":7,"attempt_origin":"automatic","expected_attempt":2}}`)
	metadata := map[string]string{"event_type": eventcatalog.EvaluationRetryRequested}
	wire, err := legacy.Encode(legacy.Envelope{UUID: "event-1", Metadata: metadata, Payload: payload}, legacy.Revision2)
	require.NoError(t, err)
	envelope, recognized, err := legacy.Decode(wire)
	require.NoError(t, err)
	require.True(t, recognized)
	message := rmtransport.Received{Provider: "rocketmq", ID: "event-1", TransportID: "broker-1", PhysicalTopic: routes[0].PhysicalTopic, Topic: routes[0].Topic, Channel: "qs-worker", Payload: envelope.Payload, Wire: wire, Metadata: envelope.Metadata, Attempts: 1, Timestamp: 123}
	require.NoError(t, mock.ExpectationsWereMet(), "constructors performed IO")
	return store, mock, message
}

func originalEvidenceRows(t *testing.T, m rmtransport.Received) *sqlmock.Rows {
	t.Helper()
	metadata, err := json.Marshal(m.Metadata)
	require.NoError(t, err)
	return sqlmock.NewRows([]string{"provider", "topic_name", "channel_name", "transport_message_id", "application_id", "physical_topic", "original_wire", "metadata_bytes"}).AddRow(m.Provider, m.Topic, m.Channel, m.TransportID, m.ID, m.PhysicalTopic, m.Wire, metadata)
}

func expectOriginalEvidence(t *testing.T, mock sqlmock.Sqlmock, m rmtransport.Received, write bool) {
	t.Helper()
	identity, err := eventtransport.RocketMQDeliveryIdentity(m)
	require.NoError(t, err)
	if write {
		metadata, err := json.Marshal(m.Metadata)
		require.NoError(t, err)
		mock.ExpectExec("INSERT INTO event_delivery_transport_evidence").WithArgs(identity[:], m.Provider, m.Topic, m.Channel, m.TransportID, m.ID, m.PhysicalTopic, m.Wire, metadata).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("FROM event_delivery_transport_evidence WHERE identity_hash=\\? FOR UPDATE").WithArgs(identity[:]).WillReturnRows(originalEvidenceRows(t, m))
	}
	mock.ExpectQuery("FROM event_delivery_transport_evidence WHERE identity_hash=\\? FOR SHARE").WithArgs(identity[:]).WillReturnRows(originalEvidenceRows(t, m))
}

func TestDeferredHoldOriginalEvidenceAndHoldShareOneCommit(t *testing.T) {
	for _, commitFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "committed", true: "unknown_commit"}[commitFails], func(t *testing.T) {
			store, mock, m := deferredFixture(t)
			identity, err := eventtransport.RocketMQDeliveryIdentity(m)
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO retry_event_hold").WithArgs(m.ID, m.ID, int64(7), m.Provider, m.Topic, m.Channel, string(m.Payload), m.Attempts, "automatic_retry_paused", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), identity[:], m.Timestamp).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectQuery("SELECT protocol_version,event_id,message_id,payload_json,transport_identity_hash").WithArgs(m.Provider, m.Topic, m.Channel, m.ID).WillReturnRows(sqlmock.NewRows([]string{"version", "event_id", "message_id", "payload", "identity"}).AddRow(2, m.ID, m.ID, string(m.Payload), identity[:]))
			expectOriginalEvidence(t, mock, m, true)
			want := errors.New("commit response unavailable")
			if commitFails {
				mock.ExpectCommit().WillReturnError(want)
			} else {
				mock.ExpectCommit()
			}
			err = store.HoldDelivery(t.Context(), m, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused)
			if commitFails {
				require.ErrorIs(t, err, want)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeferredHoldRefusesLegacyRowWithoutAdoption(t *testing.T) {
	store, mock, m := deferredFixture(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO retry_event_hold").WillReturnResult(sqlmock.NewResult(1, 0))
	mock.ExpectQuery("SELECT protocol_version,event_id,message_id,payload_json,transport_identity_hash").WillReturnRows(sqlmock.NewRows([]string{"version", "event_id", "message_id", "payload", "identity"}).AddRow(1, m.ID, m.ID, string(m.Payload), nil))
	mock.ExpectRollback()
	require.Error(t, store.HoldDelivery(t.Context(), m, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused))
}

func expectDeferredClaim(t *testing.T, mock sqlmock.Sqlmock, m rmtransport.Received, count int, grant, used any) {
	t.Helper()
	identity, err := eventtransport.RocketMQDeliveryIdentity(m)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("FROM retry_event_hold WHERE provider='rocketmq' AND channel_name=\\? AND protocol_version=2").WithArgs("qs-worker", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "event", "message", "payload", "identity", "original_attempt", "timestamp", "resume_count", "grant", "used"}).AddRow(1, m.ID, m.ID, string(m.Payload), identity[:], m.Attempts, m.Timestamp, count, grant, used))
	expectOriginalEvidence(t, mock, m, false)
}

func TestDeferredClaimPersistsAttemptsAndUsesEachManualGrantOnce(t *testing.T) {
	for _, grantNew := range []bool{false, true} {
		t.Run(map[bool]string{false: "spent_grant", true: "new_grant"}[grantNew], func(t *testing.T) {
			store, mock, m := deferredFixture(t)
			now := time.Now().UTC()
			used := "grant-1"
			grant := "grant-1"
			if grantNew {
				grant = "grant-2"
			}
			expectDeferredClaim(t, mock, m, 8, grant, used)
			if !grantNew {
				mock.ExpectExec("SET status='failed',retry_disposition='manual_required'.*deferred_delivery_budget_exhausted").WithArgs(now, 1).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
				item, err := store.claim(t.Context(), now, time.Minute)
				require.NoError(t, err)
				require.Nil(t, item)
			} else {
				mock.ExpectExec("SET status='consuming'").WithArgs(sqlmock.AnyArg(), now.Add(time.Minute), 9, grant, now, 1, 8).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
				item, err := store.claim(t.Context(), now, time.Minute)
				require.NoError(t, err)
				require.NotNil(t, item)
				require.Equal(t, 9, item.ResumeCount)
				require.Equal(t, 9, item.Attempt)
				require.True(t, item.Manual)
				require.Equal(t, m.Wire, item.Message.Wire)
				require.Equal(t, m.Payload, item.Message.Payload)
				require.Equal(t, m.Timestamp, item.Message.Timestamp)
			}
		})
	}
}

func TestDeferredClaimUnknownCommitNeverGrantsExecution(t *testing.T) {
	store, mock, m := deferredFixture(t)
	now := time.Now().UTC()
	expectDeferredClaim(t, mock, m, 0, nil, nil)
	mock.ExpectExec("SET status='consuming'").WithArgs(sqlmock.AnyArg(), now.Add(time.Minute), 1, nil, now, 1, 0).WillReturnResult(sqlmock.NewResult(0, 1))
	want := errors.New("commit acknowledgement lost")
	mock.ExpectCommit().WillReturnError(want)
	item, err := store.claim(t.Context(), now, time.Minute)
	require.ErrorIs(t, err, want)
	require.Nil(t, item)
}

func TestDeferredExpiredClaimCannotCompleteOrRenew(t *testing.T) {
	store, mock, m := deferredFixture(t)
	item := &deferredItem{ID: 1, Token: "previous-owner", Message: m}
	now := time.Now().UTC()
	mock.ExpectExec("SET status='consumed'.*claim_token=\\? AND claim_expires_at>\\?").WithArgs(now, now, 1, item.Token, now).WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, store.complete(t.Context(), item, now), ErrDeferredClaimLost)
	mock.ExpectExec("SET claim_expires_at=.*claim_token=\\? AND claim_expires_at>\\?").WithArgs(now.Add(time.Minute), now, 1, item.Token, now).WillReturnResult(sqlmock.NewResult(0, 0))
	require.ErrorIs(t, store.renew(t.Context(), item, now, time.Minute), ErrDeferredClaimLost)
}

func TestDeferredPauseRestoresUnusedTechnicalAuthorization(t *testing.T) {
	store, mock, m := deferredFixture(t)
	now := time.Now().UTC()
	item := &deferredItem{ID: 1, Token: "owned", Message: m, ResumeCount: 9, Attempt: 9, Manual: true, manualRequestID: "grant-2", previousManualUse: sql.NullString{String: "grant-1", Valid: true}}
	mock.ExpectExec("SET status='failed'.*used_manual_replay_request_id=").WithArgs("automatic", now.Add(5*time.Second), string(deferredPaused), 8, "grant-1", now, 1, "owned", now).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, store.failed(t.Context(), item, now, deferredPaused))
}

func TestDeferredStoreRefusesInvalidRoutesSourcesAndImplicitSchema(t *testing.T) {
	store, mock, m := deferredFixture(t)
	bad := deferredRoutes(t)
	bad[0].PhysicalTopic = "unreviewed"
	_, err := NewRocketMQDeferredStore(store.original, "qs-worker", 8, bad)
	require.Error(t, err)
	m.PhysicalTopic = deferredRoutes(t)[1].PhysicalTopic
	require.Error(t, store.HoldDelivery(t.Context(), m, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT protocol_version,transport_identity_hash,received_timestamp,consumed_at,used_manual_replay_request_id FROM retry_event_hold LIMIT 0")).WillReturnError(errors.New("explicit migration absent"))
	require.Error(t, store.Validate(context.Background()))
}

func TestReadableDeferredStoreKeepsOldDeliveryIdentityWithoutOldSubscription(t *testing.T) {
	oldStore, mock, message := deferredFixture(t)
	_ = mock
	history := deferredRoutes(t)
	active := append([]rmtransport.RouteBinding(nil), history...)
	for i := range active {
		r := &active[i]
		r.NamingScheme = rmtransport.ReadableNamingV1
		var err error
		r.PhysicalTopic, err = rmtransport.PhysicalTopicV3(r.Topic, r.MessageType, r.Owner, r.RouteRevision)
		require.NoError(t, err)
		r.ConsumerGroup, err = rmtransport.ConsumerGroupV3(r.Topic, r.Subscription)
		require.NoError(t, err)
	}
	store, err := NewRocketMQDeferredStoreWithHistory(oldStore.original, "qs-worker", 8, active, history)
	require.NoError(t, err)
	_, err = store.eventType(message)
	require.NoError(t, err, "old deferred delivery must keep its exact original identity")
	require.ErrorIs(t, store.HoldDelivery(context.Background(), message, eventcatalog.EvaluationRetryRequested, eventruntime.ErrAutomaticRetryPaused), ErrHistoricalDelivery, "historical read permission created a new hold")
	require.NoError(t, mock.ExpectationsWereMet(), "historical hold attempted database IO")
	resolver, err := rmtransport.NewRouteResolver(active, history)
	require.NoError(t, err)
	subscribed, err := resolver.SubscriptionRoutes("qs.evaluation.lifecycle", "qs-worker")
	require.NoError(t, err)
	require.Len(t, subscribed, 2)
	for _, r := range subscribed {
		require.Equal(t, rmtransport.ReadableNamingV1, r.NamingScheme)
	}
	require.Equal(t, history[0].PhysicalTopic, message.PhysicalTopic)
}
