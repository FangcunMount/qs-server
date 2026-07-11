package generation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestGenerationLifecycleRequiresCurrentRunAndPreservesIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	key := Key{OutcomeID: meta.FromUint64(9), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")}
	g, err := New(meta.FromUint64(1), key, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Begin(meta.FromUint64(11), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := g.Begin(meta.FromUint64(12), now.Add(2*time.Second)); err == nil {
		t.Fatal("generating generation accepted a concurrent run")
	}
	if err := g.Fail(meta.FromUint64(12), now.Add(2*time.Second)); err == nil {
		t.Fatal("generation accepted failure from a non-current run")
	}
	if err := g.Fail(meta.FromUint64(11), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := g.Begin(meta.FromUint64(12), now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := g.Succeed(meta.FromUint64(12), meta.FromUint64(21), now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if g.Status() != StatusGenerated || g.ReportID().Uint64() != 21 || g.Key() != key {
		t.Fatalf("generation = status:%s report:%s key:%#v", g.Status(), g.ReportID(), g.Key())
	}
	if err := g.Fail(meta.FromUint64(12), now.Add(5*time.Second)); err == nil {
		t.Fatal("generated generation became failed")
	}
}

func TestGenerationRejectsIncompleteIdentity(t *testing.T) {
	now := time.Now()
	if _, err := New(0, Key{}, now); err == nil {
		t.Fatal("expected incomplete generation identity to be rejected")
	}
	if _, err := New(meta.FromUint64(1), Key{OutcomeID: meta.FromUint64(2), ReportType: policy.ReportTypeStandard}, now); err == nil {
		t.Fatal("expected missing template version to be rejected")
	}
}

func TestRestoreGenerationRejectsInconsistentState(t *testing.T) {
	now := time.Now()
	key := Key{OutcomeID: meta.FromUint64(9), ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("v1")}
	if _, err := Restore(RestoreInput{ID: meta.FromUint64(1), Key: key, Status: StatusGenerated, Version: 2, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("generated generation restored without run and report references")
	}
	restored, err := Restore(RestoreInput{ID: meta.FromUint64(1), Key: key, Status: StatusFailed, LatestRunID: meta.FromUint64(2), Version: 2, CreatedAt: now, UpdatedAt: now})
	if err != nil || restored.Status() != StatusFailed || restored.Version() != 2 {
		t.Fatalf("restore = generation:%#v err:%v", restored, err)
	}
}
