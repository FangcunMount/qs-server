package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparationPolicyRequiresAnExplicitKnownSelection(t *testing.T) {
	for _, policy := range []string{"", lifecycleTestedRestorePolicy, lifecycleBackupIntegrityPolicy} {
		if !lifecyclePreparationPolicyValid(policy) {
			t.Fatalf("known policy rejected: %q", policy)
		}
	}
	for _, value := range []string{`null`, `true`, `"skip_all_checks"`} {
		raw := []byte(`{"history_policy":"` + lifecycleDiscardHistoryPolicy + `","preparation_policy":` + value + `}`)
		if _, err := decodeLifecycleStagingRequest(raw); err == nil {
			t.Fatalf("ambiguous or unknown policy accepted: %s", value)
		}
	}
	for _, policy := range []string{"", lifecycleTestedRestorePolicy, lifecycleBackupIntegrityPolicy} {
		raw := []byte(`{"history_policy":"` + lifecycleDiscardHistoryPolicy + `","preparation_policy":"` + policy + `"}`)
		if r, err := decodeLifecycleStagingRequest(raw); err != nil || r.PreparationPolicy != policy {
			t.Fatalf("explicit selection lost: %s: %v", policy, err)
		}
	}
}

func backupIntegrityOriginalStagingFixture(t *testing.T) (string, string, lifecycleRequest, map[string]string) {
	t.Helper()
	root, companion, r, hashes := originalRootStagingMaterialFixture(t, true)
	var zero lifecyclePreparationRestoreZero
	raw, err := os.ReadFile(r.PreparationRestoreZero.Path)
	if err != nil || json.Unmarshal(raw, &zero) != nil {
		t.Fatal("offline fixture restore metadata")
	}
	for _, file := range zero.Files {
		if err := os.Remove(filepath.Join(root, file.Name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(r.PreparationRestoreZero.Path); err != nil {
		t.Fatal(err)
	}
	r.PreparationRestoreZero = nil
	r.PreparationPolicy = lifecycleBackupIntegrityPolicy
	var original lifecycleRequest
	raw, err = os.ReadFile(filepath.Join(root, "lifecycle-request.json"))
	if err != nil || json.Unmarshal(raw, &original) != nil {
		t.Fatal("offline original request")
	}
	original.PreparationPolicy = lifecycleBackupIntegrityPolicy
	rewrite := func(name string, value any) []byte {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil || os.WriteFile(filepath.Join(root, name), body, 0600) != nil {
			t.Fatal("offline fixture rewrite")
		}
		return body
	}
	raw = rewrite("lifecycle-request.json", original)
	if err := os.WriteFile(filepath.Join(companion, "lifecycle-request.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var intent lifecycleSourceCopyIntent
	body, err := os.ReadFile(r.SourceCopyIntent.Path)
	if err != nil || json.Unmarshal(body, &intent) != nil {
		t.Fatal("offline source intent")
	}
	intent.PreparationPolicy = lifecycleBackupIntegrityPolicy
	intent.RequestSHA256 = digestRaw(raw)
	r.SourceCopyIntent.SHA256 = digestRaw(rewrite("source-copy.intent.private.json", intent))
	var native lifecycleAPIInvocationIntent
	body, err = os.ReadFile(filepath.Join(root, "tool.intent.private.json"))
	if err != nil || json.Unmarshal(body, &native) != nil {
		t.Fatal("offline native invocation")
	}
	native.DerivedSHA256 = intent.RequestSHA256
	body = rewrite("tool.intent.private.json", native)
	if err := os.WriteFile(filepath.Join(companion, "native-call.intent.private.json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	return root, companion, r, hashes
}

func TestBackupIntegrityStagingKeepsSourceBindingsWithoutRestoreClaims(t *testing.T) {
	for _, mutation := range []string{"none", "fake_zero", "policy_mismatch", "unregistered_restore_metadata", "changed_copy"} {
		t.Run(mutation, func(t *testing.T) {
			root, companion, r, hashes := backupIntegrityOriginalStagingFixture(t)
			switch mutation {
			case "fake_zero":
				r.PreparationRestoreZero = &lifecycleFinalFileBinding{Path: filepath.Join(root, "fake-zero.json"), SHA256: r.ManifestSHA256}
			case "policy_mismatch":
				r.PreparationPolicy = ""
			case "unregistered_restore_metadata":
				if err := os.WriteFile(filepath.Join(root, "restore-unknown.intent.private.json"), []byte("unknown"), 0600); err != nil {
					t.Fatal(err)
				}
			case "changed_copy":
				if err := os.WriteFile(filepath.Join(root, "inventory-"+r.Approval.RunID, lifecycleSourceNames[3]), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			d, peer, err := openLifecycleRootStagingMaterialFiles(context.Background(), root, companion, r, hashes, uint32(os.Getuid()), uint32(os.Getuid()))
			if mutation != "none" {
				if err == nil {
					_ = d.close()
					_ = peer.close()
					t.Fatal("changed policy, unregistered material or source accepted")
				}
				return
			}
			if err != nil || d == nil || peer == nil {
				t.Fatalf("explicit backup-only source handoff rejected: %v", err)
			}
			t.Cleanup(func() {
				if err := errors.Join(d.close(), peer.close()); err != nil {
					t.Error(err)
				}
			})
			if err := d.checkComplete(false); err != nil {
				t.Fatal(err)
			}
			if err := peer.checkComplete(false); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBackupIntegrityReceiptDoesNotAssertRestoreSuccess(t *testing.T) {
	receipt := lifecycleReceipt{PreparationPolicy: lifecycleBackupIntegrityPolicy, ArchiveBindingComplete: true, BackupIntegrityComplete: true, Complete: true}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || string(fields["isolated_content_restore_complete"]) != "false" || string(fields["restore_elapsed_millis"]) != "0" {
		t.Fatal("backup-only receipt asserted a restore")
	}
	if _, exists := fields["preparation_restore_zero_sha256"]; exists {
		t.Fatal("backup-only receipt emitted a restore-zero claim")
	}
}
