package compatibilityretirementstop

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func stoppedDependentsFixture() ([]Container, []actualContainer, map[string]bool, string) {
	original := descriptorFixture().Containers
	api := original[0]
	api.ID, api.Image = strings.Repeat("3", 64), "sha256:"+strings.Repeat("4", 64)
	api.Command = []string{"--config=/app/configs/apiserver.prod.yaml", "--migration.enabled=true"}
	dependent := original[1]
	dependent.Running = false
	return original, []actualContainer{{Container: dependent, Status: "exited"}, {Container: api, PID: 42, Status: "running"}}, map[string]bool{dependent.ID: true}, api.ID
}

// These expected-catalog comparisons do not execute Docker or construct a live
// Lease/Window. The public method separately reads the actual full catalog and
// original root-protected completed stop journals under the original budget.
func TestStoppedDependentsPreservesOriginalAndRejectsCatalogDrift(t *testing.T) {
	base, actual, stopped, api := stoppedDependentsFixture()
	frozen := append([]Container(nil), base...)
	if got, e := stoppedDependents(base, actual, stopped, api); e != nil || len(got) != 1 || got[0].ID != base[1].ID {
		t.Fatal("exact stopped dependent and independently owned replacement API rejected")
	}
	if !reflect.DeepEqual(base, frozen) {
		t.Fatal("original lease baseline mutated")
	}
	changes := map[string]func([]actualContainer){
		"dependent_cid":        func(v []actualContainer) { v[0].ID = strings.Repeat("9", 64) },
		"dependent_image":      func(v []actualContainer) { v[0].Image = "sha256:" + strings.Repeat("9", 64) },
		"dependent_time":       func(v []actualContainer) { v[0].StartedAt = "changed" },
		"dependent_running":    func(v []actualContainer) { v[0].Running = true },
		"dependent_pid":        func(v []actualContainer) { v[0].PID = 7 },
		"dependent_paused":     func(v []actualContainer) { v[0].Paused = true },
		"dependent_restarting": func(v []actualContainer) { v[0].Restarting = true },
		"dependent_dead":       func(v []actualContainer) { v[0].Dead = true },
		"dependent_oom":        func(v []actualContainer) { v[0].OOMKilled = true },
		"dependent_exit":       func(v []actualContainer) { v[0].ExitCode = 137 },
		"foreign_component":    func(v []actualContainer) { v[0].Component = "qs-worker" },
		"residual_old_api":     func(v []actualContainer) { v[1].ID = base[0].ID },
		"foreign_api":          func(v []actualContainer) { v[1].ID = strings.Repeat("8", 64) },
		"api_config":           func(v []actualContainer) { v[1].Command = []string{"--config=other"} },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			v := append([]actualContainer(nil), actual...)
			change(v)
			if _, e := stoppedDependents(base, v, stopped, api); e == nil {
				t.Fatal("catalog drift was hidden by API filtering")
			}
		})
	}
	for name, v := range map[string][]actualContainer{"missing": actual[:1], "extra_old_api": append(append([]actualContainer(nil), actual...), actualContainer{Container: base[0]})} {
		t.Run(name, func(t *testing.T) {
			if _, e := stoppedDependents(base, v, stopped, api); e == nil {
				t.Fatal("wrong full relevant-container scope accepted")
			}
		})
	}
	if _, e := stoppedDependents(base, actual, nil, api); e == nil {
		t.Fatal("missing actual successful stop accepted")
	}
	if _, e := stoppedDependents(base, actual, stopped, base[0].ID); e == nil {
		t.Fatal("original API adopted as B transition")
	}
}

func TestStoppedDependentReadbackCannotImportLease(t *testing.T) {
	for _, l := range []*Lease{nil, {}, {baseline: descriptorFixture().Containers}} {
		if l.CheckStoppedDependents(context.Background(), strings.Repeat("3", 64)) == nil {
			t.Fatal("unissued lease reached native readback")
		}
	}
}
