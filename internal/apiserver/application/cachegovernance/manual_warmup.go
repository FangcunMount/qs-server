package cachegovernance

import (
	"fmt"
	"strings"
	"time"

	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
)

const manualWarmupTrigger = "manual"

// ManualWarmupRequest 描述一次手工触发的缓存预热请求。
type ManualWarmupRequest struct {
	Targets []ManualWarmupTarget `json:"targets"`
}

// ManualWarmupTarget 描述一个由治理接口提交的预热目标。
type ManualWarmupTarget struct {
	Kind  cacheinfra.WarmupKind `json:"kind"`
	Scope string                `json:"scope"`
}

// ManualWarmupItemStatus 表示单个预热目标的执行结果。
type ManualWarmupItemStatus string

const (
	ManualWarmupItemStatusOK      ManualWarmupItemStatus = "ok"
	ManualWarmupItemStatusSkipped ManualWarmupItemStatus = "skipped"
	ManualWarmupItemStatusError   ManualWarmupItemStatus = "error"
)

// ManualWarmupSummary 汇总一次手工预热的执行结果。
type ManualWarmupSummary struct {
	TargetCount  int    `json:"target_count"`
	OkCount      int    `json:"ok_count"`
	SkippedCount int    `json:"skipped_count"`
	ErrorCount   int    `json:"error_count"`
	Result       string `json:"result"`
}

// ManualWarmupItemResult 描述单个预热目标的执行明细。
type ManualWarmupItemResult struct {
	Family  string                 `json:"family"`
	Kind    cacheinfra.WarmupKind  `json:"kind"`
	Scope   string                 `json:"scope"`
	Status  ManualWarmupItemStatus `json:"status"`
	Message string                 `json:"message,omitempty"`
}

// ManualWarmupResult 描述一次手工预热命令的完整执行结果。
type ManualWarmupResult struct {
	Trigger    string                   `json:"trigger"`
	StartedAt  time.Time                `json:"started_at"`
	FinishedAt time.Time                `json:"finished_at"`
	Summary    ManualWarmupSummary      `json:"summary"`
	Items      []ManualWarmupItemResult `json:"items"`
}

// ParseManualWarmupTarget 将治理命令中的目标解析成标准预热目标。
func ParseManualWarmupTarget(target ManualWarmupTarget) (cacheinfra.WarmupTarget, error) {
	kindRaw := strings.TrimSpace(string(target.Kind))
	kind, ok := cacheinfra.ParseWarmupKind(kindRaw)
	if !ok {
		return cacheinfra.WarmupTarget{}, fmt.Errorf("invalid warmup kind: %s", kindRaw)
	}

	scope := strings.TrimSpace(target.Scope)
	switch kind {
	case cacheinfra.WarmupKindStaticScale:
		code, ok := cacheinfra.ParseStaticScaleScope(scope)
		if !ok {
			return cacheinfra.WarmupTarget{}, fmt.Errorf("invalid static scale warmup scope: %s", scope)
		}
		return cacheinfra.NewStaticScaleWarmupTarget(code), nil
	case cacheinfra.WarmupKindStaticQuestionnaire:
		code, ok := cacheinfra.ParseStaticQuestionnaireScope(scope)
		if !ok {
			return cacheinfra.WarmupTarget{}, fmt.Errorf("invalid static questionnaire warmup scope: %s", scope)
		}
		return cacheinfra.NewStaticQuestionnaireWarmupTarget(code), nil
	case cacheinfra.WarmupKindStaticScaleList:
		expected := cacheinfra.NewStaticScaleListWarmupTarget()
		if scope != expected.Scope {
			return cacheinfra.WarmupTarget{}, fmt.Errorf("invalid static scale list warmup scope: %s", scope)
		}
		return expected, nil
	case cacheinfra.WarmupKindQueryStatsSystem:
		orgID, ok := cacheinfra.ParseQueryStatsSystemScope(scope)
		if !ok {
			return cacheinfra.WarmupTarget{}, fmt.Errorf("invalid stats system warmup scope: %s", scope)
		}
		return cacheinfra.NewQueryStatsSystemWarmupTarget(orgID), nil
	case cacheinfra.WarmupKindQueryStatsQuestionnaire:
		orgID, code, ok := cacheinfra.ParseQueryStatsQuestionnaireScope(scope)
		if !ok {
			return cacheinfra.WarmupTarget{}, fmt.Errorf("invalid stats questionnaire warmup scope: %s", scope)
		}
		return cacheinfra.NewQueryStatsQuestionnaireWarmupTarget(orgID, code), nil
	case cacheinfra.WarmupKindQueryStatsPlan:
		orgID, planID, ok := cacheinfra.ParseQueryStatsPlanScope(scope)
		if !ok {
			return cacheinfra.WarmupTarget{}, fmt.Errorf("invalid stats plan warmup scope: %s", scope)
		}
		return cacheinfra.NewQueryStatsPlanWarmupTarget(orgID, planID), nil
	default:
		return cacheinfra.WarmupTarget{}, fmt.Errorf("unsupported warmup kind: %s", kind)
	}
}

// WarmupTargetOrgID 返回查询类预热目标所属机构。
func WarmupTargetOrgID(target cacheinfra.WarmupTarget) (int64, bool) {
	switch target.Kind {
	case cacheinfra.WarmupKindQueryStatsSystem:
		return cacheinfra.ParseQueryStatsSystemScope(target.Scope)
	case cacheinfra.WarmupKindQueryStatsQuestionnaire:
		orgID, _, ok := cacheinfra.ParseQueryStatsQuestionnaireScope(target.Scope)
		return orgID, ok
	case cacheinfra.WarmupKindQueryStatsPlan:
		orgID, _, ok := cacheinfra.ParseQueryStatsPlanScope(target.Scope)
		return orgID, ok
	default:
		return 0, false
	}
}
