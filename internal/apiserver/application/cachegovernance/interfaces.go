package cachegovernance

import (
	"context"

	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	cachetarget "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
)

// Facade exposes cache-governance operations to system governance and
// compatibility transports without coupling them to a business module.
type Facade interface {
	TriggerStatisticsWarmup(ctx context.Context, orgID int64, action string)
	HandleRepairComplete(ctx context.Context, protectedOrgID int64, req RepairCompleteRequest) error
	HandleManualWarmup(ctx context.Context, protectedOrgID int64, req ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error)
	GetStatus(ctx context.Context) (*cachemodel.StatusSnapshot, error)
	GetHotset(ctx context.Context, kindRaw, limitRaw string) (*HotsetResponse, error)
}

// WarmupCoordinator is the application-owned cache warmup port.
type WarmupCoordinator interface {
	WarmStartup(context.Context) error
	HandleScalePublished(context.Context, string) error
	HandleQuestionnairePublished(context.Context, string, string) error
	HandleTypologyModelPublished(context.Context, string) error
	HandleStatisticsSync(context.Context, int64) error
	HandleRepairComplete(context.Context, cachetarget.RepairCompleteRequest) error
	HandleManualWarmup(context.Context, cachetarget.ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error)
}

// StatusReader exposes cache runtime and hotset status.
type StatusReader interface {
	GetRuntime(context.Context) (*cachemodel.RuntimeSnapshot, error)
	GetStatus(context.Context) (*cachemodel.StatusSnapshot, error)
	GetHotset(context.Context, cachetarget.WarmupKind, int64) (*cachetarget.HotsetSnapshot, error)
}

type RepairCompleteRequest struct {
	RepairKind string  `json:"repair_kind"`
	OrgIDs     []int64 `json:"org_ids"`
}

type ManualWarmupRequest = cachetarget.ManualWarmupRequest

type HotsetResponse struct {
	Family    cachemodel.Family        `json:"family,omitempty"`
	Kind      cachetarget.WarmupKind   `json:"kind,omitempty"`
	Limit     int64                    `json:"limit,omitempty"`
	Available bool                     `json:"available"`
	Degraded  bool                     `json:"degraded"`
	Message   string                   `json:"message,omitempty"`
	Items     []cachetarget.HotsetItem `json:"items"`
}
