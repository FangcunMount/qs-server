package testeestore

import (
	"strings"
	"testing"
	"time"
)

func manifestFixture(t *testing.T) (*Manifest, []MigrationItem) {
	t.Helper()
	now := time.Now()
	items := []MigrationItem{{MigrationID: "test", TesteeID: 1, OrgID: 7, BeforeVersion: 1, AppliedVersion: 2, TargetStoreID: ptr(2), HistoryID: ptr(3), Disposition: "candidate"}, {MigrationID: "test", TesteeID: 2, OrgID: 7, BeforeVersion: 1, AppliedVersion: 1, Disposition: "deferred", Reason: "no_management_relation"}}
	digest, err := ItemsHash(items)
	if err != nil {
		t.Fatal(err)
	}
	return &Manifest{MigrationID: "test", State: "applied", BeforeHash: strings.Repeat("a", 64), AfterHash: strings.Repeat("b", 64), ItemsHash: digest, ItemCount: 2, ActorID: 9, CreatedAt: now, CompletedAt: &now}, items
}
func TestManifestDistinguishesHistoryFromCurrentDrift(t *testing.T) {
	m, items := manifestFixture(t)
	state, err := Classify("test", m, items, m.AfterHash)
	if err != nil || state.State != "applied_unchanged" {
		t.Fatalf("unchanged: %+v %v", state, err)
	}
	state, err = Classify("test", m, items, strings.Repeat("c", 64))
	if err != nil || state.State != "applied_drifted" || state.NextAction != "review_drift" {
		t.Fatalf("drift: %+v %v", state, err)
	}
	now := time.Now()
	m.State = "rolled_back"
	m.RollbackHash = m.AfterHash
	m.RolledBackAt = &now
	state, err = Classify("test", m, items, m.AfterHash)
	if err != nil || state.State != "rolled_back" {
		t.Fatalf("rollback: %v", err)
	}
}
func TestManifestRejectsCorruptionBeforeReportingDrift(t *testing.T) {
	m, items := manifestFixture(t)
	items[0].TargetStoreID = ptr(999)
	state, err := Classify("test", m, items, strings.Repeat("c", 64))
	if err == nil || state.State != "invalid" {
		t.Fatal("tampered manifest treated as drift")
	}
}
func TestDeferredItemsNeverAuthorizeAssignment(t *testing.T) {
	m, items := manifestFixture(t)
	items[1].TargetStoreID = ptr(20)
	m.ItemsHash, _ = ItemsHash(items)
	if _, err := Classify("test", m, items, m.AfterHash); err == nil {
		t.Fatal("deferred item may change ownership")
	}
}
func TestItemChecksumIgnoresQueryOrderButRejectsDuplicates(t *testing.T) {
	_, items := manifestFixture(t)
	a, _ := ItemsHash(items)
	items[0], items[1] = items[1], items[0]
	b, _ := ItemsHash(items)
	if a != b {
		t.Fatal("unstable checksum")
	}
	if _, err := ItemsHash(append(items, items[0])); err == nil {
		t.Fatal("duplicate accepted")
	}
}
