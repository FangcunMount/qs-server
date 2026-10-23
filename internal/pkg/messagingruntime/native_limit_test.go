package messagingruntime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestNativeLimitConstructorsBorrowPrimariesWithoutIO(t *testing.T) {
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "original-primary", WriterEpoch: "m3-epoch"}
	store, err := NewSQLNativeLimits(new(sql.DB), binding, "m3-1")
	if err != nil || store.Binding() != binding {
		t.Fatal("inert primary rejected", err)
	}
	for _, b := range []transaction.Binding{{}, {Owner: "unknown", StoreID: "primary", WriterEpoch: "epoch"}, {Owner: "qs.mongo", StoreID: "primary", WriterEpoch: "epoch"}} {
		if _, err = NewSQLNativeLimits(new(sql.DB), b, "m3-1"); !errors.Is(err, transaction.ErrBinding) {
			t.Fatal("SQL owner mismatch accepted")
		}
	}
	if _, err = NewSQLNativeLimits(new(sql.DB), binding, "other-route"); !errors.Is(err, transaction.ErrBinding) {
		t.Fatal("unbound route accepted")
	}
	client := new(mongo.Client) // zero client, like the zero SQL pool: no monitors/dials
	binding.Owner = "qs.mongo"
	document, err := NewMongoNativeLimits(client.Database("owned"), binding, "m3-1")
	if err != nil || document.Binding() != binding {
		t.Fatal("inert Mongo primary rejected", err)
	}
	var absent *SQLNativeLimits
	if absent.Binding().Valid() {
		t.Fatal("typed nil binding accepted")
	}
	if _, err = absent.ReadNativeLimit(context.Background()); !errors.Is(err, transaction.ErrBinding) {
		t.Fatal("typed nil reader accepted")
	}
}
func TestNativeNoticeIdentityPreservesEveryOriginalByte(t *testing.T) {
	b := transaction.Binding{Owner: "qs.mysql", StoreID: "A", WriterEpoch: "bc"}
	e := sha256.Sum256([]byte("owned export"))
	key := nativeNoticeKey(b, "m3-1", e)
	for _, changed := range []transaction.Binding{{Owner: b.Owner, StoreID: "a", WriterEpoch: b.WriterEpoch}, {Owner: b.Owner, StoreID: "Ab", WriterEpoch: "c"}, {Owner: "iam.mysql", StoreID: b.StoreID, WriterEpoch: b.WriterEpoch}} {
		if nativeNoticeKey(changed, "m3-1", e) == key {
			t.Fatal("original identity collapsed")
		}
	}
	altered := e
	altered[0] ^= 1
	if nativeNoticeKey(b, "m3-1", altered) == key || nativeNoticeKey(b, "other", e) == key {
		t.Fatal("audit or routing identity lost")
	}
}
func TestNativeLimitProtectionRequiresResolutionThenNamedReview(t *testing.T) {
	now := time.Now()
	freshness := 12 * time.Second
	var p protocolProtection
	p.observe(now, protocolSample{nativeBlocked: true}, nil, false, false, freshness)
	if p.allowed(now, freshness) || p.reason != "native_transaction_limit" || p.generation != 1 {
		t.Fatal("native notice did not latch")
	}
	if !errors.Is(p.review(now, freshness, p.generation, "owned-review", true, false), ErrProtocolProtection) {
		t.Fatal("active native notice reviewed open")
	}
	p.observe(now.Add(time.Second), protocolSample{}, nil, false, false, freshness)
	if p.allowed(now.Add(time.Second), freshness) {
		t.Fatal("healthy sample auto-cleared native episode")
	}
	if err := p.review(now.Add(time.Second), freshness, p.generation, "owned-review", true, false); err != nil {
		t.Fatal(err)
	}
	if !p.allowed(now.Add(time.Second), freshness) {
		t.Fatal("resolved healthy reviewed episode remained closed")
	}
}
func TestNativeNoticeStillLatchesWhenOtherObservationFails(t *testing.T) {
	var p protocolProtection
	now := time.Now()
	p.observe(now, protocolSample{nativeBlocked: true}, errors.New("owned unavailable"), false, false, time.Second)
	if p.reason != "native_transaction_limit" || p.available || p.allowed(now, time.Second) {
		t.Fatal("observation error hid an already observed native block")
	}
}
