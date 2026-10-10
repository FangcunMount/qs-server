package compatibilityretirementbackup

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestTargetRecoveryOpaqueAndAuthority(t *testing.T) {
	if _, e := RecoverTargets(context.Background(), nil); e != ErrRecoveryBinding {
		t.Fatal("nil plan accepted")
	}
	zero := &TargetRecoveryPlan{}
	if _, e := RecoverTargets(context.Background(), zero); e != ErrRecoveryBinding {
		t.Fatal("zero plan accepted")
	}
	p := &TargetRecoveryPlan{}
	p.self = p
	p.observations[0].State = "missing_unproven"
	if _, e := RecoverTargets(context.Background(), p); e != ErrRecoveryAuthority {
		t.Fatal("unproven absence authorized")
	}
	p.mu.Lock()
	copy := &TargetRecoveryPlan{}
	reflect.ValueOf(copy).Elem().Set(reflect.ValueOf(p).Elem())
	p.mu.Unlock()
	if _, e := RecoverTargets(context.Background(), copy); e != ErrRecoveryBinding {
		t.Fatal("copied plan accepted")
	}
	if p.validDrop(-1) || p.validDrop(4) {
		t.Fatal("out of range proof")
	}
	if _, e := p.MarshalJSON(); e != ErrSerialization {
		t.Fatal("plan serializable")
	}
}
func TestTargetRecoveryJournalRealFiles(t *testing.T) {
	t.Run("sealed and no adoption", func(t *testing.T) {
		dir := targetRecoveryTestDir(t)
		j, e := newTargetJournal(dir, TargetRecoveryRequest{})
		if e != nil {
			t.Fatal(e)
		}
		if j.validate() != nil {
			t.Fatal("live journal invalid")
		}
		if _, e = newTargetJournal(dir, TargetRecoveryRequest{}); e != ErrRecoveryJournal {
			t.Fatal("adopted existing journal")
		}
		if _, e = j.write("binding", struct{}{}); e != ErrRecoveryUnknown {
			t.Fatal("overwrote immutable record")
		}
	})
	t.Run("unknown extra", func(t *testing.T) {
		dir := targetRecoveryTestDir(t)
		j, e := newTargetJournal(dir, TargetRecoveryRequest{})
		if e != nil {
			t.Fatal(e)
		}
		if os.WriteFile(filepath.Join(dir, "unknown"), []byte("unknown"), 0600) != nil {
			t.Fatal("fixture")
		}
		if j.validate() != ErrRecoveryJournal {
			t.Fatal("ignored unknown file")
		}
	})
	t.Run("tampered bytes", func(t *testing.T) {
		dir := targetRecoveryTestDir(t)
		j, e := newTargetJournal(dir, TargetRecoveryRequest{})
		if e != nil {
			t.Fatal(e)
		}
		path := filepath.Join(dir, "target-recovery-binding.json")
		if os.WriteFile(path, []byte("{}"), 0600) != nil {
			t.Fatal("fixture")
		}
		if j.validate() != ErrRecoveryJournal {
			t.Fatal("ignored changed record")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		dir := targetRecoveryTestDir(t)
		j, e := newTargetJournal(dir, TargetRecoveryRequest{})
		if e != nil {
			t.Fatal(e)
		}
		if os.Link(filepath.Join(dir, "target-recovery-binding.json"), filepath.Join(t.TempDir(), "copy")) != nil {
			t.Fatal("fixture")
		}
		if j.validate() != ErrRecoveryJournal {
			t.Fatal("accepted hardlink")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		dir := targetRecoveryTestDir(t)
		j, e := newTargetJournal(dir, TargetRecoveryRequest{})
		if e != nil {
			t.Fatal(e)
		}
		path := filepath.Join(dir, "target-recovery-binding.json")
		if os.Rename(path, path+".original") != nil || os.Symlink(path+".original", path) != nil {
			t.Fatal("fixture")
		}
		if j.validate() != ErrRecoveryJournal {
			t.Fatal("accepted symlink")
		}
	})
}
func TestTargetRecoveryHeadAndConstraints(t *testing.T) {
	for _, v := range []struct {
		actual, original uint64
		want             bool
	}{{99, 99, true}, {100, 99, false}, {101, 99, false}, {0, 0, false}, {0, ^uint64(0), false}} {
		if targetSupportedHead(v.actual, v.original) != v.want {
			t.Fatal("unsupported head rule")
		}
	}
	good := "CREATE TABLE `ai_bridge_commands` (FOREIGN KEY (`request_id`) REFERENCES `ai_bridge_requests` (`request_id`))"
	if targetRecoveryForeignKeys(good) != nil {
		t.Fatal("actual parent FK rejected")
	}
	for _, ddl := range []string{"REFERENCES `outside`.`parent` (`id`)", "REFERENCES `domain_event_outbox` (`id`)", "REFERENCES parent (id)"} {
		if targetRecoveryForeignKeys(ddl) == nil {
			t.Fatal("unsupported FK accepted")
		}
	}
	str := func(s string) *string { return &s }
	empty := [][]*string{}
	defs := map[string]any{"constraints": empty, "inbound_constraints": empty, "triggers": empty, "table:rm_outbox": "kept"}
	a, e := targetSQLNonTarget(defs, "owner")
	if e != nil {
		t.Fatal(e)
	}
	defs["table:domain_event_outbox"] = "removed"
	b, e := targetSQLNonTarget(defs, "owner")
	if e != nil || a != b {
		t.Fatal("target affects non-target hash")
	}
	defs["inbound_constraints"] = [][]*string{{str("other_schema"), str("ai_bridge_commands"), str("fk"), str("owner"), str("domain_event_outbox"), str("id"), str("id")}}
	if _, e = targetSQLNonTarget(defs, "owner"); e != ErrStructure {
		t.Fatal("cross schema same-name dependency hidden")
	}
	defs["inbound_constraints"] = empty
	defs["triggers"] = [][]*string{{str("danger"), str("domain_event_outbox"), str("body")}}
	if _, e = targetSQLNonTarget(defs, "owner"); e != ErrStructure {
		t.Fatal("target trigger hidden")
	}
}
func TestTargetRecoverySummaryDefensive(t *testing.T) {
	p := &TargetRecoveryPlan{}
	p.self = p
	p.observations[0].State = "missing_unproven"
	s := p.Summary()
	s.Targets[0].State = "restored_exact"
	if p.Summary().Targets[0].State != "missing_unproven" || s.ProductionAuthorityIntegrated || s.WholeWriterFenceProven || s.DropReady {
		t.Fatal("summary granted authority or aliased")
	}
}

func targetRecoveryTestDir(t *testing.T) string {
	t.Helper()
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil || os.Chmod(dir, 0700) != nil {
		t.Fatal("private test directory unavailable")
	}
	return dir
}
