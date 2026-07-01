package cachesignal

import "time"

const (
	SignalNameQuestionnaireCacheChanged    = "questionnaire_cache_changed"
	SignalNameScaleCacheChanged            = "scale_cache_changed"
	SignalNamePersonalityModelCacheChanged = "personality_model_cache_changed"
)

// QuestionnaireCacheChangedSignal 问卷缓存失效唤醒信号（best-effort，非业务事实）。
type QuestionnaireCacheChangedSignal struct {
	Code       string    `json:"code"`
	Version    string    `json:"version,omitempty"`
	Action     string    `json:"action,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (s QuestionnaireCacheChangedSignal) SignalName() string {
	return SignalNameQuestionnaireCacheChanged
}

func (s QuestionnaireCacheChangedSignal) SignalKey() string {
	return s.Code
}

// ScaleCacheChangedSignal 量表缓存失效唤醒信号（best-effort，非业务事实）。
type ScaleCacheChangedSignal struct {
	Code       string    `json:"code"`
	Action     string    `json:"action,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (s ScaleCacheChangedSignal) SignalName() string {
	return SignalNameScaleCacheChanged
}

func (s ScaleCacheChangedSignal) SignalKey() string {
	return s.Code
}

// PersonalityModelCacheChangedSignal 人格模型缓存失效唤醒信号（best-effort，非业务事实）。
type PersonalityModelCacheChangedSignal struct {
	Code       string    `json:"code"`
	Action     string    `json:"action,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (s PersonalityModelCacheChangedSignal) SignalName() string {
	return SignalNamePersonalityModelCacheChanged
}

func (s PersonalityModelCacheChangedSignal) SignalKey() string {
	return s.Code
}
