package statistics

import (
	"context"
	"testing"
	"time"

	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
)

type stubGovernanceCoordinator struct {
	lastSyncOrgID int64
	lastRepair    cachegov.RepairCompleteRequest
	lastManual    cachegov.ManualWarmupRequest
	manualResult  *cachegov.ManualWarmupResult
	manualErr     error
}

func (s *stubGovernanceCoordinator) WarmStartup(context.Context) error { return nil }

func (s *stubGovernanceCoordinator) HandleScalePublished(context.Context, string) error { return nil }

func (s *stubGovernanceCoordinator) HandleQuestionnairePublished(context.Context, string, string) error {
	return nil
}

func (s *stubGovernanceCoordinator) HandleStatisticsSync(_ context.Context, orgID int64) error {
	s.lastSyncOrgID = orgID
	return nil
}

func (s *stubGovernanceCoordinator) HandleRepairComplete(_ context.Context, req cachegov.RepairCompleteRequest) error {
	s.lastRepair = req
	return nil
}

func (s *stubGovernanceCoordinator) HandleManualWarmup(_ context.Context, req cachegov.ManualWarmupRequest) (*cachegov.ManualWarmupResult, error) {
	s.lastManual = req
	if s.manualErr != nil {
		return nil, s.manualErr
	}
	if s.manualResult != nil {
		return s.manualResult, nil
	}
	return &cachegov.ManualWarmupResult{
		Trigger:    "manual",
		StartedAt:  time.Unix(1, 0),
		FinishedAt: time.Unix(2, 0),
		Summary: cachegov.ManualWarmupSummary{
			TargetCount: 1,
			OkCount:     1,
			Result:      "ok",
		},
		Items: []cachegov.ManualWarmupItemResult{
			{
				Family: "static_meta",
				Kind:   cacheinfra.WarmupKindStaticScale,
				Scope:  "scale:S-001",
				Status: cachegov.ManualWarmupItemStatusOK,
			},
		},
	}, nil
}

func (s *stubGovernanceCoordinator) Snapshot() cachegov.WarmupStatusSnapshot {
	return cachegov.WarmupStatusSnapshot{}
}

func TestGovernanceFacadeGetStatusReturnsDefaultSnapshotWhenServiceUnavailable(t *testing.T) {
	facade := NewGovernanceFacade("apiserver", nil, nil)

	result, err := facade.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if result.Component != "apiserver" {
		t.Fatalf("component = %q, want apiserver", result.Component)
	}
	if !result.Summary.Ready {
		t.Fatal("summary.ready = false, want true")
	}
	if len(result.Families) != 0 {
		t.Fatalf("families len = %d, want 0", len(result.Families))
	}
}

func TestGovernanceFacadeHandleRepairCompleteDefaultsProtectedOrgID(t *testing.T) {
	coord := &stubGovernanceCoordinator{}
	facade := NewGovernanceFacade("apiserver", coord, nil)

	err := facade.HandleRepairComplete(context.Background(), 9, RepairCompleteRequest{
		RepairKind: "statistics",
	})
	if err != nil {
		t.Fatalf("HandleRepairComplete() error = %v", err)
	}
	if len(coord.lastRepair.OrgIDs) != 1 || coord.lastRepair.OrgIDs[0] != 9 {
		t.Fatalf("org_ids = %v, want [9]", coord.lastRepair.OrgIDs)
	}
}

func TestGovernanceFacadeHandleManualWarmupRejectsCrossOrgTarget(t *testing.T) {
	facade := NewGovernanceFacade("apiserver", &stubGovernanceCoordinator{}, nil)

	_, err := facade.HandleManualWarmup(context.Background(), 1, ManualWarmupRequest{
		Targets: []cachegov.ManualWarmupTarget{
			{Kind: cacheinfra.WarmupKindQueryStatsSystem, Scope: "org:2"},
		},
	})
	if err == nil {
		t.Fatal("HandleManualWarmup() error = nil, want invalid argument")
	}
}

func TestGovernanceFacadeGetHotsetReturnsFallbackWhenServiceUnavailable(t *testing.T) {
	facade := NewGovernanceFacade("apiserver", nil, nil)

	result, err := facade.GetHotset(context.Background(), "query.stats_system", "20")
	if err != nil {
		t.Fatalf("GetHotset() error = %v", err)
	}
	if result.Available {
		t.Fatal("available = true, want false")
	}
	if !result.Degraded {
		t.Fatal("degraded = false, want true")
	}
	if len(result.Items) != 0 {
		t.Fatalf("items len = %d, want 0", len(result.Items))
	}
}
