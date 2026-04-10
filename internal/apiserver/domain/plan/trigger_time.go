package plan

import (
	"strings"
	"time"
)

const (
	DefaultPlanTriggerTime = "19:00:00"
	planTriggerTimeLayout  = "15:04:05"
)

var acceptedPlanTriggerTimeLayouts = []string{
	planTriggerTimeLayout,
	"15:04",
}

// NormalizePlanTriggerTime 将触发时间规范为 HH:MM:SS；空值回退到默认时间。
func NormalizePlanTriggerTime(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultPlanTriggerTime, nil
	}

	for _, layout := range acceptedPlanTriggerTimeLayouts {
		parsed, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			return parsed.Format(planTriggerTimeLayout), nil
		}
	}
	return "", ErrInvalidTriggerTime
}

// ApplyPlanTriggerTime 将日期部分保留，并把时间部分替换为 plan 的触发时间。
func ApplyPlanTriggerTime(base time.Time, triggerTime string) (time.Time, error) {
	normalized, err := NormalizePlanTriggerTime(triggerTime)
	if err != nil {
		return time.Time{}, err
	}

	clock, err := time.ParseInLocation(planTriggerTimeLayout, normalized, time.Local)
	if err != nil {
		return time.Time{}, err
	}

	loc := base.Location()
	if loc == nil {
		loc = time.Local
	}

	return time.Date(
		base.Year(),
		base.Month(),
		base.Day(),
		clock.Hour(),
		clock.Minute(),
		clock.Second(),
		0,
		loc,
	), nil
}
