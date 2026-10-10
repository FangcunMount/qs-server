//go:build integration

package compatibilityretirementbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"debug/buildinfo"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	buildversion "github.com/FangcunMount/qs-server/pkg/version"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// This opt-in runs the public Python caller, root-once source staging and the
// actual CLI Capture/stdio-restore producers. It mints no production permission.
// The mandatory CI selector fails rather than silently skipping prerequisites.
func publicCLIOptIn(enabled, required string) (bool, error) {
	if enabled == "1" {
		return true, nil
	}
	if required == "1" {
		return false, errors.New("public_cli_native_required_but_not_enabled")
	}
	return false, nil
}

func TestLifecyclePublicCLIOptInContract(t *testing.T) {
	for _, tc := range []struct {
		enabled, required string
		run, fail         bool
	}{{"", "", false, false}, {"", "1", false, true}, {"1", "1", true, false}, {"1", "", true, false}} {
		run, e := publicCLIOptIn(tc.enabled, tc.required)
		if run != tc.run || (e != nil) != tc.fail {
			t.Fatal("public_cli_native_opt_in_contract_changed")
		}
	}
}

// Exact declared JSON names are read from the actual private durable registry.
// These values are cleanup expectations only; they never supply restore proof.
type publicCLIRestoreIntent struct {
	Format          int               `json:"format_version"`
	Kind            string            `json:"kind"`
	Original        string            `json:"original_source_sha"`
	Tool            string            `json:"tool_source_sha"`
	Operation       string            `json:"operation_id"`
	Run             string            `json:"actual_run_id"`
	Manifest        string            `json:"manifest_sha256"`
	Archive         string            `json:"archive_sha256"`
	Namespace       string            `json:"namespace"`
	Owner           string            `json:"owner"`
	Name            string            `json:"container_name"`
	Image           string            `json:"image_id"`
	Arch            string            `json:"architecture"`
	Labels          map[string]string `json:"labels"`
	ContainerLabels map[string]string `json:"container_labels"`
	Volumes         []string          `json:"volumes"`
	ToolHash        string            `json:"tool_sha256"`
	Network         string            `json:"network"`
	Drop            bool              `json:"drop_authority"`
	Purge           bool              `json:"purge_after_acceptance_required"`
}

func TestLifecyclePublicCLIRestoreIntentRegistryShape(t *testing.T) {
	// The fixed shape is startLifecycleOwnedEngine's durable intent, including
	// all exact producer keys. It is an input-contract test, not restore proof.
	raw := []byte(`{"format_version":1,"kind":"temporary_network_none_restore_intent","original_source_sha":"1111111111111111111111111111111111111111","tool_source_sha":"2222222222222222222222222222222222222222","operation_id":"100-1","actual_run_id":"100-4","manifest_sha256":"3333333333333333333333333333333333333333333333333333333333333333","archive_sha256":"4444444444444444444444444444444444444444444444444444444444444444","namespace":"qs_retirement_restore_555555555555555555555555","owner":"66666666666666666666666666666666","container_name":"qs-retirement-restore-66666666666666666666666666666666","image_id":"sha256:7777777777777777777777777777777777777777777777777777777777777777","architecture":"amd64","labels":{"codex.owner":"66666666666666666666666666666666"},"container_labels":{"codex.owner":"66666666666666666666666666666666","vendor":"actual-image-label"},"volumes":["qs-retirement-data-66666666666666666666666666666666"],"tool_sha256":"8888888888888888888888888888888888888888888888888888888888888888","network":"none","drop_authority":false,"purge_after_acceptance_required":true}`)
	var got publicCLIRestoreIntent
	if exactJSON(raw, &got) != nil {
		t.Fatal("public_cli_exact_producer_registry_rejected")
	}
	want := publicCLIRestoreIntent{
		Format: 1, Kind: "temporary_network_none_restore_intent",
		Original: strings.Repeat("1", 40), Tool: strings.Repeat("2", 40),
		Operation: "100-1", Run: "100-4", Manifest: strings.Repeat("3", 64), Archive: strings.Repeat("4", 64),
		Namespace: "qs_retirement_restore_" + strings.Repeat("5", 24), Owner: strings.Repeat("6", 32),
		Name: "qs-retirement-restore-" + strings.Repeat("6", 32), Image: "sha256:" + strings.Repeat("7", 64), Arch: "amd64",
		Labels:          map[string]string{"codex.owner": strings.Repeat("6", 32)},
		ContainerLabels: map[string]string{"codex.owner": strings.Repeat("6", 32), "vendor": "actual-image-label"},
		Volumes:         []string{"qs-retirement-data-" + strings.Repeat("6", 32)}, ToolHash: strings.Repeat("8", 64), Network: "none", Drop: false, Purge: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("public_cli_exact_producer_registry_fields_changed")
	}
	cases := map[string][]byte{
		"source_case_alias":           []byte(strings.Replace(string(raw), `"original_source_sha"`, `"Original_source_sha"`, 1)),
		"container_labels_case_alias": []byte(strings.Replace(string(raw), `"container_labels"`, `"Container_Labels"`, 1)),
		"drop_case_alias":             []byte(strings.Replace(string(raw), `"drop_authority"`, `"Drop_Authority"`, 1)),
		"unknown_field":               append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"unknown_registry_fact":true}`)...),
		"duplicate_source":            append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"original_source_sha":"overwritten"}`)...),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if exactJSON(input, new(publicCLIRestoreIntent)) == nil {
				t.Fatal("public_cli_registry_alias_unknown_or_duplicate_accepted")
			}
		})
	}
}

type publicCLIInspection struct {
	ID          string `json:"Id"`
	Name, Image string
	Config      struct{ Labels map[string]string }
	HostConfig  struct {
		NetworkMode  string
		PortBindings map[string][]struct{ HostIP, HostPort string }
	}
	NetworkSettings struct {
		Ports map[string][]struct{ HostIP, HostPort string }
	}
	ExecIDs json.RawMessage
	Mounts  []struct {
		Type, Name, Source, Destination string
		RW                              bool
	}
}

func publicCLIRuntimeMatches(v publicCLIInspection, r publicCLIRestoreIntent, id, tool, archive string) bool {
	if (len(r.Volumes) != 1 && len(r.Volumes) != 2) || len(v.ID) != 64 || v.ID != id || v.Name != "/"+r.Name || v.Image != r.Image || v.HostConfig.NetworkMode != "none" || len(v.HostConfig.PortBindings) != 0 || !reflect.DeepEqual(v.Config.Labels, r.ContainerLabels) {
		return false
	}
	var active []string
	if len(v.ExecIDs) == 0 || json.Unmarshal(v.ExecIDs, &active) != nil || len(active) != 0 {
		return false
	}
	for _, p := range v.NetworkSettings.Ports {
		if len(p) != 0 {
			return false
		}
	}
	expected := map[string]string{r.Volumes[0]: "/var/lib/mysql"}
	if len(r.Volumes) == 2 {
		expected = map[string]string{r.Volumes[0]: "/data/db", r.Volumes[1]: "/data/configdb"}
	}
	seen, binds := map[string]bool{}, map[string]bool{}
	for _, m := range v.Mounts {
		if m.Type == "volume" && m.RW && expected[m.Name] == m.Destination && !seen[m.Name] {
			seen[m.Name] = true
			continue
		}
		if m.Type == "bind" && !m.RW && ((m.Source == tool && m.Destination == "/tool/restore-native") || (m.Source == archive && m.Destination == "/backup")) && !binds[m.Destination] {
			binds[m.Destination] = true
			continue
		}
		return false
	}
	return len(seen) == len(expected) && len(binds) == 2 && len(v.Mounts) == len(expected)+2
}

func publicCLIPrivateJSON(path string, dst any) error {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return ErrPrivate
	}
	defer func() { _ = f.Close() }()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Sys().(*syscall.Stat_t).Uid != 0 || st.Sys().(*syscall.Stat_t).Nlink != 1 || st.Size() > 256<<10 {
		return ErrPrivate
	}
	raw, e := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if e != nil || len(raw) > 256<<10 {
		return ErrPrivate
	}
	return exactJSON(raw, dst)
}

func publicCLIDocker(ctx context.Context, args ...string) ([]byte, error) {
	return nativeCommand(ctx, []string{"PATH=/usr/bin:/bin"}, "/usr/bin/docker", append([]string{"--host", "unix:///run/docker.sock"}, args...)...)
}

// Test-owned cleanup reads exact durable intent, observes the same daemon and
// verifies each actual CID/image/label/mount before removing only that resource.
// It is independent fixture cleanup, never the unavailable A production purge.
func publicCLICleanupEngines(root, archive, source, operation, run, manifest string) (int, error) {
	paths, e := filepath.Glob(filepath.Join(root, "restore-*.intent.private.json"))
	if e != nil {
		return 0, ErrPrivate
	}
	count := 0
	for _, path := range paths {
		var r publicCLIRestoreIntent
		if publicCLIPrivateJSON(path, &r) != nil || filepath.Base(path) != "restore-"+r.Owner+".intent.private.json" || r.Format != 1 || r.Kind != "temporary_network_none_restore_intent" || r.Original != source || r.Tool != source || r.Operation != operation || r.Run != run || r.Manifest != manifest || r.Namespace != "qs_retirement_restore_"+sha([]byte(source + "\n" + operation + "\n" + run + "\n" + manifest))[:24] || r.Arch != runtime.GOARCH || len(r.Owner) != 32 || r.Name != "qs-retirement-restore-"+r.Owner || r.Network != "none" || r.Drop || !r.Purge || !strings.HasPrefix(r.Image, "sha256:") || !hashPattern.MatchString(strings.TrimPrefix(r.Image, "sha256:")) || !hashPattern.MatchString(r.Archive) || !hashPattern.MatchString(r.ToolHash) || (len(r.Volumes) != 1 && len(r.Volumes) != 2) {
			return count, ErrIsolation
		}
		labels := map[string]string{"codex.task": "qs-compatibility-retirement", "codex.owner": r.Owner, "qs.retirement.operation": operation, "qs.retirement.run": run}
		if !reflect.DeepEqual(r.Labels, labels) || r.Volumes[0] != "qs-retirement-data-"+r.Owner || (len(r.Volumes) == 2 && r.Volumes[1] != "qs-retirement-config-"+r.Owner) {
			return count, ErrIsolation
		}
		for k, v := range labels {
			if r.ContainerLabels[k] != v {
				return count, ErrIsolation
			}
		}
		imageRaw, e := publicCLIDocker(context.Background(), "image", "inspect", r.Image)
		var images []struct {
			ID           string `json:"Id"`
			Architecture string
			Config       struct{ Labels map[string]string }
		}
		if e != nil || json.Unmarshal(imageRaw, &images) != nil || len(images) != 1 || images[0].ID != r.Image || images[0].Architecture != r.Arch {
			return count, ErrIsolation
		}
		actualLabels := map[string]string{}
		for k, v := range images[0].Config.Labels {
			actualLabels[k] = v
		}
		for k, v := range labels {
			actualLabels[k] = v
		}
		if !reflect.DeepEqual(actualLabels, r.ContainerLabels) {
			return count, ErrIsolation
		}
		tool := filepath.Join(root, "restore-native")
		toolRaw, e := os.ReadFile(tool)
		if e != nil || sha(toolRaw) != r.ToolHash {
			return count, ErrPrivate
		}
		var created struct {
			ID        string `json:"container_id"`
			Owner     string `json:"owner"`
			Operation string `json:"operation_id"`
			Run       string `json:"actual_run_id"`
			Drop      bool   `json:"drop_authority"`
		}
		createdPath := filepath.Join(root, "restore-"+r.Owner+".created.private.json")
		hasCreated := false
		if _, statErr := os.Lstat(createdPath); statErr == nil {
			if publicCLIPrivateJSON(createdPath, &created) != nil {
				return count, ErrPrivate
			}
			hasCreated = true
		} else if !os.IsNotExist(statErr) {
			return count, ErrPrivate
		}
		raw, e := publicCLIDocker(context.Background(), "ps", "--all", "--filter", "name=^/"+r.Name+"$", "--format", "{{.ID}}")
		if e != nil {
			return count, ErrIsolation
		}
		found := strings.TrimSpace(string(raw))
		if found != "" {
			raw, e = publicCLIDocker(context.Background(), "inspect", r.Name)
			var actual []publicCLIInspection
			if e != nil || json.Unmarshal(raw, &actual) != nil || len(actual) != 1 {
				return count, ErrIsolation
			}
			id := actual[0].ID
			if hasCreated && (created.ID != id || created.Owner != r.Owner || created.Operation != operation || created.Run != run || created.Drop) {
				return count, ErrIsolation
			}
			if !publicCLIRuntimeMatches(actual[0], r, id, tool, archive) {
				return count, ErrIsolation
			}
			if _, e = publicCLIDocker(context.Background(), "rm", "--force", id); e != nil {
				return count, ErrIsolation
			}
		} else if hasCreated {
			// A previous fixture cleanup may have received a remove response.
			// Reobserve exact CID absence without adopting or creating resources.
			if len(created.ID) != 64 || created.Owner != r.Owner || created.Operation != operation || created.Run != run || created.Drop {
				return count, ErrIsolation
			}
			absent, absenceErr := publicCLIDocker(context.Background(), "ps", "--all", "--filter", "id="+created.ID, "--format", "{{.ID}}")
			if absenceErr != nil || strings.TrimSpace(string(absent)) != "" {
				return count, ErrIsolation
			}
		}
		for _, name := range r.Volumes {
			raw, e = publicCLIDocker(context.Background(), "volume", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
			if e != nil {
				return count, ErrIsolation
			}
			if strings.TrimSpace(string(raw)) == "" {
				continue
			}
			raw, e = publicCLIDocker(context.Background(), "volume", "inspect", "--format", "{{json .Labels}}", name)
			var actual map[string]string
			if e != nil || json.Unmarshal(raw, &actual) != nil || !reflect.DeepEqual(actual, labels) {
				return count, ErrIsolation
			}
			if _, e = publicCLIDocker(context.Background(), "volume", "rm", name); e != nil {
				return count, ErrIsolation
			}
		}
		for _, args := range [][]string{{"ps", "--all", "--filter", "label=codex.owner=" + r.Owner, "--format", "{{.ID}}"}, {"volume", "ls", "--filter", "label=codex.owner=" + r.Owner, "--format", "{{.Name}}"}} {
			raw, e = publicCLIDocker(context.Background(), args...)
			if e != nil || strings.TrimSpace(string(raw)) != "" {
				return count, ErrIsolation
			}
		}
		count++
	}
	for _, args := range [][]string{
		{"ps", "--all", "--filter", "label=qs.retirement.operation=" + operation, "--filter", "label=qs.retirement.run=" + run, "--format", "{{.ID}}"},
		{"volume", "ls", "--filter", "label=qs.retirement.operation=" + operation, "--filter", "label=qs.retirement.run=" + run, "--format", "{{.Name}}"},
	} {
		raw, err := publicCLIDocker(context.Background(), args...)
		if err != nil || strings.TrimSpace(string(raw)) != "" {
			return count, ErrIsolation
		}
	}
	return count, nil
}

const (
	publicCLIInventoryPageSize = 10000
	publicCLIInventoryMaxPages = 1001
)

// These paths describe only successful output from the fixed v2 inventory
// producer. They grant no backup, restore or purge permission.
func publicCLIInventoryMaterialPaths(report inventory, run string) (map[string]bool, error) {
	if !runPattern.MatchString(run) || report.RunID != run || !report.Complete || report.ErrorCategory != "none" || len(report.Targets) != 4 {
		return nil, ErrSource
	}
	directory := "inventory-" + run
	paths := map[string]bool{directory: true}
	for i, s := range report.Targets {
		database, kind := "mysql", "base_table"
		if i == 3 {
			database, kind = "mongodb", "collection"
		}
		if s.Database != database || s.Name != targetNames[i] || s.Kind != kind || !s.Present || !s.Complete || s.ErrorCategory != "none" || s.NextCycle || s.Passes != 2 || s.SourceFile != sourceNames[i] || s.Records > 1000000 || s.Boundary.Database != database || s.Boundary.Name != targetNames[i] || s.Boundary.Kind != kind || !s.Boundary.Present {
			return nil, ErrSource
		}
		// Both passes return before querying/checkpointing an approved empty bound.
		// Otherwise an exactly full last page produces a final empty EOF page.
		pagesPerPass := uint64(0)
		if s.Boundary.Empty {
			if s.Records != 0 {
				return nil, ErrSource
			}
		} else {
			pagesPerPass = s.Records/publicCLIInventoryPageSize + 1
		}
		if pagesPerPass > publicCLIInventoryMaxPages || s.Pages != 2*pagesPerPass {
			return nil, ErrSource
		}
		paths[filepath.Join(directory, s.SourceFile)] = true
		paths[filepath.Join(directory, s.SourceFile+".asset.json")] = true
		for pass := 1; pass <= 2; pass++ {
			for page := uint64(1); page <= pagesPerPass; page++ {
				paths[filepath.Join(directory, fmt.Sprintf("%s-%s-pass-%d-page-%06d.checkpoint.json", database, s.Name, pass, page))] = true
			}
		}
	}
	return paths, nil
}

func publicCLIMaterialRelativeKnown(path, name string, allowed map[string]bool) error {
	relative, e := filepath.Rel(path, name)
	if e != nil || (relative != "." && !allowed[relative]) {
		return ErrPrivate
	}
	return nil
}

func TestLifecyclePublicCLIInventoryMaterialBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		records, pagesPerPass uint64
		empty                 bool
	}{
		{"approved_empty", 0, 0, true},
		{"nonempty_bound_zero_records", 0, 1, false},
		{"partial_page", publicCLIInventoryPageSize - 1, 1, false},
		{"one_full_page_plus_empty_eof", publicCLIInventoryPageSize, 2, false},
		{"multi_page", publicCLIInventoryPageSize + 1, 2, false},
		{"two_full_pages_plus_empty_eof", 2 * publicCLIInventoryPageSize, 3, false},
		{"maximum_records_and_eof", 1000000, 1000000/publicCLIInventoryPageSize + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := publicCLIInventoryMaterialTestReport(tc.records, tc.pagesPerPass, tc.empty)
			paths, e := publicCLIInventoryMaterialPaths(report, report.RunID)
			if e != nil || len(paths) != 9+int(tc.pagesPerPass)*8 {
				t.Fatal("public_cli_inventory_material_boundary_rejected")
			}
			for _, s := range report.Targets {
				directory := "inventory-" + report.RunID
				if !paths[filepath.Join(directory, s.SourceFile+".asset.json")] {
					t.Fatal("public_cli_inventory_source_asset_missing")
				}
				for pass := 1; pass <= 2; pass++ {
					prefix := s.Database + "-" + s.Name
					if tc.pagesPerPass > 0 && !paths[filepath.Join(directory, fmt.Sprintf("%s-pass-%d-page-%06d.checkpoint.json", prefix, pass, tc.pagesPerPass))] {
						t.Fatal("public_cli_inventory_last_eof_page_missing")
					}
					if paths[filepath.Join(directory, fmt.Sprintf("%s-pass-%d-page-%06d.checkpoint.json", prefix, pass, tc.pagesPerPass+1))] {
						t.Fatal("public_cli_inventory_extra_page_allowed")
					}
				}
			}
		})
	}
	for _, name := range []string{"odd_pages", "missing_eof_page", "extra_pages", "empty_with_records", "source_path_escape", "wrong_passes", "wrong_database", "incomplete_report"} {
		t.Run(name, func(t *testing.T) {
			report := publicCLIInventoryMaterialTestReport(publicCLIInventoryPageSize, 2, false)
			switch name {
			case "odd_pages":
				report.Targets[0].Pages = 3
			case "missing_eof_page":
				report.Targets[0].Pages = 2
			case "extra_pages":
				report.Targets[0].Pages = 6
			case "empty_with_records":
				report.Targets[0].Boundary.Empty = true
			case "source_path_escape":
				report.Targets[0].SourceFile = "../unregistered"
			case "wrong_passes":
				report.Targets[0].Passes = 1
			case "wrong_database":
				report.Targets[0].Database = "mongodb"
			case "incomplete_report":
				report.Complete = false
			}
			if _, e := publicCLIInventoryMaterialPaths(report, report.RunID); e == nil {
				t.Fatal("public_cli_inventory_inconsistent_material_plan_accepted")
			}
		})
	}
}

func publicCLIInventoryMaterialTestReport(records, pagesPerPass uint64, empty bool) inventory {
	report := inventory{RunID: "100-3", Complete: true, ErrorCategory: "none"}
	for i := 0; i < 4; i++ {
		database, kind := "mysql", "base_table"
		if i == 3 {
			database, kind = "mongodb", "collection"
		}
		s := SourceSnapshot{Database: database, Name: targetNames[i], Kind: kind, Present: true, Complete: true, ErrorCategory: "none", Records: records, SourceFile: sourceNames[i], Passes: 2, Pages: 2 * pagesPerPass}
		s.Boundary.Database, s.Boundary.Name, s.Boundary.Kind = database, targetNames[i], kind
		s.Boundary.Present, s.Boundary.Empty = true, empty
		report.Targets = append(report.Targets, s)
	}
	return report
}

func TestLifecyclePublicCLIInventoryMaterialDirectory(t *testing.T) {
	report := publicCLIInventoryMaterialTestReport(publicCLIInventoryPageSize, 2, false)
	allowed, e := publicCLIInventoryMaterialPaths(report, report.RunID)
	if e != nil {
		t.Fatal("public_cli_inventory_material_test_plan_rejected")
	}
	root := t.TempDir()
	for relative := range allowed {
		path := filepath.Join(root, relative)
		if relative == "inventory-"+report.RunID {
			if os.MkdirAll(path, 0700) != nil {
				t.Fatal("public_cli_inventory_material_directory_failed")
			}
		} else {
			if os.MkdirAll(filepath.Dir(path), 0700) != nil || os.WriteFile(path, []byte("fixture"), 0600) != nil {
				t.Fatal("public_cli_inventory_material_file_failed")
			}
		}
	}
	// Exercise the same filename check on real directory entries. This does not
	// impersonate root ownership; the production-equivalent metadata gate stays
	// independent and must still reject these local non-root fixtures.
	namesKnown := func() error {
		return filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return ErrPrivate
			}
			return publicCLIMaterialRelativeKnown(root, name, allowed)
		})
	}
	if namesKnown() != nil {
		t.Fatal("public_cli_inventory_exact_directory_names_rejected")
	}
	if os.Geteuid() != 0 && publicCLIMaterialsKnown(root, allowed) == nil {
		t.Fatal("public_cli_inventory_nonroot_metadata_accepted")
	}
	extra := filepath.Join(root, "inventory-"+report.RunID, "mysql-domain_event_outbox-pass-3-page-000001.checkpoint.json")
	if os.WriteFile(extra, []byte("unregistered"), 0600) != nil {
		t.Fatal("public_cli_inventory_unknown_file_setup_failed")
	}
	if namesKnown() == nil {
		t.Fatal("public_cli_inventory_unknown_actual_file_accepted")
	}
	if os.Remove(extra) != nil || namesKnown() != nil {
		t.Fatal("public_cli_inventory_actual_directory_recheck_failed")
	}
}

func publicCLIMaterialsKnown(path string, allowed map[string]bool) error {
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return ErrPrivate
	}
	return filepath.WalkDir(path, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ErrPrivate
		}
		if publicCLIMaterialRelativeKnown(path, name, allowed) != nil {
			return ErrPrivate
		}
		info, e := os.Lstat(name)
		if e != nil || info.Mode()&os.ModeSymlink != 0 || info.Sys().(*syscall.Stat_t).Uid != 0 {
			return ErrPrivate
		}
		if info.IsDir() {
			if info.Mode().Perm() != 0700 {
				return ErrPrivate
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Nlink != 1 || (info.Mode().Perm() != 0600 && info.Mode().Perm() != 0700) {
			return ErrPrivate
		}
		return nil
	})
}

func publicCLIFixedParentRole(path string) string {
	roles := map[string]string{
		"/":                      "filesystem_root",
		"/opt":                   "opt_parent",
		"/opt/backups":           "backups_parent",
		"/opt/backups/qs-server": "service_parent",
		"/opt/backups/qs-server/compatibility-retirement": "operation_base",
		"/private":        "native_private_parent",
		"/private/tmp":    "native_tmp_parent",
		"/tmp":            "system_tmp_parent",
		nativePrivateRoot: "native_private_base",
	}
	if role, ok := roles[path]; ok {
		return role
	}
	return "other_ancestor"
}

func publicCLIProtectedDir(t *testing.T, path string) {
	t.Helper()
	if os.MkdirAll(path, 0700) != nil {
		t.Fatal("public_cli_fixture_directory_create_failed")
	}
	for p := path; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Sys().(*syscall.Stat_t).Uid != 0 || (p != "/tmp" && st.Mode().Perm()&022 != 0) {
			if e == nil {
				v := st.Sys().(*syscall.Stat_t)
				t.Logf("public_cli_fixture_parent_rejected role=%s stat_observed=true uid=%d gid=%d mode=%04o symlink=%t directory=%t", publicCLIFixedParentRole(p), v.Uid, v.Gid, uint32(v.Mode)&07777, st.Mode()&os.ModeSymlink != 0, st.IsDir())
			} else {
				t.Logf("public_cli_fixture_parent_rejected role=%s stat_observed=false", publicCLIFixedParentRole(p))
			}
			t.Fatal("public_cli_fixture_protected_parent_rejected")
		}
		if p == filepath.Dir(p) {
			break
		}
	}
}

func publicCLIInventory(t *testing.T, cli, opDir, source, operation, boundRun, invRun string, env map[string]string, db *sql.DB, mdb *mongo.Database) (Approval, inventory) {
	t.Helper()
	ids, heads := nativeIdentities(t, db, mdb)
	scope := [4][3]string{{"mysql", "domain_event_outbox", "base_table"}, {"mysql", "ai_bridge_commands", "base_table"}, {"mysql", "ai_messaging_legacy_commands", "base_table"}, {"mongodb", "domain_event_outbox", "collection"}}
	limits := map[string]int{"query_seconds": 30, "total_seconds": 1500, "max_records": 1000000, "max_bytes": 2 << 30, "page_size": publicCLIInventoryPageSize, "max_pages": publicCLIInventoryMaxPages}
	request := map[string]any{"format_version": 2, "kind": "readonly_inventory_boundary_request", "source_sha": source, "operation_id": operation, "target_hash": jsonSHA(scope), "database_scope": "mysql-and-mongodb", "identity_hashes": ids, "expected_migrations": heads, "limits": limits}
	path := filepath.Join(opDir, "boundary-request.json")
	hash := nativeJSON(t, path, request)
	out := filepath.Join(opDir, "bounds-"+boundRun)
	if os.Mkdir(out, 0700) != nil {
		t.Fatal("public_cli_native_inventory_directory_failed")
	}
	child := nativeChildEnv(env)
	if _, e := nativeCommand(context.Background(), child, cli, "--mode=bounds", "--request", path, "--request-hash", hash, "--operation-id", operation, "--run-id", boundRun, "--output-directory", out); e != nil {
		t.Fatal("public_cli_native_bounds_failed")
	}
	raw, e := os.ReadFile(filepath.Join(out, "boundary.private.json"))
	var bound inventory
	if e != nil || exactJSON(raw, &bound) != nil || !bound.Complete || len(bound.Targets) != 4 {
		t.Fatal("public_cli_native_bounds_incomplete")
	}
	boundaries := make([]any, 4)
	for i, s := range bound.Targets {
		boundaries[i] = s.Boundary
	}
	request["kind"] = "readonly_inventory_request"
	request["boundary_run_id"] = boundRun
	request["boundary_report_hash"] = sha(raw)
	request["approved_boundaries"] = boundaries
	path = filepath.Join(opDir, "inventory-request.json")
	hash = nativeJSON(t, path, request)
	out = filepath.Join(opDir, "inventory-"+invRun)
	if os.Mkdir(out, 0700) != nil {
		t.Fatal("public_cli_native_inventory_directory_failed")
	}
	if _, e = nativeCommand(context.Background(), child, cli, "--mode=inventory", "--request", path, "--request-hash", hash, "--operation-id", operation, "--run-id", invRun, "--output-directory", out); e != nil {
		t.Fatal("public_cli_native_inventory_scan_failed")
	}
	reportRaw, e := os.ReadFile(filepath.Join(out, "inventory.private.json"))
	var report inventory
	if e != nil || exactJSON(reportRaw, &report) != nil || !report.Complete || report.DropReady || len(report.Targets) != 4 || report.ErrorCategory != "none" {
		t.Fatal("public_cli_native_inventory_incomplete")
	}
	for _, s := range report.Targets {
		if !s.Complete || !s.Present || s.Passes != 2 || s.NextCycle || s.ErrorCategory != "none" {
			t.Fatal("public_cli_native_inventory_eof_missing")
		}
	}
	sqlRaw, e := os.ReadFile(filepath.Join(out, "mysql-metadata.private.json"))
	if e != nil {
		t.Fatal("public_cli_native_sql_metadata_missing")
	}
	mongoRaw, e := os.ReadFile(filepath.Join(out, "mongodb-metadata.private.json"))
	if e != nil {
		t.Fatal("public_cli_native_mongo_metadata_missing")
	}
	ordered, e := ReadOrderedMongoSchema(context.Background(), mdb)
	if e != nil {
		t.Fatal("public_cli_native_ordered_mongo_schema_missing")
	}
	return Approval{InventorySHA256: sha(reportRaw), SQLMetadataSHA256: sha(sqlRaw), MongoMetadataSHA256: sha(mongoRaw), OrderedMongoSchemaSHA256: ordered.SHA256(), SourceSHA: source, OperationID: operation, RunID: invRun, RequestHash: hash}, report
}

type publicCLIWindowIntent struct {
	Format        int    `json:"format_version"`
	Kind          string `json:"kind"`
	Dispatcher    string `json:"dispatcher_source_sha"`
	Tool          string `json:"tool_source_sha"`
	Original      string `json:"original_source_sha"`
	Operation     string `json:"operation_id"`
	OriginalRun   string `json:"original_run_id"`
	ActualRun     string `json:"actual_run_id"`
	Stage         string `json:"stage"`
	Template      string `json:"approved_template_sha256"`
	Request       string `json:"derived_request_sha256"`
	Manifest      string `json:"manifest_sha256"`
	Package       string `json:"package_sha256"`
	ToolDirectory string `json:"tool_directory"`
	ToolProgram   string `json:"tool_program_sha256"`
	Native        string `json:"native_sha256"`
	NativePath    string `json:"native_path"`
	BImage        string `json:"b_image_id"`
	BProgram      string `json:"b_program_sha256"`
	SourceUID     int    `json:"source_uid"`
	Drop          bool   `json:"drop_authority"`
}

func publicCLIWindowPackage(t *testing.T, path, cli, other, repo, toolDirectory string) (string, map[string]string, map[string]string) {
	t.Helper()
	if !filepath.IsAbs(other) || runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Fatal("public_cli_native_other_architecture_binary_missing")
	}
	otherArch := "arm64"
	if runtime.GOARCH == "arm64" {
		otherArch = "amd64"
	}
	paths := map[string]string{"inventory-linux-" + runtime.GOARCH: cli, "inventory-linux-" + otherArch: other, "compatibility-window-tool.py": filepath.Join(repo, "scripts/database/compatibility-window-tool.py"), "receipt-transport.py": filepath.Join(repo, "scripts/dbops/receipt-transport.py")}
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	binaryHashes, programHashes := map[string]string{}, map[string]string{}
	for _, name := range []string{"compatibility-window-tool.py", "receipt-transport.py", "inventory-linux-amd64", "inventory-linux-arm64"} {
		raw, e := os.ReadFile(paths[name])
		if e != nil || len(raw) == 0 || len(raw) > 64<<20 {
			t.Fatal("public_cli_native_window_package_input_missing")
		}
		if strings.HasPrefix(name, "inventory-linux-") {
			arch := strings.TrimPrefix(name, "inventory-linux-")
			image, e := elf.NewFile(bytes.NewReader(raw))
			want := elf.EM_X86_64
			if arch == "arm64" {
				want = elf.EM_AARCH64
			}
			if e != nil || image.Machine != want || image.Class != elf.ELFCLASS64 || image.Close() != nil {
				t.Fatal("public_cli_native_window_package_elf_architecture_rejected")
			}
			binaryHashes[arch] = sha(raw)
		} else {
			if writePrivate(filepath.Join(toolDirectory, name), raw) != nil {
				t.Fatal("public_cli_native_window_program_copy_failed")
			}
			programHashes[name] = sha(raw)
		}
		if tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(raw)), Typeflag: tar.TypeReg}) != nil {
			t.Fatal("public_cli_native_package_failed")
		}
		if _, e = tw.Write(raw); e != nil {
			t.Fatal("public_cli_native_package_failed")
		}
	}
	if tw.Close() != nil || gz.Close() != nil || writePrivate(path, b.Bytes()) != nil {
		t.Fatal("public_cli_native_package_failed")
	}
	return sha(b.Bytes()), binaryHashes, programHashes
}

func publicCLIDecode(t *testing.T, repo string, raw []byte) map[string]any {
	t.Helper()
	script := `import importlib.util,sys; p=sys.argv[1];s=importlib.util.spec_from_file_location("transport",p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);print(m.decode_armored_receipt(sys.stdin.read()))`
	cmd := exec.Command("/usr/bin/python3", "-I", "-c", script, filepath.Join(repo, "scripts/dbops/receipt-transport.py"))
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdin = bytes.NewReader(raw)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	var value map[string]any
	if cmd.Run() != nil || out.Len() > 64<<10 || json.Unmarshal(out.Bytes(), &value) != nil {
		t.Fatal("public_cli_native_receipt_transport_rejected")
	}
	return value
}

// A CommandContext kill of the public Python process alone leaves its
// root-once/native descendants running. All known host children inherit this
// dedicated group. Group absence is observed after Wait; it does not prove that
// a daemon-side Docker exec stopped, which cleanup still checks independently.
func publicCLIHostGroupAbsent(group int) (bool, error) {
	if group < 2 {
		return false, errors.New("public_cli_native_process_group_binding_missing")
	}
	err := syscall.Kill(-group, 0)
	if errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	return false, err
}

func publicCLIRunProcessGroup(cmd *exec.Cmd) (runErr error, stopped bool) {
	if cmd == nil || cmd.Process != nil || cmd.SysProcAttr != nil {
		return errors.New("public_cli_native_process_group_binding_rejected"), false
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	// Even a surviving descendant with an inherited pipe cannot block Wait
	// forever. A timed-out Wait never supplies a successful operation receipt.
	cmd.WaitDelay = 2 * time.Second
	if runErr = cmd.Start(); runErr != nil {
		return runErr, false
	}
	group := cmd.Process.Pid // Setpgid is applied by the actual successful fork.
	runErr = cmd.Wait()
	absent, err := publicCLIHostGroupAbsent(group)
	if err != nil {
		return errors.New("public_cli_native_process_group_observation_failed"), false
	}
	if absent {
		return runErr, true
	}
	if runErr == nil {
		runErr = errors.New("public_cli_native_host_descendants_remained")
	}
	// The host producer is known by this actual child PID/group, not a request
	// field. If the fresh group-absence observation remains unknown, every
	// fixture deletion is blocked and private recovery material is retained.
	if err = syscall.Kill(-group, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return errors.New("public_cli_native_process_group_termination_failed"), false
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		absent, err = publicCLIHostGroupAbsent(group)
		if err != nil {
			return errors.New("public_cli_native_process_group_observation_failed"), false
		}
		if absent {
			return runErr, true
		}
		if !time.Now().Before(deadline) {
			return errors.New("public_cli_native_process_group_stop_unproven"), false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// This one fixture binds a real API program from the approved source, never
// a restore-engine image or a guessed hash. Its scratch image needs no network;
// the exact copy probe is created but never started and has no host mounts.
func publicCLIFixtureAPIImage(t *testing.T, binary, source, operation, run, private string, cleanupAllowed func() bool) (string, string) {
	t.Helper()
	if !filepath.IsAbs(binary) || !sourcePattern.MatchString(source) || !runPattern.MatchString(operation) || !runPattern.MatchString(run) {
		t.Fatal("public_cli_native_api_fixture_input_rejected")
	}
	info, e := buildinfo.ReadFile(binary)
	if e != nil || info.Path != "github.com/FangcunMount/qs-server/cmd/qs-apiserver" {
		t.Fatal("public_cli_native_actual_api_program_required")
	}
	compiledSource, architecture, goos := "", "", ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "GOOS":
			goos = setting.Value
		case "GOARCH":
			architecture = setting.Value
		case "-ldflags":
			fields := strings.Fields(setting.Value)
			for i, field := range fields {
				if strings.Contains(field, "github.com/FangcunMount/qs-server/pkg/version.GitCommit=") {
					if i == 0 || fields[i-1] != "-X" || field != "github.com/FangcunMount/qs-server/pkg/version.GitCommit="+source || compiledSource != "" {
						t.Fatal("public_cli_native_actual_api_source_rejected")
					}
					compiledSource = source
				}
			}
		}
	}
	program, readErr := os.ReadFile(binary)
	if compiledSource != source || goos != "linux" || architecture != runtime.GOARCH || readErr != nil || len(program) == 0 || len(program) > 256<<20 {
		t.Fatal("public_cli_native_actual_api_source_rejected")
	}
	owner := source + "/" + operation + "/" + run
	label := "codex.public-cli-api-owner"
	name := "qs-public-cli-api-" + operation + "-" + run
	image, cid := "", ""
	imageFormat := `{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}},"revision":{{json (index .Config.Labels "org.opencontainers.image.revision")}},"owner":{{json (index .Config.Labels "codex.public-cli-api-owner")}}}`
	checkImage := func() bool {
		raw, err := publicCLIDocker(context.Background(), "image", "inspect", "--format", imageFormat, image)
		var v struct{ ID, OS, Architecture, Revision, Owner string }
		return err == nil && json.Unmarshal(raw, &v) == nil && v.ID == image && v.OS == "linux" && v.Architecture == runtime.GOARCH && v.Revision == source && v.Owner == owner
	}
	probeFormat := `{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"owner":{{json (index .Config.Labels "codex.public-cli-api-owner")}},"running":{{json .State.Running}},"status":{{json .State.Status}},"readonly":{{json .HostConfig.ReadonlyRootfs}},"network":{{json .HostConfig.NetworkMode}},"mounts":{{json .Mounts}},"execs":{{json .ExecIDs}}}`
	checkProbe := func() bool {
		raw, err := publicCLIDocker(context.Background(), "container", "inspect", "--format", probeFormat, cid)
		var v struct {
			ID, Name, Image, Owner, Network, Status string
			Running, Readonly                       bool
			Mounts                                  []json.RawMessage
			Execs                                   []string
		}
		return err == nil && json.Unmarshal(raw, &v) == nil && v.ID == cid && v.Name == "/"+name && v.Image == image && v.Owner == owner && !v.Running && v.Status == "created" && v.Readonly && v.Network == "none" && len(v.Mounts) == 0 && len(v.Execs) == 0
	}
	// An unknown build/create or unaccepted public child retains only this exact
	// fixture, with its original body-free intent for diagnosis. Never prune.
	t.Cleanup(func() {
		if !cleanupAllowed() || image == "" || cid == "" || !checkImage() || !checkProbe() {
			nativeRetainCleanup(t, private)
			t.Error("public_cli_native_api_fixture_cleanup_unproven_retained")
			return
		}
		if _, err := publicCLIDocker(context.Background(), "container", "rm", cid); err != nil {
			t.Error("public_cli_native_api_probe_remove_unknown")
			return
		}
		remaining, err := publicCLIDocker(context.Background(), "container", "ls", "--all", "--no-trunc", "--filter", "name=^/"+name+"$", "--format", "{{.ID}}")
		if err != nil || len(bytes.TrimSpace(remaining)) != 0 || !checkImage() {
			t.Error("public_cli_native_api_probe_zero_unproven")
			return
		}
		if _, err = publicCLIDocker(context.Background(), "image", "rm", "--no-prune", image); err != nil {
			t.Error("public_cli_native_api_image_remove_unknown")
			return
		}
		remaining, err = publicCLIDocker(context.Background(), "image", "ls", "--quiet", "--no-trunc", "--filter", "label="+label+"="+owner)
		if err != nil || len(bytes.TrimSpace(remaining)) != 0 {
			t.Error("public_cli_native_api_image_zero_unproven")
			return
		}
		t.Log("public_cli_native_owned_api_probe_remaining=0 owned_api_images_remaining=0")
	})
	nativeJSON(t, filepath.Join(private, "api-image.intent.private.json"), map[string]any{"source_sha": source, "operation_id": operation, "actual_run_id": run, "owner": owner, "container_name": name, "program_sha256": sha(program), "network": "none", "probe_start_allowed": false})
	var contextTar bytes.Buffer
	tarWriter := tar.NewWriter(&contextTar)
	dockerfile := []byte("FROM scratch\nLABEL org.opencontainers.image.revision=" + source + "\nLABEL " + label + "=" + owner + "\nCOPY qs-apiserver /app/qs-apiserver\n")
	for _, member := range []struct {
		name string
		raw  []byte
		mode int64
	}{{"Dockerfile", dockerfile, 0600}, {"qs-apiserver", program, 0755}} {
		if tarWriter.WriteHeader(&tar.Header{Name: member.name, Mode: member.mode, Size: int64(len(member.raw)), Typeflag: tar.TypeReg}) != nil {
			t.Fatal("public_cli_native_api_build_context_failed")
		}
		if _, e = tarWriter.Write(member.raw); e != nil {
			t.Fatal("public_cli_native_api_build_context_failed")
		}
	}
	if tarWriter.Close() != nil {
		t.Fatal("public_cli_native_api_build_context_failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "/usr/bin/docker", "--host", "unix:///run/docker.sock", "build", "--quiet", "--pull=false", "--network=none", "-")
	build.Env = []string{"PATH=/usr/bin:/bin"}
	build.Stdin = &contextTar
	build.Stderr = io.Discard
	built, e := build.Output()
	image = strings.TrimSpace(string(built))
	if e != nil || !strings.HasPrefix(image, "sha256:") || !hashPattern.MatchString(strings.TrimPrefix(image, "sha256:")) || !checkImage() || !checkImage() {
		t.Fatal("public_cli_native_actual_api_image_rejected")
	}
	created, e := publicCLIDocker(ctx, "container", "create", "--name", name, "--label", label+"="+owner, "--network", "none", "--read-only", "--entrypoint", "/app/qs-apiserver", image, "--version")
	cid = strings.TrimSpace(string(created))
	if e != nil || !hashPattern.MatchString(cid) || !checkProbe() {
		t.Fatal("public_cli_native_api_copy_probe_rejected")
	}
	// Only the program-copy stream has a binary-sized budget. Metadata keeps
	// nativeCommand's original cap; this tar may contain just the verified file.
	copyBudget := int64(len(program)) + (64 << 10)
	copyCtx, copyCancel := context.WithCancel(ctx)
	copyCommand := exec.CommandContext(copyCtx, "/usr/bin/docker", "--host", "unix:///run/docker.sock", "container", "cp", cid+":/app/qs-apiserver", "-")
	copyCommand.Env = []string{"PATH=/usr/bin:/bin"}
	copyCommand.Stderr = io.Discard
	copyCommand.WaitDelay = 2 * time.Second
	copyOutput, e := copyCommand.StdoutPipe()
	if e != nil {
		copyCancel()
		t.Fatal("public_cli_native_api_image_program_read_failed")
	}
	if e = copyCommand.Start(); e != nil {
		copyCancel()
		_ = copyOutput.Close()
		t.Fatal("public_cli_native_api_image_program_read_failed")
	}
	copied, readErr := io.ReadAll(io.LimitReader(copyOutput, copyBudget+1))
	if readErr != nil || int64(len(copied)) > copyBudget {
		copyCancel()
		_ = copyOutput.Close()
	}
	waitErr := copyCommand.Wait() // Always reap this actual fixed Docker child.
	copyCancel()
	if readErr != nil || waitErr != nil || int64(len(copied)) > copyBudget {
		t.Fatal("public_cli_native_api_image_program_read_failed")
	}
	reader := tar.NewReader(bytes.NewReader(copied))
	header, e := reader.Next()
	if e != nil || header.Typeflag != tar.TypeReg || header.Name != "qs-apiserver" || header.Size != int64(len(program)) {
		t.Fatal("public_cli_native_api_image_program_shape_rejected")
	}
	actual, e := io.ReadAll(io.LimitReader(reader, int64(len(program))+1))
	actualInfo, actualInfoErr := buildinfo.Read(bytes.NewReader(actual))
	if e != nil || actualInfoErr != nil || actualInfo.Path != info.Path || !reflect.DeepEqual(actualInfo.Settings, info.Settings) || sha(actual) != sha(program) || !checkProbe() || !checkImage() {
		t.Fatal("public_cli_native_api_image_program_changed")
	}
	if _, e = reader.Next(); e != io.EOF {
		t.Fatal("public_cli_native_api_image_program_shape_rejected")
	}
	nativeJSON(t, filepath.Join(private, "api-image.created.private.json"), map[string]string{"image_id": image, "container_id": cid, "program_sha256": sha(actual), "source_sha": source, "owner": owner})
	return image, sha(actual)
}

func TestLifecyclePublicCLINativeWindowToolPrepareRootOnce(t *testing.T) {
	run, e := publicCLIOptIn(os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_NATIVE"), os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_NATIVE_REQUIRED"))
	if e != nil {
		t.Fatal(e)
	}
	if !run {
		t.Skip("public lifecycle native fixture not requested")
	}
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || os.Getenv("SUDO_UID") != "" {
		t.Fatal("public_cli_native_actual_linux_root_required")
	}
	socket, e := os.Lstat("/run/docker.sock")
	if e != nil || socket.Mode()&os.ModeSocket == 0 || socket.Sys().(*syscall.Stat_t).Uid != 0 {
		t.Fatal("public_cli_native_fixed_socket_required")
	}
	// Existing helper commands also use the fixed daemon; no default context.
	t.Setenv("DOCKER_HOST", "unix:///run/docker.sock")
	t.Setenv("DOCKER_CONTEXT", "")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_CERT_PATH", "")
	repo := os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_REPOSITORY")
	source := os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_SOURCE_SHA")
	built := os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_BINARY")
	if !filepath.IsAbs(repo) || !sourcePattern.MatchString(source) || !filepath.IsAbs(built) {
		t.Fatal("public_cli_native_exact_source_inputs_missing")
	}
	actual, e := nativeCommand(context.Background(), []string{"PATH=/usr/bin:/bin"}, "/usr/bin/git", "-c", "safe.directory="+repo, "-C", repo, "rev-parse", "HEAD")
	if e != nil || strings.TrimSpace(string(actual)) != source {
		t.Fatal("public_cli_native_actual_checkout_sha_mismatch")
	}
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil || cwd != filepath.Join(repo, "internal/apiserver/maintenance/compatibilityretirementbackup") {
		t.Fatal("public_cli_native_fixture_package_cwd_required")
	}
	mysqlID, mongoID := os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_MYSQL_CID"), os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_MONGO_CID")
	if len(mysqlID) != 64 || len(mongoID) != 64 {
		t.Fatal("public_cli_native_exact_service_cids_missing")
	}
	mysqlRoot, e := nativeInspect(context.Background(), mysqlID)
	if e != nil || mysqlRoot.ID != mysqlID || !mysqlRoot.Running {
		t.Fatal("public_cli_native_service_identity_rejected")
	}
	mongoRoot, e := nativeInspect(context.Background(), mongoID)
	if e != nil || mongoRoot.ID != mongoID || !mongoRoot.Running {
		t.Fatal("public_cli_native_service_identity_rejected")
	}
	for _, v := range []nativeContainer{mysqlRoot, mongoRoot} {
		raw, e := publicCLIDocker(context.Background(), "image", "inspect", "--format", "{{.Architecture}}", v.Image)
		if e != nil || strings.TrimSpace(string(raw)) != runtime.GOARCH {
			t.Fatal("public_cli_native_actual_image_architecture_rejected")
		}
	}
	// The provided MySQL service is bound to the exact loopback route before a
	// connection or namespace write. Its CID/image/labels/mounts stay unchanged.
	route := mysqlRoot.Ports["3306/tcp"]
	hasRoute := false
	for _, p := range route {
		if (p.HostIP == "127.0.0.1" || p.HostIP == "0.0.0.0") && p.HostPort == "3306" {
			hasRoute = true
		}
	}
	if !hasRoute {
		t.Fatal("public_cli_native_mysql_service_route_rejected")
	}
	t.Cleanup(func() {
		for _, before := range []nativeContainer{mysqlRoot, mongoRoot} {
			after, e := nativeInspect(context.Background(), before.ID)
			comparison := nativeCompareSharedContainer(before, after)
			if e != nil || !comparison.RawEqual || !comparison.Equal {
				var observed *nativeContainer
				if e == nil {
					observed = &after
				} else {
					comparison.Category = "shared_readback_unavailable"
					comparison.Equal = false
					comparison.RawEqual = false
					comparison.AfterSHA256 = ""
					comparison.NormalizedAfterSHA256 = ""
					comparison.ChangedFields = []string{"readback"}
				}
				path, artifactHash, saveErr := nativeSaveSharedReadback(nativePrivateRoot, source, before, observed)
				if saveErr != nil {
					t.Error("public_cli_native_shared_diagnostic_persistence_failed")
				} else {
					t.Logf("public_cli_native_shared_readback category=%s fields=%s before_sha256=%s after_sha256=%s artifact_sha256=%s artifact_file=%s", comparison.Category, strings.Join(comparison.ChangedFields, ","), comparison.BeforeSHA256, comparison.AfterSHA256, artifactHash, filepath.Base(path))
				}
			}
			if e != nil || !comparison.Equal {
				t.Error("public_cli_native_shared_service_changed")
			}
		}
	})
	publicCLIProtectedDir(t, nativePrivateRoot)
	producerStopped := true       // No public child exists before the actual launch.
	fixtureCleanupAllowed := true // A launched call must earn local acceptance.
	cleanupAllowed := func() bool { return producerStopped && fixtureCleanupAllowed && !t.Failed() }
	private := nativePrivateDirWithCleanupGuard(t, cleanupAllowed)
	retainFixture := func() {
		fixtureCleanupAllowed = false
		nativeRetainCleanup(t, private)
	}
	env := map[string]string{"MYSQL_HOST": "127.0.0.1", "MYSQL_PORT": "3306", "MYSQL_USERNAME": "root", "MYSQL_PASSWORD": "root"}
	nativeStartSourceRSWithCleanupGuard(t, private, mongoRoot, env, cleanupAllowed) // Actual random admin SCRAM user.
	admin := nativeSQL(t, env, "")
	beforeCatalog, e := readSQL(context.Background(), admin, "SELECT SCHEMA_NAME FROM information_schema.schemata ORDER BY SCHEMA_NAME")
	if e != nil {
		t.Fatal("public_cli_native_global_catalog_read_failed")
	}
	name := "qs_lifecycle_public_" + primitive.NewObjectID().Hex()
	nativeExec(t, admin, "CREATE DATABASE "+quote(name)+" CHARACTER SET utf8mb4")
	t.Cleanup(func() {
		if !cleanupAllowed() {
			retainFixture()
			t.Error("public_cli_native_terminal_acceptance_unproven_sql_retained")
			return
		}
		if _, e := admin.ExecContext(context.Background(), "DROP DATABASE "+quote(name)); e != nil {
			retainFixture()
			t.Error("public_cli_native_owned_sql_cleanup_failed")
			return
		}
		after, e := readSQL(context.Background(), admin, "SELECT SCHEMA_NAME FROM information_schema.schemata ORDER BY SCHEMA_NAME")
		if e != nil || !reflect.DeepEqual(beforeCatalog, after) {
			retainFixture()
			t.Error("public_cli_native_global_catalog_changed")
		} else {
			t.Log("public_cli_native_owned_sql_namespace_remaining=0 global_sql_catalog_equal=true")
		}
	})
	db := nativeSQL(t, env, name)
	client := nativeMongo(t, env)
	mdb := client.Database(name)
	nativeSourceSchemas(t, db, mdb)
	nativeSourceData(t, db, mdb)
	env["MYSQL_DATABASE"], env["MONGODB_DBNAME"] = name, name
	before := nativeNamespaceOriginalDigest(t, db, mdb)
	// Never mutate the shared CI services or reuse an earlier root operation.
	base := "/opt/backups/qs-server/compatibility-retirement"
	publicCLIProtectedDir(t, base)
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	op := stamp + "-1"
	boundRun := stamp + "-2"
	invRun := stamp + "-3"
	actualRun := stamp + "-4"
	opDir := filepath.Join(base, op)
	if os.Mkdir(opDir, 0700) != nil {
		t.Fatal("public_cli_native_owned_operation_exists")
	}
	archive := filepath.Join(opDir, "archive")
	root := filepath.Join("/opt/backups/qs-server/compatibility-retirement-root-prepare", op+"-"+actualRun)
	pkg := filepath.Join("/tmp", "qs-compatibility-retirement-"+actualRun+".tar.gz")
	invocation := filepath.Join("/opt/backups/qs-server/compatibility-retirement-invocations", op+"-"+actualRun)
	toolDirectory, e := os.MkdirTemp("/tmp", "qs-independent-window-tool.")
	if e != nil {
		t.Fatal("public_cli_native_window_tool_directory_failed")
	}
	invocationMaterial := map[string]bool{"native-call.intent.private.json": true, "lifecycle-request.json": true, "manifest.json": true}
	toolMaterial := map[string]bool{"compatibility-window-tool.py": true, "receipt-transport.py": true}
	manifestHash := ""
	approval := Approval{}
	opMaterial := map[string]bool{"inventory-cli": true, "boundary-request.json": true, "inventory-request.json": true, "manifest.json": true, "lifecycle-request-template.json": true, "operation.lock": true, "archive": true, "bounds-" + boundRun: true, "inventory-" + invRun: true}
	rootMaterial := map[string]bool{"restore-native": true, "tool.intent.private.json": true, "source-copy.intent.private.json": true, "manifest.json": true, "lifecycle-request.json": true, "lifecycle-restore-" + actualRun + ".registration.private.json": true, "inventory-" + invRun: true}
	zeroName := "lifecycle-restore-" + actualRun + ".zero.private.json"
	rootMaterial[zeroName] = true // Exact preparation producer member, not a glob.
	for _, name := range append([]string{"inventory.private.json", "mysql-metadata.private.json", "mongodb-metadata.private.json"}, sourceNames[:]...) {
		opMaterial[filepath.Join("inventory-"+invRun, name)] = true
		rootMaterial[filepath.Join("inventory-"+invRun, name)] = true
	}
	for _, name := range []string{"boundary.private.json", "mysql-metadata.private.json", "mongodb-metadata.private.json"} {
		opMaterial[filepath.Join("bounds-"+boundRun, name)] = true
	}
	// Registered before the call. A launched but unaccepted operation retains
	// sources and registry: host group absence cannot settle a daemon-side
	// create/exec with an unknown response. Accepted cleanup still observes exact
	// identities and empty actual ExecIDs before every resource deletion.
	cleanupComplete := false
	t.Cleanup(func() {
		if !cleanupAllowed() {
			retainFixture()
			t.Error("public_cli_native_terminal_acceptance_unproven_material_retained")
			return
		}
		// A fatal read also leaves later source cleanup disabled.
		fixtureCleanupAllowed = false
		if final := nativeNamespaceOriginalDigest(t, db, mdb); final != before {
			retainFixture()
			t.Error("public_cli_native_cleanup_source_baseline_changed")
			return
		}
		fixtureCleanupAllowed = true
		n, e := publicCLICleanupEngines(root, archive, source, op, actualRun, manifestHash)
		if e != nil {
			retainFixture()
			t.Error("public_cli_native_exact_restore_cleanup_unresolved")
			return
		}
		if !cleanupComplete && n != 0 {
			retainFixture()
			t.Error("public_cli_native_operation_failed_resources_removed_by_fixture")
			return
		}
		if entries, readErr := os.ReadDir(archive); readErr == nil && len(entries) != 0 {
			if PurgeRegistered(archive, approval) != nil {
				retainFixture()
				t.Error("public_cli_native_fixture_archive_cleanup_unresolved")
				return
			}
		} else if readErr != nil && !os.IsNotExist(readErr) {
			retainFixture()
			t.Error("public_cli_native_fixture_archive_state_unknown")
			return
		}
		for _, pattern := range []string{"restore-*.intent.private.json", "restore-*.created.private.json"} {
			paths, _ := filepath.Glob(filepath.Join(root, pattern))
			for _, p := range paths {
				if strings.HasSuffix(p, ".created.private.json") {
					intent := strings.TrimSuffix(p, ".created.private.json") + ".intent.private.json"
					if _, statErr := os.Lstat(intent); statErr != nil {
						retainFixture()
						t.Error("public_cli_native_unregistered_created_record_retained")
						return
					}
				}
				rootMaterial[filepath.Base(p)] = true
			}
		}
		if publicCLIMaterialsKnown(root, rootMaterial) != nil || publicCLIMaterialsKnown(opDir, opMaterial) != nil || publicCLIMaterialsKnown(invocation, invocationMaterial) != nil || publicCLIMaterialsKnown(toolDirectory, toolMaterial) != nil {
			retainFixture()
			t.Error("public_cli_native_unregistered_material_retained")
			return
		}
		removePackage := os.Remove(pkg)
		if (removePackage != nil && !os.IsNotExist(removePackage)) || os.RemoveAll(root) != nil || os.RemoveAll(opDir) != nil || os.RemoveAll(invocation) != nil || os.RemoveAll(toolDirectory) != nil {
			retainFixture()
			t.Error("public_cli_native_owned_material_cleanup_failed")
			return
		}
		for _, p := range []string{pkg, root, opDir, invocation, toolDirectory} {
			if _, e := os.Lstat(p); !os.IsNotExist(e) {
				retainFixture()
				t.Error("public_cli_native_owned_material_remaining")
				return
			}
		}
		t.Logf("public_cli_native_registered_restore_resources_cleaned=%d owned_temporary_material_remaining=0 production_purge=false", n)
	})
	cli := filepath.Join(opDir, "inventory-cli")
	raw, e := os.ReadFile(built)
	if e != nil || writeLifecycleFixtureBinary(cli, raw) != nil {
		t.Fatal("public_cli_native_root_binary_copy_failed")
	}
	actual, e = nativeCommand(context.Background(), nativeChildEnv(env), cli, "--source-sha")
	if e != nil || strings.TrimSpace(string(actual)) != source {
		t.Fatal("public_cli_native_binary_source_mismatch")
	}
	var report inventory
	approval, report = publicCLIInventory(t, cli, opDir, source, op, boundRun, invRun, env, db, mdb)
	inventoryMaterials, materialErr := publicCLIInventoryMaterialPaths(report, invRun)
	if materialErr != nil {
		t.Fatal("public_cli_native_inventory_material_registration_failed")
	}
	for path := range inventoryMaterials {
		opMaterial[path] = true
	}
	bindings := map[string]any{}
	for database, b := range report.Bindings {
		bindings[database] = map[string]any{"identity_hash": b.IdentityHash, "migration_version": b.Version, "migration_dirty": b.Dirty, "catalog_hash": b.CatalogHash, "non_target_schema_hash": b.NonTargetHash}
	}
	objects := []any{}
	for _, s := range report.Targets {
		objects = append(objects, map[string]any{"database": s.Database, "name": s.Name, "kind": s.Kind, "identity_hash": s.IdentityHash, "schema_hash": s.SchemaHash, "data_hash": s.DataHash, "records": s.Records})
	}
	manifestHash = nativeJSON(t, filepath.Join(opDir, "manifest.json"), map[string]any{"format_version": 1, "operation_id": op, "source_sha": source, "target_hash": report.TargetHash, "database_bindings": bindings, "targets": objects, "evidence": map[string]any{}, "maintenance": map[string]int{"max_seconds": 1800, "forward_stop_seconds": 1200, "rollback_seconds": 600}})
	sourceDir := filepath.Join(opDir, "inventory-"+invRun)
	hashes := map[string]string{}
	for _, name := range append([]string{"inventory.private.json", "mysql-metadata.private.json", "mongodb-metadata.private.json"}, sourceNames[:]...) {
		raw, e := os.ReadFile(filepath.Join(sourceDir, name))
		if e != nil {
			t.Fatal("public_cli_native_exact_source_missing")
		}
		hashes[name] = sha(raw)
	}
	recovery := TargetRecoveryRequest{SourceSHA: source, OperationID: op, OriginalRunID: invRun, ManifestSHA256: manifestHash, MongoNonTargetSHA256: report.Bindings["mongodb"].NonTargetHash, SQLHead: 99, MongoHead: 38}
	template := map[string]any{"format_version": 1, "kind": "compatibility_retirement_lifecycle_request", "tool_source_sha": source, "original_source_sha": source, "operation_id": op, "actual_run_id": "", "manifest_sha256": manifestHash, "archive_directory": archive, "source_directory": sourceDir, "window_directory": filepath.Join(opDir, "window"), "journal_directory": filepath.Join(opDir, "journal"), "archive_approval": approval, "recovery": recovery, "restore_engines": map[string]string{"mysql_image_id": mysqlRoot.Image, "mongodb_image_id": mongoRoot.Image, "architecture": runtime.GOARCH}, "source_file_sha256": hashes}
	templateHash := nativeJSON(t, filepath.Join(opDir, "lifecycle-request-template.json"), template)
	// The expected current request changes only the two approved run fields.
	// Normalize the original struct-containing map into JSON objects before the
	// independently expected canonical bytes, matching the public JSON contract.
	templateBytes, e := os.ReadFile(filepath.Join(opDir, "lifecycle-request-template.json"))
	var expectedRequest map[string]any
	if e != nil || json.Unmarshal(templateBytes, &expectedRequest) != nil {
		t.Fatal("public_cli_native_template_decode_failed")
	}
	expectedRequest["actual_run_id"] = actualRun
	expectedRequest["recovery"].(map[string]any)["actual_run_id"] = actualRun
	derivedBytes, e := json.Marshal(expectedRequest)
	if e != nil {
		t.Fatal("public_cli_native_template_derived_request_failed")
	}
	derivedBytes = append(derivedBytes, '\n')
	requestHash := sha(derivedBytes)
	packageHash, binaryHashes, toolHashes := publicCLIWindowPackage(t, pkg, cli, os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_OTHER_BINARY"), repo, toolDirectory)
	apiImage, apiProgram := publicCLIFixtureAPIImage(t, os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_API_BINARY"), source, op, actualRun, private, cleanupAllowed)
	toolApproval := map[string]any{"format_version": 1, "kind": "independent_compatibility_window_tool_approval", "dispatcher_source_sha": source, "tool_source_sha": source, "original_source_sha": source, "operation_id": op, "original_run_id": invRun, "stage": "prepare", "target_hash": report.TargetHash, "manifest_sha256": manifestHash, "request_template_sha256": templateHash, "tool_binary_sha256": binaryHashes, "b_image_id": apiImage, "b_program_sha256": apiProgram}
	approvalBytes, e := json.Marshal(toolApproval)
	if e != nil {
		t.Fatal("public_cli_native_tool_approval_failed")
	}
	approvalHash := sha(append(append([]byte(nil), approvalBytes...), '\n'))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-B", filepath.Join(toolDirectory, "compatibility-window-tool.py"), "--operation", "prepare", "--operation-id", op, "--dispatcher-sha", source, "--run-id", actualRun, "--manifest-hash", manifestHash, "--template-hash", templateHash)
	child := map[string]string{}
	for k, v := range env {
		child[k] = v
	}
	child["RETIREMENT_PACKAGE_SHA256"] = packageHash
	child["RETIREMENT_BOOTSTRAP_APPROVAL_JSON"] = string(approvalBytes)
	child["RETIREMENT_BOOTSTRAP_APPROVAL_SHA256"] = approvalHash
	cmd.Env = nativeChildEnv(child)
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	started := time.Now()
	producerStopped = false
	fixtureCleanupAllowed = false
	runErr, producerStopped := publicCLIRunProcessGroup(cmd)
	if !producerStopped {
		retainFixture()
	}
	if stdout.Len() > 128<<10 || stderr.Len() > 128<<10 {
		t.Fatal("public_cli_native_output_budget_exceeded")
	}
	// Raw CLI output stays private. Only validated fixed fields enter test logs.
	if writePrivate(filepath.Join(private, "public-cli.stdout.private.log"), stdout.Bytes()) != nil || writePrivate(filepath.Join(private, "public-cli.stderr.private.log"), stderr.Bytes()) != nil {
		t.Fatal("public_cli_native_output_registration_failed")
	}
	if !producerStopped {
		t.Fatal("public_cli_native_host_producer_stop_unproven")
	}
	t.Logf("public_cli_native_output_sha256=%s private_stderr_sha256=%s public_child_reaped=true public_host_group_absent=true public_child_exit_success=%t", sha(stdout.Bytes()), sha(stderr.Bytes()), runErr == nil)
	callResult := publicCLIDecode(t, repo, stdout.Bytes())
	result, resultOK := callResult["native_result"].(map[string]any)
	if !resultOK || callResult["format_version"] != float64(1) || callResult["kind"] != "independent_window_tool_call_result" || callResult["dispatcher_source_sha"] != source || callResult["tool_source_sha"] != source || callResult["approved_template_sha256"] != templateHash || callResult["derived_request_sha256"] != requestHash || len(callResult) != 7 {
		t.Fatal("public_cli_native_window_tool_result_binding_failed")
	}
	if category, ok := result["error_category"].(string); ok && safeNativeCategory(category) {
		t.Log("public_cli_native_result_category=" + category)
	}
	if runErr != nil || result["complete"] != true || result["isolated_content_restore_complete"] != true || result["archive_binding_complete"] != true || result["error_category"] != "none" || result["source_sha"] != source || result["original_source_sha"] != source || result["operation_id"] != op || result["run_id"] != actualRun || result["request_sha256"] != requestHash || result["manifest_sha256"] != manifestHash || result["target_count"] != float64(4) {
		t.Fatal("public_cli_native_actual_prepare_failed")
	}
	for _, k := range []string{"execution_allowed", "drop_ready", "recovery_attempted", "recovery_complete", "acceptance_complete", "purge_complete"} {
		if result[k] != false {
			t.Fatal("public_cli_native_effect_authority_changed")
		}
	}
	elapsed, ok := result["restore_elapsed_millis"].(float64)
	if !ok || elapsed <= 0 || elapsed > 600000 {
		t.Fatal("public_cli_native_combined_restore_budget_failed")
	}
	zeroHash, ok := result["preparation_restore_zero_sha256"].(string)
	zeroFile, zeroOpenErr := os.OpenFile(filepath.Join(root, zeroName), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if !ok || !hashPattern.MatchString(zeroHash) || zeroOpenErr != nil {
		if zeroFile != nil {
			_ = zeroFile.Close()
		}
		t.Fatal("public_cli_native_preparation_zero_binding_failed")
	}
	zeroBefore, zeroStatErr := zeroFile.Stat()
	if zeroStatErr != nil || !zeroBefore.Mode().IsRegular() || zeroBefore.Mode().Perm() != 0600 || zeroBefore.Sys().(*syscall.Stat_t).Uid != 0 || zeroBefore.Sys().(*syscall.Stat_t).Nlink != 1 || zeroBefore.Size() <= 0 || zeroBefore.Size() > 256<<10 {
		_ = zeroFile.Close()
		t.Fatal("public_cli_native_preparation_zero_binding_failed")
	}
	zeroRaw, zeroReadErr := io.ReadAll(io.LimitReader(zeroFile, (256<<10)+1))
	zeroAfter, zeroStatErr := zeroFile.Stat()
	zeroCloseErr := zeroFile.Close()
	var zero struct {
		FormatVersion     int               `json:"format_version"`
		Kind              string            `json:"kind"`
		OriginalSourceSHA string            `json:"original_source_sha"`
		ToolSourceSHA     string            `json:"tool_source_sha"`
		OperationID       string            `json:"operation_id"`
		OriginalRunID     string            `json:"original_run_id"`
		ActualRunID       string            `json:"actual_run_id"`
		ManifestSHA256    string            `json:"manifest_sha256"`
		ArchiveSHA256     string            `json:"archive_sha256"`
		RequestSHA256     string            `json:"request_sha256"`
		ElapsedMillis     int64             `json:"elapsed_millis"`
		Engines           []json.RawMessage `json:"engines"`
		Files             []json.RawMessage `json:"files"`
	}
	if zeroReadErr != nil || zeroStatErr != nil || zeroCloseErr != nil || !os.SameFile(zeroBefore, zeroAfter) || zeroBefore.Size() != zeroAfter.Size() || zeroBefore.ModTime() != zeroAfter.ModTime() || int64(len(zeroRaw)) != zeroBefore.Size() || sha(zeroRaw) != zeroHash || exactJSON(zeroRaw, &zero) != nil || zero.FormatVersion != 1 || zero.Kind != "original_preparation_isolated_restore_zero" || zero.OriginalSourceSHA != source || zero.ToolSourceSHA != source || zero.OperationID != op || zero.OriginalRunID != invRun || zero.ActualRunID != actualRun || zero.ManifestSHA256 != manifestHash || zero.ArchiveSHA256 != result["archive_sha256"] || zero.RequestSHA256 != requestHash || zero.ElapsedMillis <= 0 || float64(zero.ElapsedMillis) > elapsed || len(zero.Engines) != 2 || len(zero.Files) != 5 {
		t.Fatal("public_cli_native_preparation_zero_binding_failed")
	}
	after := nativeNamespaceOriginalDigest(t, db, mdb)
	if before != after {
		t.Fatal("public_cli_native_original_catalog_or_content_changed")
	}
	for name, expected := range hashes {
		for _, directory := range []string{sourceDir, filepath.Join(root, "inventory-"+invRun)} {
			actual, readErr := os.ReadFile(filepath.Join(directory, name))
			if readErr != nil || sha(actual) != expected {
				t.Fatal("public_cli_native_exact_original_or_staged_source_changed")
			}
		}
	}
	for path, expected := range map[string]string{
		filepath.Join(opDir, "manifest.json"):                   manifestHash,
		filepath.Join(root, "manifest.json"):                    manifestHash,
		filepath.Join(opDir, "lifecycle-request-template.json"): templateHash,
		filepath.Join(invocation, "lifecycle-request.json"):     requestHash,
		filepath.Join(invocation, "manifest.json"):              manifestHash,
		filepath.Join(root, "lifecycle-request.json"):           requestHash,
		filepath.Join(root, zeroName):                           zeroHash,
		pkg:                                                     packageHash,
	} {
		actual, readErr := os.ReadFile(path)
		if readErr != nil || sha(actual) != expected {
			t.Fatal("public_cli_native_immutable_descriptor_changed")
		}
	}
	var actualIntent publicCLIWindowIntent
	if publicCLIPrivateJSON(filepath.Join(invocation, "native-call.intent.private.json"), &actualIntent) != nil || actualIntent != (publicCLIWindowIntent{Format: 1, Kind: "independent_window_tool_native_invocation", Dispatcher: source, Tool: source, Original: source, Operation: op, OriginalRun: invRun, ActualRun: actualRun, Stage: "prepare", Template: templateHash, Request: requestHash, Manifest: manifestHash, Package: packageHash, ToolDirectory: toolDirectory, ToolProgram: toolHashes["compatibility-window-tool.py"], Native: binaryHashes[runtime.GOARCH], NativePath: filepath.Join(root, "restore-native"), BImage: apiImage, BProgram: apiProgram, SourceUID: 0, Drop: false}) {
		t.Fatal("public_cli_native_window_tool_intent_binding_failed")
	}
	callIntent, e := os.ReadFile(filepath.Join(invocation, "native-call.intent.private.json"))
	rootIntent, rootIntentErr := os.ReadFile(filepath.Join(root, "tool.intent.private.json"))
	if e != nil || rootIntentErr != nil || !bytes.Equal(callIntent, rootIntent) {
		t.Fatal("public_cli_native_window_tool_intents_changed")
	}
	for name, expected := range toolHashes {
		raw, e := os.ReadFile(filepath.Join(toolDirectory, name))
		if e != nil || sha(raw) != expected {
			t.Fatal("public_cli_native_packaged_program_changed")
		}
	}
	if publicCLIMaterialsKnown(invocation, invocationMaterial) != nil || publicCLIMaterialsKnown(toolDirectory, toolMaterial) != nil {
		t.Fatal("public_cli_native_invocation_material_metadata_rejected")
	}
	// OpenArchive verifies actual registered files to EOF again. The fixture
	// destroys this exact four-object archive only after its local acceptance.
	hash, ok := result["archive_sha256"].(string)
	if !ok {
		t.Fatal("public_cli_native_archive_hash_missing")
	}
	opened, e := OpenArchive(context.Background(), archive, hash)
	if e != nil || opened.Summary().TargetCount != 4 || opened.Summary().DropReady {
		t.Fatal("public_cli_native_registered_archive_reopen_failed")
	}
	n, e := publicCLICleanupEngines(root, archive, source, op, actualRun, manifestHash)
	if e != nil || n != 2 {
		t.Fatal("public_cli_native_exact_restore_cleanup_failed")
	}
	// Remove engine registry only after actual resource absence; keep the once
	// root intent and source hashes until the last source-baseline comparison.
	for _, pattern := range []string{"restore-*.intent.private.json", "restore-*.created.private.json"} {
		paths, _ := filepath.Glob(filepath.Join(root, pattern))
		for _, p := range paths {
			if os.Remove(p) != nil {
				t.Fatal("public_cli_native_fixture_registry_cleanup_failed")
			}
		}
	}
	if e = PurgeRegistered(archive, approval); e != nil {
		t.Fatal("public_cli_native_fixture_archive_cleanup_failed")
	}
	entries, e := os.ReadDir(archive)
	if e != nil || len(entries) != 0 {
		t.Fatal("public_cli_native_registered_archive_assets_remaining")
	}
	if final := nativeNamespaceOriginalDigest(t, db, mdb); final != before {
		t.Fatal("public_cli_native_final_source_baseline_changed")
	}
	cleanupComplete = true
	fixtureCleanupAllowed = true
	t.Logf("public_cli_native_source_sha=%s operation_id=%s actual_run_id=%s manifest_sha256=%s request_sha256=%s archive_sha256=%s original_source_baseline_sha256=%s final_source_baseline_sha256=%s", source, op, actualRun, manifestHash, requestHash, hash, before, after)
	t.Logf("public_cli_native_prepare_complete=true actual_window_tool=true actual_template_run_derivation=true actual_root_once=true actual_inventory_eof=true actual_stdio_restore=true target_count=4 combined_restore_millis=%d public_call_millis=%d original_catalog_and_content_equal=true engines_remaining=0 volumes_remaining=0 archive_assets_remaining=0 production_drop=false production_purge=false", int64(elapsed), time.Since(started).Milliseconds())
}

func writeLifecycleFixtureBinary(path string, raw []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if e != nil {
		return e
	}
	_, e = f.Write(raw)
	se, ce := f.Sync(), f.Close()
	if e != nil {
		return e
	}
	if se != nil {
		return se
	}
	return ce
}

func TestLifecyclePublicCLICleanupRejectsLiveExecAndForeignMount(t *testing.T) {
	r := publicCLIRestoreIntent{Name: "test", Image: "sha256:" + strings.Repeat("a", 64), Volumes: []string{"owned"}, ContainerLabels: map[string]string{"owner": "self"}}
	var v publicCLIInspection
	v.ID = strings.Repeat("b", 64)
	v.Name = "/test"
	v.Image = r.Image
	v.Config.Labels = r.ContainerLabels
	v.HostConfig.NetworkMode = "none"
	v.ExecIDs = json.RawMessage("null")
	// The existing runtime matcher rejects incomplete/mutated mount sets.
	if publicCLIRuntimeMatches(v, r, v.ID, "/tool", "/archive") {
		t.Fatal("public_cli_cleanup_missing_mounts_accepted")
	}
	v.Mounts = append(v.Mounts, struct {
		Type, Name, Source, Destination string
		RW                              bool
	}{Type: "volume", Name: "owned", Destination: "/var/lib/mysql", RW: true}, struct {
		Type, Name, Source, Destination string
		RW                              bool
	}{Type: "bind", Source: "/tool", Destination: "/tool/restore-native"}, struct {
		Type, Name, Source, Destination string
		RW                              bool
	}{Type: "bind", Source: "/archive", Destination: "/backup"})
	if !publicCLIRuntimeMatches(v, r, v.ID, "/tool", "/archive") {
		t.Fatal("public_cli_cleanup_actual_empty_exec_binding_rejected")
	}
	v.ExecIDs = json.RawMessage(`["active"]`)
	if publicCLIRuntimeMatches(v, r, v.ID, "/tool", "/archive") {
		t.Fatal("public_cli_cleanup_live_exec_accepted")
	}
	v.ExecIDs = json.RawMessage("null")
	v.Mounts[2].Source = "/foreign"
	if publicCLIRuntimeMatches(v, r, v.ID, "/tool", "/archive") {
		t.Fatal("public_cli_cleanup_foreign_mount_accepted")
	}
}

type publicCLIReadyOutput struct {
	mu    sync.Mutex
	raw   bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (w *publicCLIReadyOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.raw.Write(p)
	if strings.Contains(w.raw.String(), "public_cli_leaf_ready\n") {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}

func (w *publicCLIReadyOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.raw.String()
}

// This is an actual unprivileged three-process host chain, not a simulated
// native/DB restore proof. Both descendant layers inherit the dedicated group.
func TestLifecyclePublicCLIProcessGroupTimeoutReapsTwoDescendantLayers(t *testing.T) {
	script := `import os,signal,subprocess,sys,time
depth=int(sys.argv[1]); child=None
def stopped(signum,frame):
    if child is not None:
        try: child.wait(timeout=2)
        except subprocess.TimeoutExpired:
            child.kill();child.wait(timeout=2)
    raise SystemExit(0)
signal.signal(signal.SIGTERM,stopped)
print('public_cli_pid='+str(os.getpid())+' depth='+str(depth),flush=True)
if depth:
    child=subprocess.Popen([sys.executable,'-I','-c',sys.argv[2],str(depth-1),sys.argv[2]])
else:
    print('public_cli_leaf_ready',flush=True)
while True: time.sleep(1)
`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-I", "-c", script, "2", script)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out := &publicCLIReadyOutput{ready: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = out, io.Discard
	runErr, stopped := publicCLIRunProcessGroup(cmd)
	select {
	case <-out.ready:
	default:
		t.Fatal("public_cli_timeout_test_descendant_layers_not_started")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) || runErr == nil || !stopped || cmd.Process == nil {
		t.Fatal("public_cli_timeout_test_actual_group_stop_unproven")
	}
	if absent, err := publicCLIHostGroupAbsent(cmd.Process.Pid); err != nil || !absent {
		t.Fatal("public_cli_timeout_test_group_remained")
	}
	seen := map[int]bool{}
	for _, line := range strings.Split(out.String(), "\n") {
		if !strings.HasPrefix(line, "public_cli_pid=") {
			continue
		}
		parts := strings.Fields(line)
		pid, err := strconv.Atoi(strings.TrimPrefix(parts[0], "public_cli_pid="))
		if err != nil || pid < 2 || seen[pid] || !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			t.Fatal("public_cli_timeout_test_actual_descendant_not_reaped")
		}
		seen[pid] = true
	}
	if len(seen) != 3 {
		t.Fatal("public_cli_timeout_test_two_descendant_layers_missing")
	}
}

func TestLifecyclePublicCLIProcessGroupBindingAndNormalExit(t *testing.T) {
	for _, group := range []int{-1, 0, 1} {
		if absent, err := publicCLIHostGroupAbsent(group); absent || err == nil {
			t.Fatal("public_cli_unbound_group_accepted")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-I", "-c", "pass")
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	if err, stopped := publicCLIRunProcessGroup(cmd); err != nil || !stopped || cmd.Process == nil {
		t.Fatal("public_cli_normal_child_group_absence_unproven")
	}
	if absent, err := publicCLIHostGroupAbsent(cmd.Process.Pid); err != nil || !absent {
		t.Fatal("public_cli_normal_child_group_remained")
	}
}

func TestLifecyclePublicCLIWindowInvocationRegistryShape(t *testing.T) {
	// Exact real producer shape, not a substitute for the privileged caller.
	raw := []byte(`{"format_version":1,"kind":"independent_window_tool_native_invocation","dispatcher_source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tool_source_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","original_source_sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","operation_id":"10-1","original_run_id":"10-3","actual_run_id":"10-4","stage":"prepare","approved_template_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","derived_request_sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","manifest_sha256":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","package_sha256":"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff","tool_directory":"/tmp/qs-independent-window-tool.ABCDEF","tool_program_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","native_sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","native_path":"/opt/backups/qs-server/compatibility-retirement-root-prepare/10-1-10-4/restore-native","b_image_id":"","b_program_sha256":"","source_uid":0,"drop_authority":false}`)
	var got publicCLIWindowIntent
	if exactJSON(raw, &got) != nil || got.OriginalRun != "10-3" || got.ActualRun != "10-4" || got.SourceUID != 0 || got.Drop {
		t.Fatal("public_cli_window_invocation_exact_registry_rejected")
	}
	for _, mutation := range []string{
		strings.Replace(string(raw), `"source_uid"`, `"Source_UID"`, 1),
		strings.TrimSuffix(string(raw), "}") + `,"unregistered":true}`,
		strings.TrimSuffix(string(raw), "}") + `,"actual_run_id":"overwritten"}`,
	} {
		if exactJSON([]byte(mutation), new(publicCLIWindowIntent)) == nil {
			t.Fatal("public_cli_window_invocation_unknown_alias_or_duplicate_accepted")
		}
	}
}

func TestLifecyclePublicCLILinuxRootControlLossReapsDetachedDescendants(t *testing.T) {
	run, e := publicCLIOptIn(os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_NATIVE"), os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_NATIVE_REQUIRED"))
	if e != nil {
		t.Fatal(e)
	}
	if !run {
		t.Skip("public lifecycle Linux root fixture not requested")
	}
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || os.Getenv("SUDO_UID") != "" {
		t.Fatal("public_cli_owned_process_actual_linux_root_required")
	}
	repo, source := os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_REPOSITORY"), os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_SOURCE_SHA")
	if !filepath.IsAbs(repo) || !sourcePattern.MatchString(source) {
		t.Fatal("public_cli_owned_process_source_missing")
	}
	actual, e := nativeCommand(context.Background(), []string{"PATH=/usr/bin:/bin"}, "/usr/bin/git", "-c", "safe.directory="+repo, "-C", repo, "rev-parse", "HEAD")
	if e != nil || strings.TrimSpace(string(actual)) != source {
		t.Fatal("public_cli_owned_process_actual_checkout_mismatch")
	}
	private, e := os.MkdirTemp("/tmp", "qs-owned-process-native.")
	if e != nil {
		t.Fatal("public_cli_owned_process_private_directory_failed")
	}
	accepted := false
	t.Cleanup(func() {
		// No automatic RemoveAll: unknown producer/result/material is retained,
		// including if registering a disk checkpoint would itself fail.
		if !accepted || t.Failed() {
			t.Error("public_cli_owned_process_unaccepted_material_retained")
			return
		}
		if publicCLIMaterialsKnown(private, map[string]bool{"intent.private.json": true, "children.private.log": true}) != nil {
			t.Error("public_cli_owned_process_unregistered_material_retained")
			return
		}
		for _, name := range []string{"children.private.log", "intent.private.json"} {
			if os.Remove(filepath.Join(private, name)) != nil {
				t.Error("public_cli_owned_process_material_cleanup_failed")
				return
			}
		}
		if os.Remove(private) != nil {
			t.Error("public_cli_owned_process_private_directory_cleanup_failed")
			return
		}
		if _, e := os.Lstat(private); !os.IsNotExist(e) {
			t.Error("public_cli_owned_process_material_remaining")
			return
		}
		t.Log("public_cli_owned_process_accepted_private_material_remaining=0 production_operations=false")
	})
	intent := []byte("{\"format_version\":1,\"kind\":\"owned_process_control_loss_fixture\",\"drop_authority\":false}\n")
	if writePrivate(filepath.Join(private, "intent.private.json"), intent) != nil {
		t.Fatal("public_cli_owned_process_intent_failed")
	}
	script := `import importlib.util,json,os,signal,subprocess,sys,time
spec=importlib.util.spec_from_file_location('actual_owned',sys.argv[1]);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
owner=m.LinuxChildOwner()
child_script=r'''import os,signal,subprocess,sys,time
signal.signal(signal.SIGTERM,signal.SIG_IGN)
fd=os.open(sys.argv[2],os.O_WRONLY|os.O_CREAT|os.O_APPEND|os.O_NOFOLLOW,0o600)
os.write(fd,(str(os.getpid())+'\n').encode());os.fsync(fd);os.close(fd)
if int(sys.argv[1]):subprocess.Popen([sys.executable,'-I','-c',sys.argv[3],str(int(sys.argv[1])-1),sys.argv[2],sys.argv[3]],start_new_session=True)
while True:time.sleep(.02)
'''
try:m.owned_process([sys.executable,'-I','-c',child_script,'2',sys.argv[2],child_script],{'PATH':'/usr/bin:/bin'},control=0,owner=owner,timeout=90)
except m.Refused as e:
 if str(e)!='window_tool_local_execution_unknown':raise
else:raise SystemExit(4)
owner.reap_adopted()
if owner.descendants():raise SystemExit(5)
with open(sys.argv[2],'r',encoding='ascii') as f:pids=[int(line.strip()) for line in f]
if len(set(pids))!=3:raise SystemExit(6)
for pid in pids:
 try:os.kill(pid,0)
 except ProcessLookupError:pass
 else:raise SystemExit(7)
print(json.dumps({'actual_root':os.getuid()==0 and os.geteuid()==0,'control_eof':True,'actual_adopted_descendants_remaining':len(owner.descendants()),'actual_pids':pids,'drop_authority':False},sort_keys=True),flush=True)
`
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-I", "-c", script, filepath.Join(repo, "scripts/database/compatibility-window-tool.py"), filepath.Join(private, "children.private.log"))
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	control, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal("public_cli_owned_process_actual_control_pipe_failed")
	}
	defer func() { _ = control.Close() }()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	type terminal struct {
		err     error
		stopped bool
	}
	done := make(chan terminal, 1)
	go func() { err, stopped := publicCLIRunProcessGroup(cmd); done <- terminal{err, stopped} }()
	var observed []int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		raw, e := os.ReadFile(filepath.Join(private, "children.private.log"))
		if e == nil && len(raw) < 1024 {
			fields := strings.Fields(string(raw))
			observed = nil
			for _, f := range fields {
				pid, e := strconv.Atoi(f)
				if e != nil || pid < 2 {
					t.Fatal("public_cli_owned_process_actual_pid_invalid")
				}
				observed = append(observed, pid)
			}
			if len(observed) == 3 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Actual EOF on the root supervisor's live control pipe requests stop.
	disconnected := time.Now()
	if e = control.Close(); e != nil {
		t.Fatal("public_cli_owned_process_control_disconnect_failed")
	}
	outcome := <-done
	if time.Since(disconnected) >= 20*time.Second || len(observed) != 3 || outcome.err != nil || !outcome.stopped || stdout.Len() > 32768 || stderr.Len() > 32768 {
		t.Fatal("public_cli_owned_process_control_loss_terminal_unproven")
	}
	var proof struct {
		Root      bool  `json:"actual_root"`
		EOF       bool  `json:"control_eof"`
		Remaining int   `json:"actual_adopted_descendants_remaining"`
		PIDs      []int `json:"actual_pids"`
		Drop      bool  `json:"drop_authority"`
	}
	if exactJSON(stdout.Bytes(), &proof) != nil || !proof.Root || !proof.EOF || proof.Remaining != 0 || proof.Drop || !reflect.DeepEqual(proof.PIDs, observed) {
		t.Fatal("public_cli_owned_process_actual_root_proof_rejected")
	}
	// Independently observe absence after the real supervisor has returned;
	// a JSON count alone is not evidence that the privileged descendants died.
	for _, pid := range observed {
		if !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			t.Fatal("public_cli_owned_process_descendant_still_present")
		}
	}
	actualIntent, e := os.ReadFile(filepath.Join(private, "intent.private.json"))
	if e != nil || !bytes.Equal(intent, actualIntent) {
		t.Fatal("public_cli_owned_process_failure_intent_changed")
	}
	accepted = true
	t.Logf("public_cli_owned_process_source_sha=%s actual_linux_root=true actual_subreaper=true actual_pidfd=true actual_control_eof=true detached_descendant_layers=3 actual_descendants_remaining=0 failure_intent_retained_until_acceptance=true daemon_exec_proven=false production_operations=false", source)
}

// This opt-in native fixture is an inert, source-bound service process, not a
// business Worker. Docker supplies its actual CID/image/PID/start time. It uses
// the production stop/recovery constructors without imported opaque authority.
func init() {
	if len(os.Args) != 2 || os.Args[1] != "--config=/app/configs/worker.prod.yaml" || os.Getenv("QS_PUBLIC_RECOVERY_WORKER") == "" {
		return
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || os.Getenv("QS_PUBLIC_RECOVERY_WORKER") != buildversion.GitCommit || !sourcePattern.MatchString(buildversion.GitCommit) {
		os.Exit(93)
	}
	ended := make(chan os.Signal, 1)
	signal.Notify(ended, syscall.SIGTERM)
	select {
	case <-ended:
		signal.Stop(ended)
		os.Exit(0)
	case <-time.After(5 * time.Minute):
		os.Exit(94)
	}
}

type publicCLIServiceRecoveryInput struct {
	DescriptorPath, DescriptorSHA256, Directory string
	WindowBinding                               fence.WindowBinding
}

func publicCLIServiceRecoveryPhase(t *testing.T, path, phase string) {
	t.Helper()
	var h publicCLIServiceRecoveryInput
	if publicCLIPrivateJSON(path, &h) != nil || (phase != "stop" && phase != "recover") {
		t.Fatal("public_cli_service_recovery_handoff_rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	a, e := stop.ReadApprovedDescriptor(h.DescriptorPath, h.DescriptorSHA256)
	if e != nil {
		t.Fatal("public_cli_service_recovery_actual_approval_rejected")
	}
	windowDir, journal := filepath.Join(h.Directory, "window"), filepath.Join(h.Directory, "service-journal")
	var w *fence.MaintenanceWindow
	if phase == "stop" {
		w, e = fence.StartMaintenanceWindow(ctx, windowDir, h.WindowBinding)
	} else {
		w, e = fence.OpenMaintenanceWindow(ctx, windowDir, h.WindowBinding)
	}
	if e != nil {
		t.Fatal("public_cli_service_recovery_actual_window_rejected")
	}
	t.Cleanup(func() {
		if w.Close() != nil {
			t.Error("public_cli_service_recovery_window_close_failed")
		}
	})
	var lease *stop.Lease
	if phase == "stop" {
		lease, e = stop.StopAndDrain(ctx, a, journal, w)
	} else {
		q, finish, recoveryErr := w.RecoveryContext(ctx)
		if recoveryErr != nil || q.Err() != nil {
			t.Fatal("public_cli_service_recovery_budget_rejected")
		}
		finish()
		lease, e = stop.OpenRecoveryDependents(ctx, a, journal, w)
	}
	if e != nil || lease == nil {
		t.Fatal("public_cli_service_recovery_original_native_owner_rejected")
	}
	t.Cleanup(func() {
		if lease.Close() != nil {
			t.Error("public_cli_service_recovery_owner_close_failed")
		}
	})
	if phase == "recover" {
		if lease.Check(ctx) == nil || lease.CheckRecoveryStopped(ctx) != nil {
			t.Fatal("public_cli_service_recovery_only_guard_failed")
		}
		if lease.RestoreDependents(ctx) != nil {
			t.Fatal("public_cli_service_recovery_actual_restore_failed")
		}
	}
	d, e := w.Diagnostic(ctx)
	if e != nil || d.MutationAllowed || d.DropReady {
		t.Fatal("public_cli_service_recovery_window_overclaimed")
	}
	nativeJSON(t, filepath.Join(h.Directory, phase+".receipt.private.json"), map[string]any{"pid": os.Getpid(), "stage": phase, "window_start_sha256": d.StartSHA256, "recovery_sha256": d.RecoverySHA256, "drop_ready": false})
}

func TestLifecyclePublicCLINativeCrossProcessServiceRecovery(t *testing.T) {
	run, e := publicCLIOptIn(os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_NATIVE"), os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_NATIVE_REQUIRED"))
	if e != nil {
		t.Fatal(e)
	}
	if !run {
		t.Skip("public lifecycle native fixture not requested")
	}
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || os.Getenv("SUDO_UID") != "" {
		t.Fatal("public_cli_service_recovery_actual_linux_root_required")
	}
	if input := os.Getenv("QS_PUBLIC_SERVICE_RECOVERY_INPUT"); input != "" {
		publicCLIServiceRecoveryPhase(t, input, os.Getenv("QS_PUBLIC_SERVICE_RECOVERY_PHASE"))
		return
	}
	source, repo := os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_SOURCE_SHA"), os.Getenv("QS_LIFECYCLE_PUBLIC_CLI_REPOSITORY")
	actual, e := nativeCommand(context.Background(), []string{"PATH=/usr/bin:/bin"}, "/usr/bin/git", "-c", "safe.directory="+repo, "-C", repo, "rev-parse", "HEAD")
	if e != nil || !sourcePattern.MatchString(source) || strings.TrimSpace(string(actual)) != source || buildversion.GitCommit != source {
		t.Fatal("public_cli_service_recovery_compiled_source_mismatch")
	}
	programPath, e := os.Executable()
	if e != nil {
		t.Fatal("public_cli_service_recovery_executable_unknown")
	}
	program, e := os.ReadFile(programPath)
	elfFile, elfErr := elf.NewFile(bytes.NewReader(program))
	if e != nil || elfErr != nil || len(program) == 0 || len(program) > 256<<20 {
		t.Fatal("public_cli_service_recovery_static_program_rejected")
	}
	for _, segment := range elfFile.Progs {
		if segment.Type == elf.PT_INTERP {
			t.Fatal("public_cli_service_recovery_dynamic_program_rejected")
		}
	}
	if elfFile.Close() != nil {
		t.Fatal("public_cli_service_recovery_program_close_failed")
	}
	base := "/opt/backups/qs-server/compatibility-retirement"
	publicCLIProtectedDir(t, base)
	op := strconv.FormatInt(time.Now().UnixNano(), 10) + "-1"
	dir := filepath.Join(base, op)
	for _, path := range []string{dir, filepath.Join(dir, "window"), filepath.Join(dir, "service-journal")} {
		if os.Mkdir(path, 0700) != nil {
			t.Fatal("public_cli_service_recovery_owned_directory_rejected")
		}
	}
	owner := source + "/" + op
	name := "qs-public-recovery-worker-" + op
	image, cid := "", ""
	settled := false
	checkImage := func() bool {
		raw, err := publicCLIDocker(context.Background(), "image", "inspect", "--format", `{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}},"revision":{{json (index .Config.Labels "org.opencontainers.image.revision")}},"owner":{{json (index .Config.Labels "codex.public-recovery-owner")}}}`, image)
		var v struct{ ID, OS, Architecture, Revision, Owner string }
		return err == nil && json.Unmarshal(raw, &v) == nil && v.ID == image && v.OS == "linux" && v.Architecture == runtime.GOARCH && v.Revision == source && v.Owner == owner
	}
	const format = `{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"component":{{json (index .Config.Labels "prometheus.component")}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"service":{{json (index .Config.Labels "com.docker.compose.service")}},"entrypoint":{{json .Config.Entrypoint}},"command":{{json .Config.Cmd}},"running":{{json .State.Running}},"started_at":{{json .State.StartedAt}},"restart_policy":{{json .HostConfig.RestartPolicy.Name}},"restart_maximum":{{json .HostConfig.RestartPolicy.MaximumRetryCount}},"owner":{{json (index .Config.Labels "codex.public-recovery-owner")}},"pid":{{json .State.Pid}},"exit_code":{{json .State.ExitCode}},"readonly":{{json .HostConfig.ReadonlyRootfs}},"network":{{json .HostConfig.NetworkMode}},"mounts":{{json .Mounts}},"execs":{{json .ExecIDs}}}`
	inspect := func() (stop.Container, int, int, error) {
		raw, err := publicCLIDocker(context.Background(), "inspect", "--format", format, cid)
		var v struct {
			stop.Container
			Owner, Network string
			PID, ExitCode  int
			Readonly       bool
			Mounts, Execs  []json.RawMessage
		}
		if err != nil || json.Unmarshal(raw, &v) != nil || v.ID != cid || v.Image != image || v.Name != "/"+name || v.Owner != owner || !v.Readonly || v.Network != "none" || len(v.Mounts) != 0 || len(v.Execs) != 0 {
			return stop.Container{}, 0, 0, ErrIsolation
		}
		return v.Container, v.PID, v.ExitCode, nil
	}
	t.Cleanup(func() {
		if !settled || t.Failed() {
			nativeRetainCleanup(t, dir)
			return
		}
		if _, _, _, err := inspect(); err != nil || !checkImage() {
			t.Error("public_cli_service_recovery_cleanup_identity_unproven")
			return
		}
		if _, err := publicCLIDocker(context.Background(), "rm", "--force", cid); err != nil {
			t.Error("public_cli_service_recovery_owned_remove_unknown")
			return
		}
		remaining, err := publicCLIDocker(context.Background(), "ps", "--all", "--no-trunc", "--filter", "label=codex.public-recovery-owner="+owner, "--format", "{{.ID}}")
		if err != nil || len(bytes.TrimSpace(remaining)) != 0 {
			t.Error("public_cli_service_recovery_owned_zero_unproven")
			return
		}
		if _, err = publicCLIDocker(context.Background(), "image", "rm", "--no-prune", image); err != nil || os.RemoveAll(dir) != nil {
			t.Error("public_cli_service_recovery_owned_material_cleanup_failed")
			return
		}
		remaining, err = publicCLIDocker(context.Background(), "image", "ls", "--quiet", "--no-trunc", "--filter", "label=codex.public-recovery-owner="+owner)
		if err != nil || len(bytes.TrimSpace(remaining)) != 0 {
			t.Error("public_cli_service_recovery_owned_image_zero_unproven")
		} else {
			t.Log("public_cli_service_recovery_owned_containers_remaining=0 owned_images_remaining=0")
		}
	})
	nativeJSON(t, filepath.Join(dir, "fixture.intent.private.json"), map[string]string{"source_sha": source, "operation_id": op, "owner": owner, "name": name, "program_sha256": sha(program)})
	var contextTar bytes.Buffer
	tw := tar.NewWriter(&contextTar)
	for _, member := range []struct {
		name string
		raw  []byte
		mode int64
	}{
		{"Dockerfile", []byte("FROM scratch\nLABEL org.opencontainers.image.revision=" + source + "\nLABEL codex.public-recovery-owner=" + owner + "\nCOPY worker /app/qs-worker\n"), 0600},
		{"worker", program, 0555},
	} {
		if tw.WriteHeader(&tar.Header{Name: member.name, Mode: member.mode, Size: int64(len(member.raw)), Typeflag: tar.TypeReg}) != nil {
			t.Fatal("public_cli_service_recovery_context_rejected")
		}
		if _, err := tw.Write(member.raw); err != nil {
			t.Fatal("public_cli_service_recovery_context_rejected")
		}
	}
	if tw.Close() != nil {
		t.Fatal("public_cli_service_recovery_context_rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "/usr/bin/docker", "--host", "unix:///run/docker.sock", "build", "--quiet", "--pull=false", "--network=none", "-")
	build.Env, build.Stdin, build.Stderr = []string{"PATH=/usr/bin:/bin"}, &contextTar, io.Discard
	built, e := build.Output()
	image = strings.TrimSpace(string(built))
	if e != nil || !strings.HasPrefix(image, "sha256:") || !hashPattern.MatchString(strings.TrimPrefix(image, "sha256:")) || !checkImage() {
		t.Fatal("public_cli_service_recovery_image_unknown")
	}
	created, e := publicCLIDocker(ctx, "create", "--name", name, "--label", "codex.public-recovery-owner="+owner, "--label", "prometheus.component=qs-worker", "--label", "com.docker.compose.project=qs-worker", "--label", "com.docker.compose.service=runtime", "--network", "none", "--read-only", "--restart", "unless-stopped", "--env", "QS_PUBLIC_RECOVERY_WORKER="+source, "--entrypoint", "/app/qs-worker", image, "--config=/app/configs/worker.prod.yaml")
	cid = strings.TrimSpace(string(created))
	if e != nil || !hashPattern.MatchString(cid) {
		t.Fatal("public_cli_service_recovery_creation_unknown")
	}
	nativeJSON(t, filepath.Join(dir, "fixture.created.private.json"), map[string]string{"container_id": cid, "image_id": image, "owner": owner})
	if _, e = publicCLIDocker(ctx, "start", cid); e != nil {
		t.Fatal("public_cli_service_recovery_start_unknown")
	}
	original, pid, _, e := inspect()
	if e != nil || !original.Running || pid <= 0 {
		t.Fatal("public_cli_service_recovery_original_native_runtime_rejected")
	}
	machine, e := os.ReadFile("/etc/machine-id")
	dockerProgram, dockerErr := os.ReadFile("/usr/bin/docker")
	if e != nil || dockerErr != nil || len(machine) == 0 || len(dockerProgram) == 0 {
		t.Fatal("public_cli_service_recovery_host_identity_unknown")
	}
	manifest := sha([]byte(owner + "/local-service-recovery-only"))
	descriptor := stop.Descriptor{Version: 1, SourceSHA: source, RuntimeSourceSHA: source, ToolSourceSHA: source, OriginalRunID: op, OperationID: op, ManifestSHA256: manifest, HostRole: "server-d", MachineIDSHA256: sha(bytes.TrimSpace(machine)), DockerPath: "/usr/bin/docker", DockerSHA256: sha(dockerProgram), Containers: []stop.Container{original}}
	descriptorPath := filepath.Join(dir, "approved-services.json")
	descriptorSHA := nativeJSON(t, descriptorPath, descriptor)
	input := publicCLIServiceRecoveryInput{descriptorPath, descriptorSHA, dir, fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: source, OperationID: op, ManifestSHA256: manifest, OriginalRunID: op}}
	path := filepath.Join(dir, "recovery-input.private.json")
	nativeJSON(t, path, input)
	var receipts [2]struct {
		PID                                      int
		Stage, WindowStartSHA256, RecoverySHA256 string
		DropReady                                bool
	}
	for i, phase := range []string{"stop", "recover"} {
		child := exec.CommandContext(ctx, programPath, "-test.run=^TestLifecyclePublicCLINativeCrossProcessServiceRecovery$", "-test.count=1", "-test.timeout=2m")
		child.Env = append(nativeChildEnv(map[string]string{"QS_LIFECYCLE_PUBLIC_CLI_NATIVE": "1", "QS_LIFECYCLE_PUBLIC_CLI_NATIVE_REQUIRED": "1"}), "QS_PUBLIC_SERVICE_RECOVERY_INPUT="+path, "QS_PUBLIC_SERVICE_RECOVERY_PHASE="+phase)
		var out, stderr bytes.Buffer
		child.Stdout, child.Stderr = &out, &stderr
		if err, reaped := publicCLIRunProcessGroup(child); err != nil || !reaped {
			t.Logf("public_cli_service_recovery_stage=%s child_started=%t child_reaped=%t child_stdout_sha256=%s child_stderr_sha256=%s", phase, child.Process != nil, reaped, sha(out.Bytes()), sha(stderr.Bytes()))
			t.Fatal("public_cli_service_recovery_original_child_failed")
		}
		var raw struct {
			PID      int    `json:"pid"`
			Stage    string `json:"stage"`
			Start    string `json:"window_start_sha256"`
			Recovery string `json:"recovery_sha256"`
			Drop     bool   `json:"drop_ready"`
		}
		if publicCLIPrivateJSON(filepath.Join(dir, phase+".receipt.private.json"), &raw) != nil || raw.PID <= 0 || raw.PID != child.Process.Pid || raw.Stage != phase || raw.Drop || !hashPattern.MatchString(raw.Start) {
			t.Fatal("public_cli_service_recovery_native_child_receipt_rejected")
		}
		receipts[i].PID, receipts[i].Stage, receipts[i].WindowStartSHA256, receipts[i].RecoverySHA256, receipts[i].DropReady = raw.PID, raw.Stage, raw.Start, raw.Recovery, raw.Drop
		observed, observedPID, exit, err := inspect()
		if err != nil || observed.ID != original.ID || observed.Image != original.Image || (i == 0 && (observed.Running || observedPID != 0 || exit != 0 || observed.StartedAt != original.StartedAt)) || (i == 1 && (!observed.Running || observedPID <= 0 || observed.StartedAt == original.StartedAt)) {
			t.Fatal("public_cli_service_recovery_actual_runtime_state_mismatch")
		}
	}
	if receipts[0].PID == receipts[1].PID || receipts[0].WindowStartSHA256 != receipts[1].WindowStartSHA256 || receipts[0].RecoverySHA256 != "" || !hashPattern.MatchString(receipts[1].RecoverySHA256) {
		t.Fatal("public_cli_service_recovery_original_process_budget_binding_failed")
	}
	settled = true
	t.Log("public_cli_service_recovery_cross_process=true original_native_stop_journal=true actual_same_cid_restored=true original_window_retained=true recovery_only=true production_writer_fence=false production_drop=false")
}
