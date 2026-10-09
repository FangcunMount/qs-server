package main

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

func TestWriterScopeCannotImportAuthorityOrEnterPreparation(t *testing.T) {
	for _, name := range []string{"drop_ready", "whole_writer_fence_proven", "platform_observation", "SourceSHA", "workflow_scope_sha256_extra"} {
		raw := []byte(`{"writer_control":{"workflow_scope_sha256":"` + strings.Repeat("a", 64) + `","` + name + `":true}}`)
		if lifecycleExactJSONNames(raw, reflect.TypeOf(lifecycleRequest{})) == nil {
			t.Fatal("imported field accepted")
		}
	}
	for _, value := range []string{"null", `{"workflow_scope_sha256":"` + strings.Repeat("a", 64) + `"}`} {
		if _, e := decodeLifecycleStagingRequest([]byte(`{"writer_control":` + value + `}`)); e == nil {
			t.Fatal("prepare accepted writer field")
		}
	}
	for _, v := range []*lifecycleWriterControl{nil, {}, {WorkflowScopeSHA256: "main"}} {
		if v.valid() {
			t.Fatal("unbound scope accepted")
		}
	}
	v := &lifecycleWriterControl{WorkflowScopeSHA256: strings.Repeat("a", 64)}
	raw, e := json.Marshal(v)
	if e != nil || string(raw) != `{"workflow_scope_sha256":"`+strings.Repeat("a", 64)+`"}` {
		t.Fatal("unexpected expected-input schema")
	}
}

func TestWriterTokenStaysInLiveOwnerAndIsCleared(t *testing.T) {
	for _, raw := range []string{"", "invalid\ncredential", strings.Repeat("a", 8193)} {
		t.Setenv("GITHUB_READ_TOKEN", raw)
		if b, e := lifecyclePlatformToken(); e == nil || b != nil || os.Getenv("GITHUB_READ_TOKEN") != "" {
			t.Fatal("invalid token retained")
		}
	}
	t.Setenv("GITHUB_READ_TOKEN", "offline-private-input")
	b, e := lifecyclePlatformToken()
	if e != nil || os.Getenv("GITHUB_READ_TOKEN") != "" {
		t.Fatal("token environment retained")
	}
	owner := &lifecycleWriterObservation{token: b, platform: new(fence.PlatformObservation)}
	owner.close()
	owner.close()
	if owner.token != nil || owner.platform != nil {
		t.Fatal("owner retains credential/observation")
	}
	for _, c := range b {
		if c != 0 {
			t.Fatal("borrowed credential bytes not cleared")
		}
	}
}

func TestWriterScopeRequiresOriginalLiveNativeManagement(t *testing.T) {
	r := lifecycleRequest{WriterControl: &lifecycleWriterControl{WorkflowScopeSHA256: strings.Repeat("a", 64)}}
	for _, h := range []*lifecycleFixedHost{nil, {}, {services: &lifecycleServiceController{window: new(fence.MaintenanceWindow), managementReady: true}}} {
		if e := h.CheckWholeWriterFence(context.Background(), r); e == nil {
			t.Fatal("expected identities became isolation")
		}
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("partial platform leaf activated DDL")
	}
}
