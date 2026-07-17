package cachetarget

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
)

var ErrWarmupSkipped = errors.New("cache warmup skipped")

// PublishedModelWarmer is the narrow governance port exported by the
// modelcatalog composition root. WarmupKind keeps this contract business-model
// independent while preserving the public scale/typology target names.
type PublishedModelWarmer interface {
	WarmByCode(context.Context, WarmupKind, string) error
}

type contextKey string

const suppressHotsetRecordingKey contextKey = "suppress-hotset-recording"

// WarmupKind 标识可治理的预热目标类型。
type WarmupKind string

const (
	WarmupKindStaticScale         WarmupKind = "static.scale"
	WarmupKindStaticQuestionnaire WarmupKind = "static.questionnaire"
	WarmupKindStaticTypologyModel WarmupKind = "static.typology_model"
	WarmupKindQueryStatsOverview  WarmupKind = "query.stats_overview"
)

// WarmupTarget 描述一个稳定的预热目标。
type WarmupTarget struct {
	Family cachemodel.Family
	Kind   WarmupKind
	Scope  string
}

type HotsetItem struct {
	Target WarmupTarget `json:"target"`
	Score  float64      `json:"score"`
}

// HotsetRecorder 记录和读取可治理缓存目标的热点排行。
type HotsetRecorder interface {
	Record(context.Context, WarmupTarget) error
	Top(context.Context, cachemodel.Family, WarmupKind, int64) ([]WarmupTarget, error)
}

// HotsetInspector 读取带分数的热点排行，供治理状态接口使用。
type HotsetInspector interface {
	TopWithScores(context.Context, cachemodel.Family, WarmupKind, int64) ([]HotsetItem, error)
}

func (t WarmupTarget) Key() string {
	return fmt.Sprintf("%s|%s|%s", t.Family, t.Kind, t.Scope)
}

// OrgID returns the owning organization for query warmup targets.
func (t WarmupTarget) OrgID() (int64, bool) {
	switch t.Kind {
	case WarmupKindQueryStatsOverview:
		orgID, _, ok := ParseQueryStatsOverviewScope(t.Scope)
		return orgID, ok
	default:
		return 0, false
	}
}

// FamilyForKind returns the Redis family used by a governance warmup kind.
func FamilyForKind(kind WarmupKind) cachemodel.Family {
	switch kind {
	case WarmupKindStaticScale, WarmupKindStaticQuestionnaire, WarmupKindStaticTypologyModel:
		return cachemodel.FamilyStatic
	case WarmupKindQueryStatsOverview:
		return cachemodel.FamilyQuery
	default:
		return cachemodel.FamilyDefault
	}
}

func normalizeCodeScope(prefix, code string) string {
	return prefix + ":" + strings.ToLower(strings.TrimSpace(code))
}

// NewStaticScaleWarmupTarget 创建量表静态缓存预热目标。
func NewStaticScaleWarmupTarget(code string) WarmupTarget {
	return WarmupTarget{
		Family: cachemodel.FamilyStatic,
		Kind:   WarmupKindStaticScale,
		Scope:  normalizeCodeScope("scale", code),
	}
}

// NewStaticQuestionnaireWarmupTarget 创建问卷静态缓存预热目标。
func NewStaticQuestionnaireWarmupTarget(code string) WarmupTarget {
	return WarmupTarget{
		Family: cachemodel.FamilyStatic,
		Kind:   WarmupKindStaticQuestionnaire,
		Scope:  normalizeCodeScope("questionnaire", code),
	}
}

// NewStaticTypologyModelWarmupTarget 创建类型学模型静态缓存预热目标。
func NewStaticTypologyModelWarmupTarget(code string) WarmupTarget {
	return WarmupTarget{
		Family: cachemodel.FamilyStatic,
		Kind:   WarmupKindStaticTypologyModel,
		Scope:  normalizeCodeScope("typology_model", code),
	}
}

// NewQueryStatsOverviewWarmupTarget 创建 operating 统计概览查询预热目标。
func NewQueryStatsOverviewWarmupTarget(orgID int64, preset string) WarmupTarget {
	return WarmupTarget{
		Family: cachemodel.FamilyQuery,
		Kind:   WarmupKindQueryStatsOverview,
		Scope:  fmt.Sprintf("org:%d:preset:%s", orgID, normalizeOverviewPreset(preset)),
	}
}

func ParseWarmupKind(raw string) (WarmupKind, bool) {
	switch WarmupKind(strings.TrimSpace(raw)) {
	case WarmupKindStaticScale,
		WarmupKindStaticQuestionnaire,
		WarmupKindStaticTypologyModel,
		WarmupKindQueryStatsOverview:
		return WarmupKind(strings.TrimSpace(raw)), true
	default:
		return "", false
	}
}

// ParseWarmupTarget parses a validated governance kind and scope into a canonical warmup target.
func ParseWarmupTarget(kind WarmupKind, scope string) (WarmupTarget, error) {
	scope = strings.TrimSpace(scope)
	switch kind {
	case WarmupKindStaticScale:
		code, ok := ParseStaticScaleScope(scope)
		if !ok {
			return WarmupTarget{}, fmt.Errorf("invalid static scale warmup scope: %s", scope)
		}
		return NewStaticScaleWarmupTarget(code), nil
	case WarmupKindStaticQuestionnaire:
		code, ok := ParseStaticQuestionnaireScope(scope)
		if !ok {
			return WarmupTarget{}, fmt.Errorf("invalid static questionnaire warmup scope: %s", scope)
		}
		return NewStaticQuestionnaireWarmupTarget(code), nil
	case WarmupKindStaticTypologyModel:
		code, ok := ParseStaticTypologyModelScope(scope)
		if !ok {
			return WarmupTarget{}, fmt.Errorf("invalid static typology model warmup scope: %s", scope)
		}
		return NewStaticTypologyModelWarmupTarget(code), nil
	case WarmupKindQueryStatsOverview:
		orgID, preset, ok := ParseQueryStatsOverviewScope(scope)
		if !ok {
			return WarmupTarget{}, fmt.Errorf("invalid stats overview warmup scope: %s", scope)
		}
		return NewQueryStatsOverviewWarmupTarget(orgID, preset), nil
	default:
		return WarmupTarget{}, fmt.Errorf("unsupported warmup kind: %s", kind)
	}
}

func ParseStaticScaleScope(scope string) (string, bool) {
	if !strings.HasPrefix(scope, "scale:") {
		return "", false
	}
	code := strings.TrimPrefix(scope, "scale:")
	return code, code != ""
}

func ParseStaticQuestionnaireScope(scope string) (string, bool) {
	if !strings.HasPrefix(scope, "questionnaire:") {
		return "", false
	}
	code := strings.TrimPrefix(scope, "questionnaire:")
	return code, code != ""
}

func ParseStaticTypologyModelScope(scope string) (string, bool) {
	if !strings.HasPrefix(scope, "typology_model:") {
		return "", false
	}
	code := strings.TrimPrefix(scope, "typology_model:")
	return code, code != ""
}

func ParseQueryStatsOverviewScope(scope string) (int64, string, bool) {
	parts := strings.Split(scope, ":")
	if len(parts) != 4 || parts[0] != "org" || parts[2] != "preset" {
		return 0, "", false
	}
	orgID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || orgID <= 0 {
		return 0, "", false
	}
	preset := normalizeOverviewPreset(parts[3])
	if !isSupportedOverviewPreset(preset) {
		return 0, "", false
	}
	return orgID, preset, true
}

func normalizeOverviewPreset(preset string) string {
	return strings.ToLower(strings.TrimSpace(preset))
}

func isSupportedOverviewPreset(preset string) bool {
	switch preset {
	case "today", "7d", "30d":
		return true
	default:
		return false
	}
}

// SuppressHotsetRecording returns a context that prevents best-effort hotset writes.
func SuppressHotsetRecording(ctx context.Context) context.Context {
	return context.WithValue(ctx, suppressHotsetRecordingKey, true)
}

func HotsetRecordingSuppressed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	suppressed, _ := ctx.Value(suppressHotsetRecordingKey).(bool)
	return suppressed
}
