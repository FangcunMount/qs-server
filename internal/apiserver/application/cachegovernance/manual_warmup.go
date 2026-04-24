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
	return cachetarget.ParseWarmupTarget(kind, target.Scope)
}

// WarmupTargetOrgID 返回查询类预热目标所属机构。
func WarmupTargetOrgID(target cachetarget.WarmupTarget) (int64, bool) {
	return target.OrgID()
}
