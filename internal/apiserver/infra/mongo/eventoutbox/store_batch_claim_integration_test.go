package eventoutbox

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func openMongoOutboxTestStore(t *testing.T) *Store {
	t.Helper()

	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if uri == "" {
		skipMongoOutboxContract(t)
	}
	dbName := os.Getenv("QS_SERVER_TEST_MONGO_DB")
	if dbName == "" {
		dbName = "qs_server_contract_test"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect mongo test db: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("ping mongo test db: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})

	store, err := NewStoreWithTopicResolver(client.Database(dbName), testOutboxTopicResolver())
	if err != nil {
		t.Fatalf("NewStoreWithTopicResolver: %v", err)
	}
	return store
}

func skipMongoOutboxContract(t *testing.T) {
	t.Helper()
	message := "QS_SERVER_TEST_MONGO_URI is not set; skipping Mongo outbox batch claim contract tests. " +
		"Run: QS_SERVER_TEST_MONGO_URI='mongodb://127.0.0.1:27017' QS_SERVER_TEST_MONGO_DB='qs_server_contract_test' " +
		"go test ./internal/apiserver/infra/mongo/eventoutbox -run BatchClaim -v"
	fmt.Fprintln(os.Stderr, message)
	t.Skip(message)
}

func testOutboxTopicResolver() fakeTopicResolver {
	return fakeTopicResolver{
		topics: map[string]string{"sample.created": "sample.topic"},
		deliveries: map[string]eventcatalog.DeliveryClass{
			"sample.created": eventcatalog.DeliveryClassDurableOutbox,
		},
	}
}

func insertOutboxDocs(t *testing.T, store *Store, docs ...*OutboxPO) {
	t.Helper()
	if len(docs) == 0 {
		return
	}
	items := make([]interface{}, len(docs))
	for i, doc := range docs {
		items[i] = doc
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := store.coll.InsertMany(ctx, items); err != nil {
		t.Fatalf("InsertMany: %v", err)
	}
}

func buildSampleOutboxDoc(t *testing.T, store *Store, eventID string, now time.Time) *OutboxPO {
	t.Helper()
	evt := event.New("sample.created", "Sample", eventID, map[string]string{"id": eventID})
	docs, err := store.buildDocumentsAt([]event.DomainEvent{evt}, now)
	if err != nil {
		t.Fatalf("buildDocumentsAt: %v", err)
	}
	doc := docs[0]
	doc.EventID = eventID
	return doc
}

func outboxDocStatus(t *testing.T, store *Store, eventID string) OutboxPO {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var po OutboxPO
	if err := store.coll.FindOne(ctx, bson.M{"event_id": eventID}).Decode(&po); err != nil {
		t.Fatalf("FindOne %q: %v", eventID, err)
	}
	return po
}

func cleanupOutboxDocs(t *testing.T, store *Store, eventIDs ...string) {
	t.Helper()
	if len(eventIDs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = store.coll.DeleteMany(ctx, bson.M{"event_id": bson.M{"$in": eventIDs}})
}

func TestBatchClaimPendingDueClaimsSubset(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	prefix := fmt.Sprintf("batch-pending-%d", time.Now().UnixNano())
	eventIDs := make([]string, 10)
	docs := make([]*OutboxPO, 10)
	for i := range eventIDs {
		eventIDs[i] = fmt.Sprintf("%s-%d", prefix, i)
		docs[i] = buildSampleOutboxDoc(t, store, eventIDs[i], now.Add(-time.Duration(i)*time.Second))
	}
	t.Cleanup(func() { cleanupOutboxDocs(t, store, eventIDs...) })
	insertOutboxDocs(t, store, docs...)

	claimed, err := store.claimBatchByFilter(ctxBackground(t), bson.M{
		"status":          outboxcore.StatusPending,
		"next_attempt_at": bson.M{"$lte": now},
		"event_id":        bson.M{"$in": eventIDs},
	}, bson.D{{Key: "created_at", Value: 1}}, 5, now)
	if err != nil {
		t.Fatalf("claimBatchByFilter: %v", err)
	}
	if len(claimed) != 5 {
		t.Fatalf("claimed len = %d, want 5", len(claimed))
	}

	publishing := 0
	pending := 0
	for _, eventID := range eventIDs {
		po := outboxDocStatus(t, store, eventID)
		switch po.Status {
		case outboxcore.StatusPublishing:
			publishing++
			if po.ClaimToken == "" {
				t.Fatalf("event %q publishing without claim_token", eventID)
			}
		case outboxcore.StatusPending:
			pending++
		default:
			t.Fatalf("event %q status = %q, want publishing or pending", eventID, po.Status)
		}
	}
	if publishing != 5 || pending != 5 {
		t.Fatalf("publishing=%d pending=%d, want 5/5", publishing, pending)
	}
}

func TestBatchClaimSkipsFutureNextAttemptAt(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	eventID := fmt.Sprintf("batch-future-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupOutboxDocs(t, store, eventID) })

	doc := buildSampleOutboxDoc(t, store, eventID, now)
	doc.NextAttemptAt = now.Add(time.Hour)
	insertOutboxDocs(t, store, doc)

	claimed, err := store.claimBatchByFilter(ctxBackground(t), bson.M{
		"status":          outboxcore.StatusPending,
		"next_attempt_at": bson.M{"$lte": now},
		"event_id":        eventID,
	}, bson.D{{Key: "created_at", Value: 1}}, 1, now)
	if err != nil {
		t.Fatalf("claimBatchByFilter: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed len = %d, want 0", len(claimed))
	}
	po := outboxDocStatus(t, store, eventID)
	if po.Status != outboxcore.StatusPending {
		t.Fatalf("status = %q, want pending", po.Status)
	}
}

func TestBatchClaimFailedDue(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	eventID := fmt.Sprintf("batch-failed-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupOutboxDocs(t, store, eventID) })

	doc := buildSampleOutboxDoc(t, store, eventID, now.Add(-time.Minute))
	doc.Status = outboxcore.StatusFailed
	doc.LastError = "publish timeout"
	doc.AttemptCount = 1
	insertOutboxDocs(t, store, doc)

	claimed, err := store.claimDueByNextAttempt(ctxBackground(t), outboxcore.StatusFailed, 1, now)
	if err != nil {
		t.Fatalf("claimDueByNextAttempt: %v", err)
	}
	if len(claimed) != 1 || claimed[0].EventID != eventID {
		t.Fatalf("claimed = %#v, want event %q", claimed, eventID)
	}
	po := outboxDocStatus(t, store, eventID)
	if po.Status != outboxcore.StatusPublishing || po.ClaimToken == "" {
		t.Fatalf("po = %#v, want publishing with claim_token", po)
	}
}

func TestBatchClaimStalePublishingReclaims(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	staleBefore := now.Add(-store.publishingStaleFor)
	eventID := fmt.Sprintf("batch-stale-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupOutboxDocs(t, store, eventID) })

	doc := buildSampleOutboxDoc(t, store, eventID, now.Add(-2*store.publishingStaleFor))
	doc.Status = outboxcore.StatusPublishing
	doc.UpdatedAt = staleBefore.Add(-time.Minute)
	doc.ClaimToken = "old-token"
	insertOutboxDocs(t, store, doc)

	claimed, err := store.claimStalePublishing(ctxBackground(t), 1, now, staleBefore)
	if err != nil {
		t.Fatalf("claimStalePublishing: %v", err)
	}
	if len(claimed) != 1 || claimed[0].EventID != eventID {
		t.Fatalf("claimed = %#v, want event %q", claimed, eventID)
	}
	po := outboxDocStatus(t, store, eventID)
	if po.Status != outboxcore.StatusPublishing || po.ClaimToken == "" || po.ClaimToken == "old-token" {
		t.Fatalf("po = %#v, want publishing with new claim_token", po)
	}
	if !po.UpdatedAt.Equal(now) {
		t.Fatalf("updated_at = %s, want %s", po.UpdatedAt, now)
	}
}

func TestBatchClaimFreshPublishingSkipped(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	staleBefore := now.Add(-store.publishingStaleFor)
	eventID := fmt.Sprintf("batch-fresh-pub-%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupOutboxDocs(t, store, eventID) })

	doc := buildSampleOutboxDoc(t, store, eventID, now)
	doc.Status = outboxcore.StatusPublishing
	doc.UpdatedAt = now.Add(-time.Minute)
	doc.ClaimToken = "active-token"
	insertOutboxDocs(t, store, doc)

	claimed, err := store.claimStalePublishing(ctxBackground(t), 1, now, staleBefore)
	if err != nil {
		t.Fatalf("claimStalePublishing: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed len = %d, want 0", len(claimed))
	}
	po := outboxDocStatus(t, store, eventID)
	if po.ClaimToken != "active-token" {
		t.Fatalf("claim_token = %q, want active-token", po.ClaimToken)
	}
}

func TestClaimEventsByIDsClaimsOnlySpecifiedIDs(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	prefix := fmt.Sprintf("claim-by-id-%d", time.Now().UnixNano())
	allIDs := make([]string, 10)
	targetIDs := make([]string, 3)
	docs := make([]*OutboxPO, 10)
	for i := range allIDs {
		allIDs[i] = fmt.Sprintf("%s-%d", prefix, i)
		docs[i] = buildSampleOutboxDoc(t, store, allIDs[i], now.Add(-time.Duration(i)*time.Second))
		if i < 3 {
			targetIDs[i] = allIDs[i]
		}
	}
	t.Cleanup(func() { cleanupOutboxDocs(t, store, allIDs...) })
	insertOutboxDocs(t, store, docs...)

	claimed, err := store.ClaimEventsByIDs(ctxBackground(t), targetIDs, now)
	if err != nil {
		t.Fatalf("ClaimEventsByIDs: %v", err)
	}
	if len(claimed) != 3 {
		t.Fatalf("claimed len = %d, want 3", len(claimed))
	}
	for _, eventID := range allIDs[3:] {
		po := outboxDocStatus(t, store, eventID)
		if po.Status != outboxcore.StatusPending {
			t.Fatalf("event %q status = %q, want pending", eventID, po.Status)
		}
	}
}

func TestBatchClaimConcurrentNoDuplicateClaims(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	prefix := fmt.Sprintf("batch-concurrent-%d", time.Now().UnixNano())
	eventIDs := make([]string, 20)
	docs := make([]*OutboxPO, 20)
	for i := range eventIDs {
		eventIDs[i] = fmt.Sprintf("%s-%d", prefix, i)
		docs[i] = buildSampleOutboxDoc(t, store, eventIDs[i], now.Add(-time.Duration(i)*time.Second))
	}
	t.Cleanup(func() { cleanupOutboxDocs(t, store, eventIDs...) })
	insertOutboxDocs(t, store, docs...)

	filter := bson.M{
		"status":          outboxcore.StatusPending,
		"next_attempt_at": bson.M{"$lte": now},
		"event_id":        bson.M{"$in": eventIDs},
	}
	sort := bson.D{{Key: "created_at", Value: 1}}

	var wg sync.WaitGroup
	results := make([][]string, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			claimed, err := store.claimBatchByFilter(ctxBackground(t), filter, sort, 20, now)
			if err != nil {
				errs[idx] = err
				return
			}
			ids := make([]string, len(claimed))
			for j, item := range claimed {
				ids[j] = item.EventID
			}
			results[idx] = ids
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	seen := make(map[string]struct{})
	total := 0
	for _, ids := range results {
		total += len(ids)
		for _, eventID := range ids {
			if _, ok := seen[eventID]; ok {
				t.Fatalf("event %q claimed by multiple goroutines", eventID)
			}
			seen[eventID] = struct{}{}
		}
	}
	if total > len(eventIDs) {
		t.Fatalf("total claimed = %d, want <= %d", total, len(eventIDs))
	}
	if total == 0 {
		t.Fatalf("expected at least one claimed event")
	}
}

func ctxBackground(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestMarkEventsFailedBulkUpdatesMultipleRows(t *testing.T) {
	store := openMongoOutboxTestStore(t)
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	retryAt := now.Add(time.Minute)
	prefix := fmt.Sprintf("mark-failed-%d", time.Now().UnixNano())
	eventIDs := []string{
		fmt.Sprintf("%s-a", prefix),
		fmt.Sprintf("%s-b", prefix),
	}
	docs := []*OutboxPO{
		buildSampleOutboxDoc(t, store, eventIDs[0], now),
		buildSampleOutboxDoc(t, store, eventIDs[1], now),
	}
	for _, doc := range docs {
		doc.Status = outboxcore.StatusPublishing
		doc.ClaimToken = "claim-" + doc.EventID
	}
	t.Cleanup(func() { cleanupOutboxDocs(t, store, eventIDs...) })
	insertOutboxDocs(t, store, docs...)

	ctx := ctxBackground(t)
	err := store.MarkEventsFailed(ctx, []outboxport.FailedMark{
		{EventID: eventIDs[0], LastError: "publish timeout"},
		{EventID: eventIDs[1], LastError: "nsq unavailable"},
	}, retryAt)
	if err != nil {
		t.Fatalf("MarkEventsFailed: %v", err)
	}

	for i, eventID := range eventIDs {
		po := outboxDocStatus(t, store, eventID)
		if po.Status != outboxcore.StatusFailed {
			t.Fatalf("event %q status = %q, want failed", eventID, po.Status)
		}
		if po.AttemptCount != 1 {
			t.Fatalf("event %q attempt_count = %d, want 1", eventID, po.AttemptCount)
		}
		if !po.NextAttemptAt.Equal(retryAt) {
			t.Fatalf("event %q next_attempt_at = %s, want %s", eventID, po.NextAttemptAt, retryAt)
		}
		if po.ClaimToken != "" {
			t.Fatalf("event %q claim_token = %q, want cleared", eventID, po.ClaimToken)
		}
		wantError := []string{"publish timeout", "nsq unavailable"}[i]
		if po.LastError != wantError {
			t.Fatalf("event %q last_error = %q, want %q", eventID, po.LastError, wantError)
		}
	}
}
