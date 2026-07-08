package cachesignal

import "time"

const (
	SignalNameQuestionnaireCacheChanged = "questionnaire_cache_changed"
	SignalNameScaleCacheChanged         = "scale_cache_changed"
	SignalNameTypologyModelCacheChanged = "typology_model_cache_changed"
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

// TypologyModelCacheChangedSignal 类型学模型缓存失效唤醒信号（best-effort，非业务事实）。
type TypologyModelCacheChangedSignal struct {
	Code       string    `json:"code"`
	Action     string    `json:"action,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

func (s TypologyModelCacheChangedSignal) SignalName() string {
	return SignalNameTypologyModelCacheChanged
}

func (s TypologyModelCacheChangedSignal) SignalKey() string {
	return s.Code
}
