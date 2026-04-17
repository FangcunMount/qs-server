package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	seedRelationPrimaryOffset      = 2 * time.Hour
	seedRelationAttendingOffset    = 4 * time.Hour
	seedRelationCollaboratorOffset = 6 * time.Hour
	seedEntryResolveMinDelay       = 24 * time.Hour
	seedEntryResolveOffset         = 30 * time.Minute
	seedEntryIntakeOffset          = 10 * time.Minute
	seedEntryAttendingOffset       = 1 * time.Minute
	seedEntryAssessmentOffset      = 20 * time.Minute
	seedAssessmentInterpretOffset  = 30 * time.Second
	seedClinicianCreatedLead       = 7 * 24 * time.Hour
	seedStaffCreatedLead           = 24 * time.Hour
	seedEntryFlowPageSize          = 100
	seedEntryFlowDefaultMaxIntakes = 5
	seedByEntryDefaultMaxCount     = 5
	seedAssessmentPollInterval     = 500 * time.Millisecond
	seedAssessmentPollTimeout      = 20 * time.Second
	defaultActorWaveInterval       = 90 * 24 * time.Hour
	defaultActorWaveWeeks          = 4
	defaultActorWaveDayStartHour   = 9
	defaultActorWaveDayEndHour     = 18
	defaultActorWaveSlotInterval   = 10 * time.Minute
)

var defaultActorWaveDaysOfWeek = []time.Weekday{time.Monday, time.Wednesday, time.Friday}

type actorWaveSchedule struct {
	WaveInterval time.Duration
	WaveWeeks    int
	DayStartHour int
	DayEndHour   int
	SlotInterval time.Duration
	WaveDays     []time.Weekday
}

type actorWaveAllocator struct {
	schedule actorWaveSchedule
	anchor   time.Time
	nextSlot time.Time
}

func deriveRelationBoundAt(testeeCreatedAt time.Time, relationType string) (time.Time, error) {
	base := testeeCreatedAt.Round(0)
	if base.IsZero() {
		return time.Time{}, fmt.Errorf("testee created_at is zero")
	}

	switch strings.ToLower(strings.TrimSpace(relationType)) {
	case "primary":
		return base.Add(seedRelationPrimaryOffset), nil
	case "attending", "assigned":
		return base.Add(seedRelationAttendingOffset), nil
	case "collaborator":
		return base.Add(seedRelationCollaboratorOffset), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported relation type %q", relationType)
	}
}

func deriveEntryResolveAt(entryCreatedAt, testeeCreatedAt time.Time) time.Time {
	entryAnchor := entryCreatedAt.Round(0).Add(seedEntryResolveOffset)
	testeeAnchor := testeeCreatedAt.Round(0).Add(seedEntryResolveMinDelay)
	if entryAnchor.Before(testeeAnchor) {
		return testeeAnchor
	}
	return entryAnchor
}

func deriveEntryIntakeAt(resolveAt time.Time) time.Time {
	return resolveAt.Round(0).Add(seedEntryIntakeOffset)
}

func deriveEntryAccessRelationAt(intakeAt time.Time) time.Time {
	return intakeAt.Round(0).Add(seedEntryAttendingOffset)
}

func deriveEntryAssessmentSubmitAt(intakeAt time.Time) time.Time {
	return intakeAt.Round(0).Add(seedEntryAssessmentOffset)
}

func deriveAssessmentInterpretAt(submittedAt time.Time) time.Time {
	return submittedAt.Round(0).Add(seedAssessmentInterpretOffset)
}

func deriveClinicianCreatedAt(firstBoundAt time.Time) time.Time {
	if firstBoundAt.IsZero() {
		return time.Time{}
	}
	return firstBoundAt.Round(0).Add(-seedClinicianCreatedLead)
}

func deriveStaffCreatedAt(clinicianCreatedAt time.Time) time.Time {
	if clinicianCreatedAt.IsZero() {
		return time.Time{}
	}
	return clinicianCreatedAt.Round(0).Add(-seedStaffCreatedLead)
}

func normalizeMaxIntakesPerEntry(value int) int {
	if value <= 0 {
		return seedEntryFlowDefaultMaxIntakes
	}
	return value
}

func normalizeMaxAssessmentsPerEntry(value int) int {
	if value <= 0 {
		return seedByEntryDefaultMaxCount
	}
	return value
}

func flexibleIDSet(values []FlexibleID) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value.IsZero() {
			continue
		}
		result[value.String()] = struct{}{}
	}
	return result
}

func sortClinicianRelationsByTesteeCreatedAt(items []*ClinicianRelationResponse) {
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left == nil || left.Testee == nil {
			return false
		}
		if right == nil || right.Testee == nil {
			return true
		}
		if !left.Testee.CreatedAt.Equal(right.Testee.CreatedAt) {
			return left.Testee.CreatedAt.Before(right.Testee.CreatedAt)
		}
		return parseID(left.Testee.ID) < parseID(right.Testee.ID)
	})
}

func isAccessGrantRelationType(relationType string) bool {
	switch strings.ToLower(strings.TrimSpace(relationType)) {
	case "primary", "attending", "assigned", "collaborator":
		return true
	default:
		return false
	}
}

func normalizeActorWaveSchedule(cfg ActorTimelineConfig) (actorWaveSchedule, error) {
	schedule := actorWaveSchedule{
		WaveInterval: defaultActorWaveInterval,
		WaveWeeks:    defaultActorWaveWeeks,
		DayStartHour: defaultActorWaveDayStartHour,
		DayEndHour:   defaultActorWaveDayEndHour,
		SlotInterval: defaultActorWaveSlotInterval,
		WaveDays:     append([]time.Weekday(nil), defaultActorWaveDaysOfWeek...),
	}

	if raw := strings.TrimSpace(cfg.WaveInterval); raw != "" {
		duration, err := parseSeedRelativeDuration(raw)
		if err != nil {
			return actorWaveSchedule{}, fmt.Errorf("invalid actorTimeline.waveInterval %q: %w", raw, err)
		}
		if duration <= 0 {
			return actorWaveSchedule{}, fmt.Errorf("actorTimeline.waveInterval must be greater than 0")
		}
		schedule.WaveInterval = duration
	}
	if cfg.WaveWeeks > 0 {
		schedule.WaveWeeks = cfg.WaveWeeks
	}
	if cfg.DayStartHour > 0 {
		schedule.DayStartHour = cfg.DayStartHour
	}
	if cfg.DayEndHour > 0 {
		schedule.DayEndHour = cfg.DayEndHour
	}
	if raw := strings.TrimSpace(cfg.SlotInterval); raw != "" {
		duration, err := parseSeedRelativeDuration(raw)
		if err != nil {
			return actorWaveSchedule{}, fmt.Errorf("invalid actorTimeline.slotInterval %q: %w", raw, err)
		}
		if duration <= 0 {
			return actorWaveSchedule{}, fmt.Errorf("actorTimeline.slotInterval must be greater than 0")
		}
		schedule.SlotInterval = duration
	}
	if len(cfg.WaveDaysOfWeek) > 0 {
		days := make([]time.Weekday, 0, len(cfg.WaveDaysOfWeek))
		seen := make(map[time.Weekday]struct{}, len(cfg.WaveDaysOfWeek))
		for _, rawDay := range cfg.WaveDaysOfWeek {
			weekday, err := parseActorWaveWeekday(rawDay)
			if err != nil {
				return actorWaveSchedule{}, err
			}
			if _, ok := seen[weekday]; ok {
				continue
			}
			seen[weekday] = struct{}{}
			days = append(days, weekday)
		}
		sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })
		schedule.WaveDays = days
	}

	if schedule.WaveWeeks <= 0 {
		return actorWaveSchedule{}, fmt.Errorf("actorTimeline.waveWeeks must be greater than 0")
	}
	if schedule.DayStartHour < 0 || schedule.DayStartHour > 23 {
		return actorWaveSchedule{}, fmt.Errorf("actorTimeline.dayStartHour must be between 0 and 23")
	}
	if schedule.DayEndHour <= schedule.DayStartHour || schedule.DayEndHour > 24 {
		return actorWaveSchedule{}, fmt.Errorf("actorTimeline.dayEndHour must be greater than dayStartHour and at most 24")
	}
	if len(schedule.WaveDays) == 0 {
		return actorWaveSchedule{}, fmt.Errorf("actorTimeline.waveDaysOfWeek must not be empty")
	}
	if schedule.SlotInterval <= 0 {
		return actorWaveSchedule{}, fmt.Errorf("actorTimeline.slotInterval must be greater than 0")
	}
	return schedule, nil
}

func parseActorWaveWeekday(value int) (time.Weekday, error) {
	switch value {
	case 1:
		return time.Monday, nil
	case 2:
		return time.Tuesday, nil
	case 3:
		return time.Wednesday, nil
	case 4:
		return time.Thursday, nil
	case 5:
		return time.Friday, nil
	case 6:
		return time.Saturday, nil
	case 7, 0:
		return time.Sunday, nil
	default:
		return time.Sunday, fmt.Errorf("actorTimeline.waveDaysOfWeek contains unsupported day %d", value)
	}
}

func newActorWaveAllocator(firstBoundAt time.Time, schedule actorWaveSchedule) *actorWaveAllocator {
	anchorDay := startOfWeek(firstBoundAt)
	anchor := time.Date(anchorDay.Year(), anchorDay.Month(), anchorDay.Day(), schedule.DayStartHour, 0, 0, 0, firstBoundAt.Location())
	return &actorWaveAllocator{
		schedule: schedule,
		anchor:   anchor,
	}
}

func (a *actorWaveAllocator) NextAtOrAfter(minAt time.Time) time.Time {
	candidate := minAt.Round(0)
	if !a.nextSlot.IsZero() && a.nextSlot.After(candidate) {
		candidate = a.nextSlot
	}
	slot := a.align(candidate)
	a.nextSlot = a.align(slot.Add(a.schedule.SlotInterval))
	return slot
}

func (a *actorWaveAllocator) align(candidate time.Time) time.Time {
	if candidate.Before(a.anchor) {
		candidate = a.anchor
	}
	for {
		waveStart := a.waveStartFor(candidate)
		if slot, ok := a.alignWithinWave(waveStart, candidate); ok {
			return slot
		}
		candidate = waveStart.Add(a.schedule.WaveInterval)
	}
}

func (a *actorWaveAllocator) waveStartFor(candidate time.Time) time.Time {
	if candidate.Before(a.anchor) {
		return a.anchor
	}
	delta := candidate.Sub(a.anchor)
	waveOffset := (delta / a.schedule.WaveInterval) * a.schedule.WaveInterval
	return a.anchor.Add(waveOffset)
}

func (a *actorWaveAllocator) alignWithinWave(waveStart, candidate time.Time) (time.Time, bool) {
	waveEnd := waveStart.Add(time.Duration(a.schedule.WaveWeeks) * 7 * 24 * time.Hour)
	if !candidate.Before(waveEnd) {
		return time.Time{}, false
	}

	dayCursor := startOfDay(maxTime(candidate, waveStart))
	for dayCursor.Before(waveEnd) {
		if a.isWaveActiveDay(waveStart, dayCursor) {
			slot := time.Date(dayCursor.Year(), dayCursor.Month(), dayCursor.Day(), a.schedule.DayStartHour, 0, 0, 0, dayCursor.Location())
			dayEnd := time.Date(dayCursor.Year(), dayCursor.Month(), dayCursor.Day(), a.schedule.DayEndHour, 0, 0, 0, dayCursor.Location())
			if sameDate(dayCursor, candidate) && candidate.After(slot) {
				slot = roundUpTime(candidate, a.schedule.SlotInterval)
			}
			if slot.Before(dayEnd) && !slot.Before(waveStart) {
				return slot, true
			}
		}
		dayCursor = dayCursor.AddDate(0, 0, 1)
	}
	return time.Time{}, false
}

func (a *actorWaveAllocator) isWaveActiveDay(waveStart, day time.Time) bool {
	weekIndex := int(startOfDay(day).Sub(startOfDay(waveStart)) / (7 * 24 * time.Hour))
	if weekIndex < 0 || weekIndex >= a.schedule.WaveWeeks {
		return false
	}
	for _, weekday := range a.schedule.WaveDays {
		if day.Weekday() == weekday {
			return true
		}
	}
	return false
}

func startOfWeek(value time.Time) time.Time {
	day := startOfDay(value)
	offset := int(day.Weekday() - time.Monday)
	if offset < 0 {
		offset += 7
	}
	return day.AddDate(0, 0, -offset)
}

func startOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func sameDate(left, right time.Time) bool {
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}

func roundUpTime(value time.Time, interval time.Duration) time.Time {
	if interval <= 0 {
		return value
	}
	remainder := value.Sub(startOfDay(value)) % interval
	if remainder == 0 {
		return value
	}
	return value.Add(interval - remainder)
}

func maxTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}
