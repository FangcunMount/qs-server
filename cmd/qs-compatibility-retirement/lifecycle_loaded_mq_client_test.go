package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
	reader "github.com/FangcunMount/qs-server/internal/pkg/runtimefactsreader"
)

func TestLifecycleLoadedMQClientIsReadOnlyDirectAndBounded(t *testing.T) {
	c, transport := lifecycleLoadedMQHTTPClient()
	defer transport.CloseIdleConnections()
	if c.Transport != transport || transport.Proxy != nil || transport.DialContext == nil || !transport.DisableKeepAlives || c.Timeout != 5*time.Second || transport.TLSHandshakeTimeout != 5*time.Second || transport.ResponseHeaderTimeout != 5*time.Second || c.Jar != nil || c.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("metadata reader gained indirect transport or unbounded authentication")
	}
}

func TestLifecycleCurrentMQSummaryPreservesScopeAndCannotIssueAcceptance(t *testing.T) {
	d := stop.LoadedMQDiagnostic{ObservationSHA256: strings.Repeat("a", 64), Workers: 1, Nodes: 2, Clients: 3, PublisherHistoryUnproven: true, SharedHandoffUnproven: true, ExternalAIUnproven: true}
	observed, e := summarizeLifecycleCurrentMQConnections(d, reader.ErrScopeUnproven)
	if e != nil || observed == nil || observed.scope != "original_worker_registered_consumers_and_visible_publishers" || observed.diagnostic != d || observed.diagnostic.BrokerScopeComplete {
		t.Fatal("valid current connections erased broader gaps or claimed complete broker scope", e)
	}
	h := &lifecycleFixedHost{currentMQ: observed}
	if h.VerifyAcceptance(t.Context(), lifecycleRequest{}, new(backup.Archive)) == nil || h.currentMQ != nil || h.acceptedMaterials != nil {
		t.Fatal("imported connection summary minted acceptance or survived a failed fresh check")
	}
	if lifecycleEffectsPreflight(t.Context()) == nil || h.CheckWholeWriterFence(t.Context(), lifecycleRequest{}) == nil {
		t.Fatal("connection summary bypassed actual adapters or whole writer fence")
	}
}

func TestLifecycleCurrentMQSummaryRejectsIncompleteOrFailedObservation(t *testing.T) {
	good := stop.LoadedMQDiagnostic{ObservationSHA256: strings.Repeat("a", 64), Workers: 1, Nodes: 2, Clients: 3, ExternalAIUnproven: true}
	for name, mutate := range map[string]func(*stop.LoadedMQDiagnostic){
		"missing-hash":     func(d *stop.LoadedMQDiagnostic) { d.ObservationSHA256 = "" },
		"no-worker":        func(d *stop.LoadedMQDiagnostic) { d.Workers = 0 },
		"too-many-workers": func(d *stop.LoadedMQDiagnostic) { d.Workers = 33 },
		"no-node":          func(d *stop.LoadedMQDiagnostic) { d.Nodes = 0 },
		"too-many-nodes":   func(d *stop.LoadedMQDiagnostic) { d.Nodes = 2049 },
		"no-connection":    func(d *stop.LoadedMQDiagnostic) { d.Clients = 0 },
		"too-many-clients": func(d *stop.LoadedMQDiagnostic) { d.Clients = 65537 },
		"full-broker":      func(d *stop.LoadedMQDiagnostic) { d.BrokerScopeComplete = true },
		"external-cleared": func(d *stop.LoadedMQDiagnostic) { d.ExternalAIUnproven = false },
	} {
		t.Run(name, func(t *testing.T) {
			d := good
			mutate(&d)
			if summary, e := summarizeLifecycleCurrentMQConnections(d, reader.ErrScopeUnproven); e == nil || summary != nil {
				t.Fatal("invalid native diagnostic became a current-connection observation")
			}
		})
	}
	for _, e := range []error{nil, stop.ErrLoadedMQ, context.Canceled, errors.Join(reader.ErrScopeUnproven, stop.ErrLoadedMQ)} {
		if summary, result := summarizeLifecycleCurrentMQConnections(good, e); result == nil || summary != nil {
			t.Fatal("failed/unknown native result was hidden by a scope sentinel", e)
		}
	}
}

func TestLifecycleCurrentMQCallerReadsNativeThenFencesBeforeSaving(t *testing.T) {
	calls := preBComparisonProductionCalls(t, "lifecycle_native_acceptance.go", "verifyNativeAcceptance")
	read, lastFence := -1, -1
	for i, name := range calls {
		if name == "ObserveLoadedMQ" {
			read = i
		}
		if name == "CheckWholeWriterFence" {
			lastFence = i
		}
	}
	if read < 0 || lastFence <= read {
		t.Fatal("native current-MQ caller omitted the real read or its post-read whole fence")
	}
}
