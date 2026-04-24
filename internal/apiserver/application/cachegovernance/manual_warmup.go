package cachegovernance

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
)

const manualWarmupTrigger = "manual"

// ManualWarmupRequest 描述一次手工触发的缓存预热请求。
type ManualWarmupRequest struct {
	Targets []ManualWarmupTarget `json:"targets"`
}

// ManualWarmupTarget 描述一个由治理接口提交的预热目标。
type ManualWarmupTarget struct {
	Kind  cachetarget.WarmupKind `json:"kind"`
	Scope string                 `json:"scope"`
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
	Kind    cachetarget.WarmupKind `json:"kind"`
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
func ParseManualWarmupTarget(target ManualWarmupTarget) (cachetarget.WarmupTarget, error) {
	kindRaw := strings.TrimSpace(string(target.Kind))
	kind, ok := cachetarget.ParseWarmupKind(kindRaw)
	if !ok {
		return cachetarget.WarmupTarget{}, fmt.Errorf("invalid warmup kind: %s", kindRaw)
	}

	scope := strings.TrimSpace(target.Scope)
	switch kind {
	case cachetarget.WarmupKindStaticScale:
		code, ok := cachetarget.ParseStaticScaleScope(scope)
		if !ok {
			return cachetarget.WarmupTarget{}, fmt.Errorf("invalid static scale warmup scope: %s", scope)
		}
		return cachetarget.NewStaticScaleWarmupTarget(code), nil
	case cachetarget.WarmupKindStaticQuestionnaire:
		code, ok := cachetarget.ParseStaticQuestionnaireScope(scope)
		if !ok {
			return cachetarget.WarmupTarget{}, fmt.Errorf("invalid static questionnaire warmup scope: %s", scope)
		}
		return cachetarget.NewStaticQuestionnaireWarmupTarget(code), nil
	case cachetarget.WarmupKindStaticScaleList:
		expected := cachetarget.NewStaticScaleListWarmupTarget()
		if scope != expected.Scope {
			return cachetarget.WarmupTarget{}, fmt.Errorf("invalid static scale list warmup scope: %s", scope)
		}
		return expected, nil
	case cachetarget.WarmupKindQueryStatsSystem:
		orgID, ok := cachetarget.ParseQueryStatsSystemScope(scope)
		if !ok {
			return cachetarget.WarmupTarget{}, fmt.Errorf("invalid stats system warmup scope: %s", scope)
		}
		return cachetarget.NewQueryStatsSystemWarmupTarget(orgID), nil
	case cachetarget.WarmupKindQueryStatsQuestionnaire:
		orgID, code, ok := cachetarget.ParseQueryStatsQuestionnaireScope(scope)
		if !ok {
			return cachetarget.WarmupTarget{}, fmt.Errorf("invalid stats questionnaire warmup scope: %s", scope)
		}
		return cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, code), nil
	case cachetarget.WarmupKindQueryStatsPlan:
		orgID, planID, ok := cachetarget.ParseQueryStatsPlanScope(scope)
		if !ok {
			return cachetarget.WarmupTarget{}, fmt.Errorf("invalid stats plan warmup scope: %s", scope)
		}
		return cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID), nil
	default:
		return cachetarget.WarmupTarget{}, fmt.Errorf("unsupported warmup kind: %s", kind)
	}
}

// WarmupTargetOrgID 返回查询类预热目标所属机构。
func WarmupTargetOrgID(target cachetarget.WarmupTarget) (int64, bool) {
	switch target.Kind {
	case cachetarget.WarmupKindQueryStatsSystem:
		return cachetarget.ParseQueryStatsSystemScope(target.Scope)
	case cachetarget.WarmupKindQueryStatsQuestionnaire:
		orgID, _, ok := cachetarget.ParseQueryStatsQuestionnaireScope(target.Scope)
		return orgID, ok
	case cachetarget.WarmupKindQueryStatsPlan:
		orgID, _, ok := cachetarget.ParseQueryStatsPlanScope(target.Scope)
		return orgID, ok
	default:
		return 0, false
	}
}
