package cachetarget

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

type contextKey string

const suppressHotsetRecordingKey contextKey = "suppress-hotset-recording"

// WarmupKind 标识可治理的预热目标类型。
type WarmupKind string

const (
	WarmupKindStaticScale             WarmupKind = "static.scale"
	WarmupKindStaticQuestionnaire     WarmupKind = "static.questionnaire"
	WarmupKindStaticScaleList         WarmupKind = "static.scale_list"
	WarmupKindQueryStatsSystem        WarmupKind = "query.stats_system"
	WarmupKindQueryStatsQuestionnaire WarmupKind = "query.stats_questionnaire"
	WarmupKindQueryStatsPlan          WarmupKind = "query.stats_plan"
)

const scaleListWarmupScope = "published"

// WarmupTarget 描述一个稳定的预热目标。
type WarmupTarget struct {
	Family redisplane.Family
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
	Top(context.Context, redisplane.Family, WarmupKind, int64) ([]WarmupTarget, error)
}

// HotsetInspector 读取带分数的热点排行，供治理状态接口使用。
type HotsetInspector interface {
	TopWithScores(context.Context, redisplane.Family, WarmupKind, int64) ([]HotsetItem, error)
}

func (t WarmupTarget) Key() string {
	return fmt.Sprintf("%s|%s|%s", t.Family, t.Kind, t.Scope)
}

// OrgID returns the owning organization for query warmup targets.
func (t WarmupTarget) OrgID() (int64, bool) {
	switch t.Kind {
	case WarmupKindQueryStatsSystem:
		return ParseQueryStatsSystemScope(t.Scope)
	case WarmupKindQueryStatsQuestionnaire:
		orgID, _, ok := ParseQueryStatsQuestionnaireScope(t.Scope)
		return orgID, ok
	case WarmupKindQueryStatsPlan:
		orgID, _, ok := ParseQueryStatsPlanScope(t.Scope)
		return orgID, ok
	default:
		return 0, false
	}
}

// FamilyForKind returns the Redis family used by a governance warmup kind.
func FamilyForKind(kind WarmupKind) redisplane.Family {
	switch kind {
	case WarmupKindStaticScale, WarmupKindStaticQuestionnaire, WarmupKindStaticScaleList:
		return redisplane.FamilyStatic
	case WarmupKindQueryStatsSystem, WarmupKindQueryStatsQuestionnaire, WarmupKindQueryStatsPlan:
		return redisplane.FamilyQuery
	default:
		return redisplane.FamilyDefault
	}
}

func normalizeCodeScope(prefix, code string) string {
	return prefix + ":" + strings.ToLower(strings.TrimSpace(code))
}

// NewStaticScaleWarmupTarget 创建量表静态缓存预热目标。
func NewStaticScaleWarmupTarget(code string) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   WarmupKindStaticScale,
		Scope:  normalizeCodeScope("scale", code),
	}
}

// NewStaticQuestionnaireWarmupTarget 创建问卷静态缓存预热目标。
func NewStaticQuestionnaireWarmupTarget(code string) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   WarmupKindStaticQuestionnaire,
		Scope:  normalizeCodeScope("questionnaire", code),
	}
}

// NewStaticScaleListWarmupTarget 创建量表列表预热目标。
func NewStaticScaleListWarmupTarget() WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyStatic,
		Kind:   WarmupKindStaticScaleList,
		Scope:  scaleListWarmupScope,
	}
}

// NewQueryStatsSystemWarmupTarget 创建系统统计查询预热目标。
func NewQueryStatsSystemWarmupTarget(orgID int64) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyQuery,
		Kind:   WarmupKindQueryStatsSystem,
		Scope:  fmt.Sprintf("org:%d", orgID),
	}
}

// NewQueryStatsQuestionnaireWarmupTarget 创建问卷统计查询预热目标。
func NewQueryStatsQuestionnaireWarmupTarget(orgID int64, code string) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyQuery,
		Kind:   WarmupKindQueryStatsQuestionnaire,
		Scope:  fmt.Sprintf("org:%d:questionnaire:%s", orgID, strings.ToLower(strings.TrimSpace(code))),
	}
}

// NewQueryStatsPlanWarmupTarget 创建计划统计查询预热目标。
func NewQueryStatsPlanWarmupTarget(orgID int64, planID uint64) WarmupTarget {
	return WarmupTarget{
		Family: redisplane.FamilyQuery,
		Kind:   WarmupKindQueryStatsPlan,
		Scope:  fmt.Sprintf("org:%d:plan:%d", orgID, planID),
	}
}

func ParseWarmupKind(raw string) (WarmupKind, bool) {
	switch WarmupKind(strings.TrimSpace(raw)) {
	case WarmupKindStaticScale,
		WarmupKindStaticQuestionnaire,
		WarmupKindStaticScaleList,
		WarmupKindQueryStatsSystem,
		WarmupKindQueryStatsQuestionnaire,
		WarmupKindQueryStatsPlan:
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
	case WarmupKindStaticScaleList:
		expected := NewStaticScaleListWarmupTarget()
		if scope != expected.Scope {
			return WarmupTarget{}, fmt.Errorf("invalid static scale list warmup scope: %s", scope)
		}
		return expected, nil
	case WarmupKindQueryStatsSystem:
		orgID, ok := ParseQueryStatsSystemScope(scope)
		if !ok {
			return WarmupTarget{}, fmt.Errorf("invalid stats system warmup scope: %s", scope)
		}
		return NewQueryStatsSystemWarmupTarget(orgID), nil
	case WarmupKindQueryStatsQuestionnaire:
		orgID, code, ok := ParseQueryStatsQuestionnaireScope(scope)
		if !ok {
			return WarmupTarget{}, fmt.Errorf("invalid stats questionnaire warmup scope: %s", scope)
		}
		return NewQueryStatsQuestionnaireWarmupTarget(orgID, code), nil
	case WarmupKindQueryStatsPlan:
		orgID, planID, ok := ParseQueryStatsPlanScope(scope)
		if !ok {
			return WarmupTarget{}, fmt.Errorf("invalid stats plan warmup scope: %s", scope)
		}
		return NewQueryStatsPlanWarmupTarget(orgID, planID), nil
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

func ParseQueryStatsSystemScope(scope string) (int64, bool) {
	var orgID int64
	if _, err := fmt.Sscanf(scope, "org:%d", &orgID); err != nil || orgID == 0 {
		return 0, false
	}
	return orgID, true
}

func ParseQueryStatsQuestionnaireScope(scope string) (int64, string, bool) {
	var orgID int64
	var code string
	if _, err := fmt.Sscanf(scope, "org:%d:questionnaire:%s", &orgID, &code); err != nil || orgID == 0 || code == "" {
		return 0, "", false
	}
	return orgID, code, true
}

func ParseQueryStatsPlanScope(scope string) (int64, uint64, bool) {
	parts := strings.Split(scope, ":")
	if len(parts) != 4 || parts[0] != "org" || parts[2] != "plan" {
		return 0, 0, false
	}
	orgID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || orgID == 0 {
		return 0, 0, false
	}
	planID, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil || planID == 0 {
		return 0, 0, false
	}
	return orgID, planID, true
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
