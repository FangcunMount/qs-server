package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRestoreMaterialOwnerPolicyKeepsTestedRestoreSeparate(t *testing.T) {
	tested := &lifecyclePreparationOwner{engines: []*lifecycleOwnedEngine{{Kind: "mysql"}, {Kind: "mongodb"}}}
	for _, tc := range []struct {
		name, policy string
		owner        *lifecyclePreparationOwner
		want         bool
	}{
		{"backup-empty", lifecycleBackupIntegrityPolicy, &lifecyclePreparationOwner{}, true},
		{"backup-missing-owner", lifecycleBackupIntegrityPolicy, nil, false},
		{"backup-one-engine", lifecycleBackupIntegrityPolicy, &lifecyclePreparationOwner{engines: []*lifecycleOwnedEngine{{}}}, false},
		{"backup-two-engines", lifecycleBackupIntegrityPolicy, tested, false},
		{"backup-unknown-record", lifecycleBackupIntegrityPolicy, &lifecyclePreparationOwner{materialRecords: map[string]string{"foreign": "unknown"}}, false},
		{"default-tested", "", tested, true},
		{"explicit-tested", "tested_restore_v1", tested, true},
		{"default-empty", "", &lifecyclePreparationOwner{}, false},
		{"tested-empty", "tested_restore_v1", &lifecyclePreparationOwner{}, false},
		{"unknown-policy", "backup-integrity-only", &lifecyclePreparationOwner{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if lifecycleRestoreOwnerMatchesPolicy(lifecycleRequest{PreparationPolicy: tc.policy}, tc.owner) != tc.want {
				t.Fatal("restore material policy did not preserve the exact owner boundary")
			}
		})
	}
}

func TestBackupIntegrityDoesNotRegisterOrDeleteUnknownRestoreMetadata(t *testing.T) {
	d := materialTestDirectory(t, filepath.Join(t.TempDir(), "current"))
	r := lifecycleRequest{PreparationPolicy: lifecycleBackupIntegrityPolicy, OperationID: "123-1", ActualRunID: "125-1", prepareRoot: d.path}
	owner := &lifecyclePreparationOwner{}
	if err := registerLifecycleCurrentRestoreMetadata(t.Context(), d, owner, r); err != nil || len(d.files) != 0 {
		t.Fatal("backup integrity required or registered invented restore metadata", err)
	}
	if err := d.checkComplete(false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(d.path, "lifecycle-restore-125-1.registration.private.json")
	body := []byte("unregistered restore material")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := registerLifecycleCurrentRestoreMetadata(t.Context(), d, owner, r); err != nil || len(d.files) != 0 {
		t.Fatal("unknown restore metadata was adopted", err)
	}
	if d.checkComplete(false) == nil || d.purge(t.Context()) == nil {
		t.Fatal("backup integrity bypassed the unchanged file closed set")
	}
	remaining, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(remaining, body) {
		t.Fatal("failed closed-set validation removed unknown material", err)
	}
}

func TestBackupIntegrityRestoreSetRequiresEmptyExactCurrentRun(t *testing.T) {
	binding := materialTestBinding()
	want := [][]string{
		{"ps", "--all", "--no-trunc", "--filter", "label=qs.retirement.operation=" + binding.operation, "--filter", "label=qs.retirement.run=" + binding.actualRun, "--format", "{{.ID}}"},
		{"volume", "ls", "--filter", "label=qs.retirement.operation=" + binding.operation, "--filter", "label=qs.retirement.run=" + binding.actualRun, "--format", "{{.Name}}"},
	}
	for _, tc := range []struct {
		name      string
		responses []string
		readError bool
		wantCalls int
		wantOK    bool
	}{
		{"empty", []string{"", ""}, false, 2, true},
		{"unknown-container", []string{"foreign-container\n"}, false, 1, false},
		{"unknown-volume", []string{"", "foreign-volume\n"}, false, 2, false},
		{"duplicate-container", []string{"foreign\nforeign\n"}, false, 1, false},
		{"read-failed", nil, true, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			read := func(_ context.Context, docker string, args ...string) ([]byte, error) {
				if docker != "fixed-docker" || calls >= len(want) || !reflect.DeepEqual(args, want[calls]) {
					t.Fatal("restore absence query changed scope or performed a mutation", args)
				}
				calls++
				if tc.readError {
					return nil, errors.New("unavailable")
				}
				return []byte(tc.responses[calls-1]), nil
			}
			err := checkLifecycleBackupIntegrityRestoreSet(t.Context(), binding, "fixed-docker", read)
			if (err == nil) != tc.wantOK || calls != tc.wantCalls {
				t.Fatal("unknown restore resources were not rejected", err, calls)
			}
		})
	}
}

func TestBackupIntegrityRestoreSetRejectsMissingBindingAndOwnedResources(t *testing.T) {
	read := func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unbound restore inventory reached Docker")
		return nil, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		ctx     context.Context
		binding lifecycleMaterialBinding
	}{
		{t.Context(), lifecycleMaterialBinding{}},
		{ctx, materialTestBinding()},
	} {
		if checkLifecycleBackupIntegrityRestoreSet(tc.ctx, tc.binding, "fixed-docker", read) == nil {
			t.Fatal("unknown restore inventory binding accepted")
		}
	}
	c := &lifecycleBatchMaterials{backupIntegrityOnly: true, engines: []*lifecycleEnginePurge{{}}}
	if c.checkRestoreMaterials(t.Context(), false) == nil || c.checkRestoreMaterials(t.Context(), true) == nil {
		t.Fatal("backup-only catalog acquired a restore purge resource")
	}
}
