//go:build integration

package compatibilityretirementbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

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
	publicCLIInventoryPageSize = 1000
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
		{"partial_page", 999, 1, false},
		{"one_full_page_plus_empty_eof", 1000, 2, false},
		{"multi_page", 1001, 2, false},
		{"two_full_pages_plus_empty_eof", 2000, 3, false},
		{"maximum_records_and_eof", 1000000, 1001, false},
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
			report := publicCLIInventoryMaterialTestReport(1000, 2, false)
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
	report := publicCLIInventoryMaterialTestReport(1000, 2, false)
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

func publicCLIPackage(t *testing.T, path, cli string) string {
	t.Helper()
	raw, e := os.ReadFile(cli)
	if e != nil {
		t.Fatal("public_cli_native_binary_missing")
	}
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	if tw.WriteHeader(&tar.Header{Name: "inventory-linux-" + runtime.GOARCH, Mode: 0700, Size: int64(len(raw)), Typeflag: tar.TypeReg}) != nil {
		t.Fatal("public_cli_native_package_failed")
	}
	if _, e = tw.Write(raw); e != nil || tw.Close() != nil || gz.Close() != nil || writePrivate(path, b.Bytes()) != nil {
		t.Fatal("public_cli_native_package_failed")
	}
	return sha(b.Bytes())
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

func TestLifecyclePublicCLINativePrepareRootOnce(t *testing.T) {
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
			if e != nil || !reflect.DeepEqual(before, after) {
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
	manifestHash := ""
	approval := Approval{}
	opMaterial := map[string]bool{"inventory-cli": true, "boundary-request.json": true, "inventory-request.json": true, "manifest.json": true, "lifecycle-request.json": true, "operation.lock": true, "archive": true, "bounds-" + boundRun: true, "inventory-" + invRun: true}
	rootMaterial := map[string]bool{"restore-native": true, "tool.intent.private.json": true, "source-copy.intent.private.json": true, "manifest.json": true, "lifecycle-request.json": true, "lifecycle-restore-" + actualRun + ".registration.private.json": true, "inventory-" + invRun: true}
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
		if publicCLIMaterialsKnown(root, rootMaterial) != nil || publicCLIMaterialsKnown(opDir, opMaterial) != nil {
			retainFixture()
			t.Error("public_cli_native_unregistered_material_retained")
			return
		}
		removePackage := os.Remove(pkg)
		if (removePackage != nil && !os.IsNotExist(removePackage)) || os.RemoveAll(root) != nil || os.RemoveAll(opDir) != nil {
			retainFixture()
			t.Error("public_cli_native_owned_material_cleanup_failed")
			return
		}
		for _, p := range []string{pkg, root, opDir} {
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
	recovery := TargetRecoveryRequest{SourceSHA: source, OperationID: op, OriginalRunID: invRun, ActualRunID: actualRun, ManifestSHA256: manifestHash, MongoNonTargetSHA256: report.Bindings["mongodb"].NonTargetHash, SQLHead: 99, MongoHead: 38}
	requestHash := nativeJSON(t, filepath.Join(opDir, "lifecycle-request.json"), map[string]any{"format_version": 1, "kind": "compatibility_retirement_lifecycle_request", "tool_source_sha": source, "original_source_sha": source, "operation_id": op, "actual_run_id": actualRun, "manifest_sha256": manifestHash, "archive_directory": archive, "source_directory": sourceDir, "window_directory": filepath.Join(opDir, "window"), "journal_directory": filepath.Join(opDir, "journal"), "archive_approval": approval, "recovery": recovery, "restore_engines": map[string]string{"mysql_image_id": mysqlRoot.Image, "mongodb_image_id": mongoRoot.Image, "architecture": runtime.GOARCH}, "source_file_sha256": hashes})
	packageHash := publicCLIPackage(t, pkg, cli)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", filepath.Join(repo, "scripts/database/compatibility-retirement.py"), "--operation", "prepare", "--prepare-mode", "lifecycle", "--operation-id", op, "--approved-source-sha", source, "--actual-source-sha", source, "--run-id", actualRun, "--manifest-hash", manifestHash, "--lifecycle-request-hash", requestHash, "--inventory-binary", cli, "--root", base)
	child := map[string]string{}
	for k, v := range env {
		child[k] = v
	}
	child["RETIREMENT_PACKAGE_SHA256"] = packageHash
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
	result := publicCLIDecode(t, repo, stdout.Bytes())
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
	caps, ok := result["capabilities"].(map[string]any)
	if !ok || len(caps) == 0 {
		t.Fatal("public_cli_native_capabilities_missing")
	}
	for _, v := range caps {
		if v != false {
			t.Fatal("public_cli_native_capability_promoted")
		}
	}
	elapsed, ok := result["restore_elapsed_millis"].(float64)
	if !ok || elapsed <= 0 || elapsed > 600000 {
		t.Fatal("public_cli_native_combined_restore_budget_failed")
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
		filepath.Join(opDir, "manifest.json"):          manifestHash,
		filepath.Join(root, "manifest.json"):           manifestHash,
		filepath.Join(opDir, "lifecycle-request.json"): requestHash,
	} {
		actual, readErr := os.ReadFile(path)
		if readErr != nil || sha(actual) != expected {
			t.Fatal("public_cli_native_immutable_descriptor_changed")
		}
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
	t.Logf("public_cli_native_prepare_complete=true actual_root_once=true actual_inventory_eof=true actual_stdio_restore=true target_count=4 combined_restore_millis=%d public_call_millis=%d original_catalog_and_content_equal=true engines_remaining=0 volumes_remaining=0 archive_assets_remaining=0 production_drop=false production_purge=false", int64(elapsed), time.Since(started).Milliseconds())
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
