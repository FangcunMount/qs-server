package cachetarget

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
)

// ManualWarmupRequest is the consumer-facing command contract.
type ManualWarmupRequest struct {
	Targets []ManualWarmupTarget `json:"targets"`
}

type ManualWarmupTarget struct {
	Kind  WarmupKind `json:"kind"`
	Scope string     `json:"scope"`
}

type RepairCompleteRequest struct {
	RepairKind         string
	OrgIDs             []int64
	QuestionnaireCodes []string
	PlanIDs            []uint64
}

type HotsetSnapshot struct {
	Family    cachemodel.Family `json:"family"`
	Kind      WarmupKind        `json:"kind"`
	Limit     int64             `json:"limit"`
	Available bool              `json:"available"`
	Degraded  bool              `json:"degraded"`
	Message   string            `json:"message,omitempty"`
	Items     []HotsetItem      `json:"items"`
}

func ParseManualWarmupTarget(target ManualWarmupTarget) (WarmupTarget, error) {
	kindRaw := strings.TrimSpace(string(target.Kind))
	kind, ok := ParseWarmupKind(kindRaw)
	if !ok {
		return WarmupTarget{}, fmt.Errorf("invalid warmup kind: %s", kindRaw)
	}
	return ParseWarmupTarget(kind, target.Scope)
}

func WarmupTargetOrgID(target WarmupTarget) (int64, bool) {
	return target.OrgID()
}
