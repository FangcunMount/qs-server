package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"github.com/FangcunMount/qs-server/pkg/version"
)

// Public key publication is a native preparation result. It neither creates a
// window nor installs D trust/SSH access, and conveys no stop/fence/DROP permit.
type lifecycleServiceKeyReceipt struct {
	Kind                   string `json:"kind"`
	ToolSourceSHA          string `json:"tool_source_sha"`
	OperationID            string `json:"operation_id"`
	ActualRunID            string `json:"actual_run_id"`
	PublicKey              string `json:"public_key,omitempty"`
	KeyAvailable           bool   `json:"key_available"`
	WholeWriterFenceProven bool   `json:"whole_writer_fence_proven"`
	DropReady              bool   `json:"drop_ready"`
	ErrorCategory          string `json:"error_category"`
}

func runLifecycleServiceKey(ctx context.Context, mode, path, hash, operation, run string) (receipt lifecycleServiceKeyReceipt, result error) {
	receipt = lifecycleServiceKeyReceipt{Kind: "qs_native_temporary_budget_key_result", ToolSourceSHA: sourceSHA, OperationID: operation, ActualRunID: run}
	defer func() { receipt.ErrorCategory = lifecycleCategory(result) }()
	if mode != "host-budget-key-create" && mode != "host-budget-key-open" {
		return receipt, lifecycleError("lifecycle_service_session_binding_rejected")
	}
	a, _, err := loadLifecycleServiceRole(ctx, path, hash, operation, run, "server-a")
	if mode == "host-budget-key-create" && path == filepath.Join(lifecycleServicesRoot(operation, "server-a"), "budget-issuer", "service-session.json") {
		a, err = loadLifecyclePreparationBudgetRole(ctx, path, hash, operation, run)
	}
	if err != nil {
		return receipt, err
	}
	var key *stop.RootBudgetKey
	if mode == "host-budget-key-create" {
		key, err = stop.CreateRootBudgetKey(ctx, a)
	} else {
		key, err = stop.OpenRootBudgetKey(ctx, a)
	}
	if err != nil {
		return receipt, lifecycleError("lifecycle_actual_budget_issuer_missing")
	}
	receipt.PublicKey, err = key.PublicKey()
	if err != nil {
		return receipt, lifecycleError("lifecycle_actual_budget_issuer_missing")
	}
	receipt.KeyAvailable = true
	return receipt, nil
}

// The existing root preparation binary can create only this batch's original
// seed using an independently approved real A inventory. No remote descriptor,
// trust, Window or service-stop result exists yet. Apply opens the same seed.
func loadLifecyclePreparationBudgetRole(ctx context.Context, path, hash, operation, run string) (*stop.Approval, error) {
	if ctx == nil || ctx.Err() != nil || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 ||
		!shaRE.MatchString(sourceSHA) || version.GitCommit != sourceSHA || !runRE.MatchString(operation) || !runRE.MatchString(run) ||
		path != filepath.Join(lifecycleServicesRoot(operation, "server-a"), "budget-issuer", "service-session.json") {
		return nil, lifecycleError("lifecycle_service_session_binding_rejected")
	}
	var r lifecycleServiceSessionRequest
	if readLifecyclePrivate(path, hash, &r) != nil || r.FormatVersion != 1 || r.Kind != "qs_root_service_session" ||
		r.ToolSourceSHA != sourceSHA || !shaRE.MatchString(r.OriginalSourceSHA) || r.OperationID != operation || r.ActualRunID != run ||
		!runRE.MatchString(r.OriginalRunID) || r.ActualRunID == r.OriginalRunID || !hashRE.MatchString(r.ManifestSHA256) || !hashRE.MatchString(r.DescriptorSHA256) || !hashRE.MatchString(r.ToolBinarySHA256) {
		return nil, lifecycleError("lifecycle_service_session_binding_rejected")
	}
	program, err := os.Executable()
	if err != nil || program != filepath.Join(lifecycleRootBatch(operation, run), "restore-native") {
		return nil, lifecycleError("lifecycle_service_session_binding_rejected")
	}
	if _, err = readLifecycleRootFile(program, r.ToolBinarySHA256, true); err != nil {
		return nil, err
	}
	a, err := stop.ReadApprovedDescriptor(filepath.Join(filepath.Dir(path), "approved-services.json"), r.DescriptorSHA256)
	if err != nil {
		return nil, lifecycleError("lifecycle_service_approval_rejected")
	}
	binding, err := a.WindowBinding(ctx)
	if err != nil || binding.SourceSHA != r.OriginalSourceSHA || binding.OperationID != operation || binding.OriginalRunID != r.OriginalRunID || binding.ManifestSHA256 != r.ManifestSHA256 {
		return nil, lifecycleError("lifecycle_service_approval_rejected")
	}
	return a, nil
}
