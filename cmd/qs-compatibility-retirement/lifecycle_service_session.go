package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	"github.com/FangcunMount/qs-server/pkg/version"
)

// These inputs name an independently approved service inventory; they never
// import a stop/fence/recovery result. This entrypoint is run only through the
// existing pinned SSH/root management channel, not an invented probe identity.
type lifecycleServiceSessionRequest struct {
	FormatVersion     int    `json:"format_version"`
	Kind              string `json:"kind"`
	ToolSourceSHA     string `json:"tool_source_sha"`
	ToolBinarySHA256  string `json:"tool_binary_sha256"`
	OriginalSourceSHA string `json:"original_source_sha"`
	OperationID       string `json:"operation_id"`
	ManifestSHA256    string `json:"manifest_sha256"`
	OriginalRunID     string `json:"original_run_id"`
	ActualRunID       string `json:"actual_run_id"`
	DescriptorSHA256  string `json:"descriptor_sha256"`
}

func lifecycleServicesRoot(operation, role string) string {
	service := "qs-apiserver"
	if role == "server-d" {
		service = "qs-worker"
	}
	return filepath.Join("/opt/qs-server", service, "compatibility-retirement", operation)
}

func loadLifecycleServiceRole(ctx context.Context, path, hash, operation, run, role string) (*stop.Approval, string, error) {
	if ctx == nil || ctx.Err() != nil || (role != "server-a" && role != "server-d") || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 ||
		!shaRE.MatchString(sourceSHA) || version.GitCommit != sourceSHA || !runRE.MatchString(operation) || !runRE.MatchString(run) ||
		(path != filepath.Join(lifecycleServicesRoot(operation, role), "service-session.json") && (role != "server-d" || path != filepath.Join(lifecycleServiceInvocationRoot(operation, run), "service-session.json"))) {
		return nil, "", lifecycleError("lifecycle_service_session_binding_rejected")
	}
	var r lifecycleServiceSessionRequest
	if readLifecyclePrivate(path, hash, &r) != nil || r.FormatVersion != 1 || r.Kind != "qs_root_service_session" ||
		r.ToolSourceSHA != sourceSHA || !shaRE.MatchString(r.OriginalSourceSHA) || r.OperationID != operation || r.ActualRunID != run ||
		!runRE.MatchString(r.OriginalRunID) || !hashRE.MatchString(r.ManifestSHA256) || !hashRE.MatchString(r.DescriptorSHA256) || !hashRE.MatchString(r.ToolBinarySHA256) {
		return nil, "", lifecycleError("lifecycle_service_session_binding_rejected")
	}
	root := lifecycleServicesRoot(operation, role)
	program, err := os.Executable()
	if err != nil || program != filepath.Join(root, "qs-compatibility-retirement") {
		return nil, "", lifecycleError("lifecycle_service_session_binding_rejected")
	}
	if _, err = readLifecycleRootFile(program, r.ToolBinarySHA256, true); err != nil {
		return nil, "", err
	}
	a, err := stop.ReadApprovedDescriptor(filepath.Join(root, "approved-services.json"), r.DescriptorSHA256)
	if err != nil {
		return nil, "", lifecycleError("lifecycle_service_approval_rejected")
	}
	b, err := a.WindowBinding(ctx)
	if err != nil || b.SourceSHA != r.OriginalSourceSHA || b.OperationID != operation || b.OriginalRunID != r.OriginalRunID || b.ManifestSHA256 != r.ManifestSHA256 {
		return nil, "", lifecycleError("lifecycle_service_approval_rejected")
	}
	return a, filepath.Join(root, "service-journal"), nil
}

func runLifecycleServiceSession(ctx context.Context, mode, path, hash, operation, run string, in, out *os.File) error {
	originalHash := hash
	template := mode == "host-services-d-template" || mode == "host-services-d-recovery-template"
	recovery := mode == "host-services-d-recovery" || mode == "host-services-d-recovery-template"
	if mode != "host-services-d" && mode != "host-services-d-recovery" && !template {
		return lifecycleError("lifecycle_service_session_binding_rejected")
	}
	if template {
		var err error
		path, hash, err = stageLifecycleServiceTemplate(ctx, path, hash, operation, run, recovery)
		if err != nil {
			return err
		}
	}
	a, journal, err := loadLifecycleServiceRole(ctx, path, hash, operation, run, "server-d")
	if err != nil {
		return err
	}
	// Session stdout is the bounded native challenge/reply wire. Do not append a
	// JSON completion receipt or raw diagnostic after the stream has terminated.
	if recovery {
		return stop.ServeRootRemoteHostSession(ctx, a, journal, in, out, true)
	}
	templateHash := ""
	if template {
		templateHash = originalHash
	}
	materials, err := stop.OpenRootRemoteMaterials(ctx, a, path, hash, run, templateHash)
	if err != nil {
		return err
	}
	return stop.ServeRootRemoteOwnedSession(ctx, a, materials, journal, in, out)
}
