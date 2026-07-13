package systemgovernance

import (
	"context"
	"testing"

	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
)

type fakeCachePolicyReloader struct {
	request cachemodel.CachePolicyReloadRequest
}

func (f *fakeCachePolicyReloader) ReloadPolicy(_ context.Context, _ int64, request cachemodel.CachePolicyReloadRequest) (*cachemodel.CachePolicyReloadResult, error) {
	f.request = request
	return &cachemodel.CachePolicyReloadResult{PreviousVersion: 1, CurrentVersion: 2, Changed: true, Source: "config.yaml", ChangedCapabilities: []string{"statistics.query"}}, nil
}

func TestActionExecutorRunsCachePolicyReload(t *testing.T) {
	reloader := &fakeCachePolicyReloader{}
	executor := NewActionExecutor(NewActionRegistry(), &fakeStatisticsGovernance{}, reloader)
	result, err := executor.Run(context.Background(), 9, "cache.reload_policy", ActionRunRequest{
		Confirm: true, Input: map[string]interface{}{"expected_version": 1},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if reloader.request.ExpectedVersion != 1 || result.Result["current_version"] != float64(2) {
		t.Fatalf("reload request=%#v result=%#v", reloader.request, result.Result)
	}
}
