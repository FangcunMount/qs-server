package cache

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
)

type WarmupKind = cachetarget.WarmupKind

const (
	WarmupKindStaticScale             = cachetarget.WarmupKindStaticScale
	WarmupKindStaticQuestionnaire     = cachetarget.WarmupKindStaticQuestionnaire
	WarmupKindStaticScaleList         = cachetarget.WarmupKindStaticScaleList
	WarmupKindQueryStatsSystem        = cachetarget.WarmupKindQueryStatsSystem
	WarmupKindQueryStatsQuestionnaire = cachetarget.WarmupKindQueryStatsQuestionnaire
	WarmupKindQueryStatsPlan          = cachetarget.WarmupKindQueryStatsPlan
)

type WarmupTarget = cachetarget.WarmupTarget
type HotsetItem = cachetarget.HotsetItem

func NewStaticScaleWarmupTarget(code string) WarmupTarget {
	return cachetarget.NewStaticScaleWarmupTarget(code)
}

func NewStaticQuestionnaireWarmupTarget(code string) WarmupTarget {
	return cachetarget.NewStaticQuestionnaireWarmupTarget(code)
}

func NewStaticScaleListWarmupTarget() WarmupTarget {
	return cachetarget.NewStaticScaleListWarmupTarget()
}

func NewQueryStatsSystemWarmupTarget(orgID int64) WarmupTarget {
	return cachetarget.NewQueryStatsSystemWarmupTarget(orgID)
}

func NewQueryStatsQuestionnaireWarmupTarget(orgID int64, code string) WarmupTarget {
	return cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, code)
}

func NewQueryStatsPlanWarmupTarget(orgID int64, planID uint64) WarmupTarget {
	return cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID)
}

func ParseWarmupKind(raw string) (WarmupKind, bool) {
	return cachetarget.ParseWarmupKind(raw)
}

func ParseStaticScaleScope(scope string) (string, bool) {
	return cachetarget.ParseStaticScaleScope(scope)
}

func ParseStaticQuestionnaireScope(scope string) (string, bool) {
	return cachetarget.ParseStaticQuestionnaireScope(scope)
}

func ParseQueryStatsSystemScope(scope string) (int64, bool) {
	return cachetarget.ParseQueryStatsSystemScope(scope)
}

func ParseQueryStatsQuestionnaireScope(scope string) (int64, string, bool) {
	return cachetarget.ParseQueryStatsQuestionnaireScope(scope)
}

func ParseQueryStatsPlanScope(scope string) (int64, uint64, bool) {
	return cachetarget.ParseQueryStatsPlanScope(scope)
}

func SuppressHotsetRecording(ctx context.Context) context.Context {
	return cachetarget.SuppressHotsetRecording(ctx)
}

func hotsetRecordingSuppressed(ctx context.Context) bool {
	return cachetarget.HotsetRecordingSuppressed(ctx)
}
