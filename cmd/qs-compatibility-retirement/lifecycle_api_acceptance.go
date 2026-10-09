package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"runtime"
	"time"

	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/pkg/version"
)

// This private runtime observation is owned by the original API transition.
// It does not establish current MQ/business-flow acceptance or mint a batch
// purge token. Only source/image/process and actual fixed read endpoints pass.
type lifecycleAPIRuntimeObservation struct {
	self                   *lifecycleAPIRuntimeObservation
	owner                  *lifecycleAPITransition
	cid, image, source     string
	pid                    int
	started, address       string
	health, version, ready string
}

func (*lifecycleAPIRuntimeObservation) MarshalJSON() ([]byte, error) {
	return nil, lifecycleError("lifecycle_private_runtime_serialization_forbidden")
}

func lifecycleAPIReadAddress(v lifecycleAPIInspection) (string, error) {
	var ports map[string]json.RawMessage
	if json.Unmarshal(v.Config["ExposedPorts"], &ports) != nil || ports["8080/tcp"] == nil || len(v.NetworkSettings.Networks) != 1 {
		return "", lifecycleError("lifecycle_actual_api_fixed_read_route_unproven")
	}
	for _, raw := range v.NetworkSettings.Networks {
		var actual struct {
			IPAddress string
			NetworkID string
		}
		if rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &actual) != nil || !hashRE.MatchString(actual.NetworkID) {
			return "", lifecycleError("lifecycle_actual_api_fixed_read_route_unproven")
		}
		ip := net.ParseIP(actual.IPAddress)
		if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
			return "", lifecycleError("lifecycle_actual_api_fixed_read_route_unproven")
		}
		return net.JoinHostPort(ip.String(), "8080"), nil
	}
	return "", lifecycleError("lifecycle_actual_api_fixed_read_route_unproven")
}

func lifecycleAPIReadEndpoint(ctx context.Context, client *http.Client, address, path string) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || client == nil || (path != "/healthz" && path != "/version" && path != "/readyz") {
		return nil, lifecycleError("lifecycle_actual_api_readback_failed")
	}
	q, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(q, http.MethodGet, "http://"+address+path, nil)
	if e != nil {
		return nil, lifecycleError("lifecycle_actual_api_readback_failed")
	}
	res, e := client.Do(req)
	if e != nil {
		return nil, lifecycleError("lifecycle_actual_api_readback_failed")
	}
	raw, re := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	ce := res.Body.Close()
	if re != nil || ce != nil || res.StatusCode != 200 || len(raw) > 1<<20 || rejectDuplicateJSON(raw) != nil {
		return nil, lifecycleError("lifecycle_actual_api_readback_failed")
	}
	return raw, nil
}

func lifecycleAPIValidateResponses(health, build, ready []byte, source, architecture string) error {
	var h struct {
		Code    *int              `json:"code"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	var b struct {
		Code    *int         `json:"code"`
		Message string       `json:"message"`
		Data    version.Info `json:"data"`
	}
	var r struct {
		Status    string                     `json:"status"`
		Component string                     `json:"component"`
		Redis     cachemodel.RuntimeSnapshot `json:"redis"`
	}
	for _, raw := range [][]byte{health, build, ready} {
		if rejectDuplicateJSON(raw) != nil {
			return lifecycleError("lifecycle_actual_api_runtime_unproven")
		}
	}
	if json.Unmarshal(health, &h) != nil || json.Unmarshal(build, &b) != nil || json.Unmarshal(ready, &r) != nil || h.Code == nil || *h.Code != 0 || h.Message != "success" || !reflect.DeepEqual(h.Data, map[string]string{"status": "ok"}) || b.Code == nil || *b.Code != 0 || b.Message != "success" || b.Data.GitCommit != source || b.Data.GitTreeState != "clean" || b.Data.Platform != "linux/"+architecture || r.Status != "ready" || r.Component != "apiserver" || r.Redis.Component != "apiserver" || !r.Redis.Summary.Ready || r.Redis.GeneratedAt.IsZero() || r.Redis.Summary.FamilyTotal <= 0 || len(r.Redis.Families) != r.Redis.Summary.FamilyTotal || r.Redis.Summary.AvailableCount <= 0 {
		return lifecycleError("lifecycle_actual_api_runtime_unproven")
	}
	return nil
}

func (v *lifecycleAPITransition) observeAcceptance(ctx context.Context, r lifecycleRequest) error {
	if ctx == nil || ctx.Err() != nil || v == nil || v.self != v || v.unknown || v.acceptance != nil || v.engine == nil || !v.bProgramVerified || !hashRE.MatchString(v.bCID) || v.materials == nil || v.request.OperationID != r.OperationID || v.request.ActualRunID != r.ActualRunID || v.request.ToolSourceSHA != r.ToolSourceSHA || !r.DeploymentControl.valid() {
		return lifecycleError("lifecycle_actual_api_runtime_unproven")
	}
	if e := v.materials.checkComplete(false); e != nil {
		return e
	}
	if e := v.verifyTemporaryProgramProbesAbsent(ctx); e != nil {
		return e
	}
	body, e := readLifecycleAPIRecord(filepath.Join(v.dir, "b-api-create-body.private.json"))
	if e != nil {
		return e
	}
	actual, e := lifecycleAPIInspect(ctx, v.engine, v.bCID)
	if e != nil || !lifecycleAPIExpectedRuntime(actual, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) || actual.State.Health.Status != "healthy" {
		return lifecycleError("lifecycle_actual_api_runtime_unproven")
	}
	address, e := lifecycleAPIReadAddress(actual)
	if e != nil {
		return e
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: func(q context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" || addr != address {
			return nil, lifecycleError("lifecycle_actual_api_fixed_read_route_unproven")
		}
		return (&net.Dialer{}).DialContext(q, "tcp", address)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	health, e := lifecycleAPIReadEndpoint(ctx, client, address, "/healthz")
	if e != nil {
		return e
	}
	build, e := lifecycleAPIReadEndpoint(ctx, client, address, "/version")
	if e != nil {
		return e
	}
	ready, e := lifecycleAPIReadEndpoint(ctx, client, address, "/readyz")
	if e != nil {
		return e
	}
	if e = lifecycleAPIValidateResponses(health, build, ready, r.ToolSourceSHA, runtime.GOARCH); e != nil {
		return e
	}
	var readiness struct {
		Redis cachemodel.RuntimeSnapshot `json:"redis"`
	}
	started, parseErr := time.Parse(time.RFC3339Nano, actual.State.StartedAt)
	if parseErr != nil || json.Unmarshal(ready, &readiness) != nil || readiness.Redis.GeneratedAt.Before(started) || readiness.Redis.GeneratedAt.After(time.Now().Add(5*time.Second)) {
		return lifecycleError("lifecycle_actual_api_runtime_unproven")
	}
	after, e := lifecycleAPIInspect(ctx, v.engine, v.bCID)
	afterAddress, routeErr := lifecycleAPIReadAddress(after)
	if e != nil || routeErr != nil || afterAddress != address || after.State.StartedAt != actual.State.StartedAt || after.State.PID != actual.State.PID || after.State.Health.Status != "healthy" || !lifecycleAPIExpectedRuntime(after, v.original, r.DeploymentControl.BImageID, digestRaw(body), body) {
		return lifecycleError("lifecycle_actual_api_runtime_unproven")
	}
	if e = v.materials.checkComplete(false); e != nil {
		return e
	}
	if e = v.verifyTemporaryProgramProbesAbsent(ctx); e != nil {
		return e
	}
	o := &lifecycleAPIRuntimeObservation{owner: v, cid: v.bCID, image: actual.Image, source: r.ToolSourceSHA, pid: actual.State.PID, started: actual.State.StartedAt, address: address, health: digestRaw(health), version: digestRaw(build), ready: digestRaw(ready)}
	o.self, v.acceptance = o, o
	return ctx.Err()
}

// These are exact never-started native probe IDs produced by this original
// API owner. Existing B/rollback images and the actual deployed API container
// are excluded. A foreign same-scope resource fails; names are never adopted.
func (v *lifecycleAPITransition) verifyTemporaryProgramProbesAbsent(ctx context.Context) error {
	if v == nil || v.self != v || v.unknown || v.engine == nil || !hashRE.MatchString(v.programProbeIDs[0]) || !hashRE.MatchString(v.programProbeIDs[1]) || v.programProbeIDs[0] == v.programProbeIDs[1] {
		return lifecycleError("lifecycle_actual_api_temporary_scope_unproven")
	}
	for i, id := range v.programProbeIDs {
		if _, e := v.engine.call(ctx, http.MethodGet, "/containers/"+id+"/json", nil, 404); e != nil {
			return lifecycleError("lifecycle_actual_api_temporary_scope_unproven")
		}
		kind := []string{"api-program-b", "api-program-rollback"}[i]
		filters, e := json.Marshal(map[string][]string{"label": {"codex.task=qs-compatibility-retirement", "codex.operation=" + v.request.OperationID, "codex.run=" + v.request.ActualRunID, "codex.tool_source=" + sourceSHA, "codex.kind=" + kind}})
		if e != nil {
			return lifecycleError("lifecycle_actual_api_temporary_scope_unproven")
		}
		raw, e := v.engine.call(ctx, http.MethodGet, "/containers/json?all=true&filters="+url.QueryEscape(string(filters)), nil, 200)
		var found []json.RawMessage
		if e != nil || rejectDuplicateJSON(raw) != nil || json.Unmarshal(raw, &found) != nil || found == nil || len(found) != 0 {
			return lifecycleError("lifecycle_actual_api_temporary_scope_unproven")
		}
	}
	return ctx.Err()
}
