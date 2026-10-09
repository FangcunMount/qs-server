package main

import (
	"context"

	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
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
