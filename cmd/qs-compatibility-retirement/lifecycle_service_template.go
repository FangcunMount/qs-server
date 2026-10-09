package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/FangcunMount/qs-server/pkg/version"
)

// This binds metadata to the run assigned by the existing authenticated SSH
// caller. It creates no lease, Window, issuer, permission or stop result.
func deriveLifecycleServiceTemplate(raw []byte, expected, operation, run string) ([]byte, lifecycleServiceSessionRequest, error) {
	var r lifecycleServiceSessionRequest
	if !hashRE.MatchString(expected) || digestRaw(raw) != expected || !runRE.MatchString(operation) || !runRE.MatchString(run) ||
		!shaRE.MatchString(sourceSHA) || rejectDuplicateJSON(raw) != nil || lifecycleExactJSONNames(raw, reflect.TypeOf(r)) != nil {
		return nil, r, lifecycleError("lifecycle_service_template_rejected")
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || len(values) != reflect.TypeOf(r).NumField() {
		return nil, r, lifecycleError("lifecycle_service_template_rejected")
	}
	var current any
	if json.Unmarshal(values["actual_run_id"], &current) != nil || current != "" || json.Unmarshal(raw, &r) != nil ||
		r.FormatVersion != 1 || r.Kind != "qs_root_service_session" || r.ToolSourceSHA != sourceSHA || !hashRE.MatchString(r.ToolBinarySHA256) ||
		!shaRE.MatchString(r.OriginalSourceSHA) || r.OperationID != operation || !hashRE.MatchString(r.ManifestSHA256) || !runRE.MatchString(r.OriginalRunID) || !hashRE.MatchString(r.DescriptorSHA256) {
		return nil, r, lifecycleError("lifecycle_service_template_rejected")
	}
	r.ActualRunID = run
	derived, err := json.Marshal(r)
	if err != nil {
		return nil, r, lifecycleError("lifecycle_service_template_rejected")
	}
	return append(derived, '\n'), r, nil
}

func lifecycleServiceInvocationRoot(operation, run string) string {
	return filepath.Join(lifecycleServicesRoot(operation, "server-d"), "service-invocations", run)
}

type lifecycleServiceTemplateIntent struct {
	Kind              string `json:"kind"`
	TemplateSHA256    string `json:"template_sha256"`
	DerivedSHA256     string `json:"derived_sha256"`
	ToolSourceSHA     string `json:"tool_source_sha"`
	OriginalSourceSHA string `json:"original_source_sha"`
	OperationID       string `json:"operation_id"`
	OriginalRunID     string `json:"original_run_id"`
	ActualRunID       string `json:"actual_run_id"`
}

func lifecycleProtectedServiceDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return lifecycleError("lifecycle_service_template_registration_failed")
	}
	for p := path; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		st, ok := infoStat(info)
		if e != nil || !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || st.Uid != 0 ||
			info.Mode().Perm()&022 != 0 || p == path && info.Mode().Perm() != 0700 {
			return lifecycleError("lifecycle_service_template_registration_failed")
		}
		if p == "/" {
			break
		}
	}
	return nil
}

func stageLifecycleServiceTemplate(ctx context.Context, path, expected, operation, run string, recovery bool) (string, string, error) {
	if ctx == nil || ctx.Err() != nil || runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || version.GitCommit != sourceSHA ||
		!runRE.MatchString(operation) || !runRE.MatchString(run) || path != filepath.Join(lifecycleServicesRoot(operation, "server-d"), "service-session-template.json") {
		return "", "", lifecycleError("lifecycle_service_template_rejected")
	}
	raw, err := readLifecycleRootFile(path, expected, false)
	if err != nil {
		return "", "", err
	}
	derived, r, err := deriveLifecycleServiceTemplate(raw, expected, operation, run)
	if err != nil {
		return "", "", err
	}
	intent := lifecycleServiceTemplateIntent{Kind: "qs_native_assigned_service_run", TemplateSHA256: expected, DerivedSHA256: digestRaw(derived),
		ToolSourceSHA: r.ToolSourceSHA, OriginalSourceSHA: r.OriginalSourceSHA, OperationID: operation, OriginalRunID: r.OriginalRunID, ActualRunID: run}
	intentRaw, err := json.Marshal(intent)
	if err != nil {
		return "", "", lifecycleError("lifecycle_service_template_rejected")
	}
	intentRaw = append(intentRaw, '\n')
	base := filepath.Dir(lifecycleServiceInvocationRoot(operation, run))
	if err = os.Mkdir(base, 0700); err != nil {
		if !os.IsExist(err) {
			return "", "", lifecycleError("lifecycle_service_template_registration_failed")
		}
	} else if err = syncLifecycleDirectory(filepath.Dir(base)); err != nil {
		return "", "", err
	}
	if err = lifecycleProtectedServiceDirectory(base); err != nil {
		return "", "", err
	}
	root := lifecycleServiceInvocationRoot(operation, run)
	request := filepath.Join(root, "service-session.json")
	intentPath := filepath.Join(root, "derived-service-session.intent.private.json")
	if err = os.Mkdir(root, 0700); err != nil {
		// Only recovery may reuse the exact original native registration. This
		// never retries Stop or claims D was stopped: its real journal/issuer
		// must still pass OpenRecovery and the fresh live budget protocol.
		if !recovery || !os.IsExist(err) || lifecycleProtectedServiceDirectory(root) != nil {
			return "", "", lifecycleError("lifecycle_service_template_existing_or_unknown")
		}
		if _, err = readLifecycleRootFile(intentPath, digestRaw(intentRaw), false); err != nil {
			return "", "", err
		}
		if _, err = readLifecycleRootFile(request, digestRaw(derived), false); err != nil {
			return "", "", err
		}
		return request, digestRaw(derived), nil
	}
	if err = lifecycleProtectedServiceDirectory(root); err != nil {
		return "", "", err
	}
	if err = syncLifecycleDirectory(base); err != nil {
		return "", "", err
	}
	if err = writeLifecycleRaw(intentPath, intentRaw, 0600); err != nil {
		return "", "", err
	}
	if err = writeLifecycleRaw(request, derived, 0600); err != nil {
		return "", "", err
	}
	return request, digestRaw(derived), nil
}
