package retirement

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAIExternalExecModesFixedOperationDirectoryAcrossAttempts(t *testing.T) {
	base, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	directory := filepath.Join(base, "backups", "qs-server", "compatibility-retirement", "900-1")
	if e = os.MkdirAll(directory, 0700); e != nil {
		t.Fatal(e)
	}
	verify, e := aiExternalExecModePath(directory, "900-1", aiExternalVerifyMode)
	if e != nil {
		t.Fatal(e)
	}
	bounds, e := aiExternalExecModePath(directory, "900-1", aiExternalBoundsMode)
	if e != nil || verify == bounds {
		t.Fatal("bounds and verify journals mixed")
	}
	repeated, e := aiExternalExecModePath(directory, "900-1", aiExternalVerifyMode)
	if e != nil || repeated != verify {
		t.Fatal("same operation changed journal location")
	}
	for _, input := range []struct {
		directory, operation string
		mode                 aiExternalExecMode
	}{
		{directory, "900-2", aiExternalVerifyMode}, {filepath.Join(directory, "history-901"), "900-1", aiExternalVerifyMode}, {directory, "900-1", "arbitrary"}, {filepath.Join(base, "900-1"), "900-1", aiExternalBoundsMode},
	} {
		if _, e = aiExternalExecModePath(input.directory, input.operation, input.mode); e == nil {
			t.Fatal("attempt directory/foreign operation/mode accepted")
		}
	}
}
func TestAIExternalExecModesBindActualProtocolPacketAndInheritedDeadline(t *testing.T) {
	host := aiExecUnitHost(t)
	owner := HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "900-1"}
	runtime := strings.Repeat("b", 40)
	image := "sha256:" + strings.Repeat("c", 64)
	cid := strings.Repeat("d", 64)
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	packet := map[string]string{"protocol": "qs-ai-readonly-host-input/v2", "source_sha": owner.SourceSHA, "operation_id": owner.OperationID, "run_id": "901", "runtime_source_sha": runtime, "image_id": image, "container_id": cid}
	raw, e := json.Marshal(packet)
	if e != nil {
		t.Fatal(e)
	}
	binding, e := aiExternalExecModeBinding(ctx, aiExternalVerifyMode, owner, "901", runtime, image, cid, host, raw)
	if e != nil || binding.DeadlineUnixNano != deadline.UnixNano() || binding.InputSHA256 != sourceSHA(raw) {
		t.Fatal("original actual packet/deadline not bound")
	}
	if _, e = aiExternalExecModeBinding(ctx, aiExternalBoundsMode, owner, "901", runtime, image, cid, host, raw); e == nil {
		t.Fatal("verify packet executed as bounds")
	}
	for _, field := range []string{"source_sha", "operation_id", "run_id", "runtime_source_sha", "image_id", "container_id", "protocol"} {
		changed := map[string]string{}
		for k, v := range packet {
			changed[k] = v
		}
		changed[field] = "changed"
		altered, e := json.Marshal(changed)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = aiExternalExecModeBinding(ctx, aiExternalVerifyMode, owner, "901", runtime, image, cid, host, altered); e == nil {
			t.Fatal("packet original identity changed")
		}
	}
	packet["protocol"] = "qs-ai-readonly-bounds-discovery-input/v1"
	raw, e = json.Marshal(packet)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = aiExternalExecModeBinding(ctx, aiExternalBoundsMode, owner, "901", runtime, image, cid, host, raw); e != nil {
		t.Fatal("actual discovery mode rejected")
	}
	if _, e = aiExternalExecModeBinding(context.Background(), aiExternalBoundsMode, owner, "901", runtime, image, cid, host, raw); e == nil {
		t.Fatal("unbounded/new deadline accepted")
	}
}
func TestAIExternalExecModeMissingHostInputsCannotStartOrReconcile(t *testing.T) {
	if _, e := aiExternalExecuteMode(context.Background(), nil, "", aiExternalVerifyMode, HistoricalCoordinatorBinding{}, "", "", "", "", nil, nil); e == nil {
		t.Fatal("missing actual host/source inputs admitted")
	}
	result, e := ObserveAIExternalExecLifecycle(context.Background(), AIExternalExecReconcileInput{})
	if e == nil || result.TerminalObserved || result.PermissionToRetry || result.PermissionToReleaseExclusion || result.CASAuthority || result.DropReady {
		t.Fatal("missing durable original handle created capability")
	}
	var imported AIExternalExecLifecycleSummary
	if json.Unmarshal([]byte(`{"terminal_observed":true,"permission_to_retry":true,"cas_authority":true}`), &imported) != nil {
		t.Fatal("diagnostic DTO setup failed")
	}
	// This DTO has no Apply/Start or conversion to an actual qualification. It
	// cannot be consumed by either producer, which requires its original packet.
	if _, e = aiExternalExecModeBinding(context.Background(), "invalid", HistoricalCoordinatorBinding{}, "", "", "", "", nil, nil); e == nil {
		t.Fatal("DTO/mode substituted for actual producer context")
	}
}
