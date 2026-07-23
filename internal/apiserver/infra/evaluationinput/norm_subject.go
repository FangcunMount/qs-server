package evaluationinput

import (
	"context"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// AgeMonthsAt computes completed age in months at asOf from birthday.
// The bool is false when inputs are incomplete or asOf is before birthday.
func AgeMonthsAt(birthday, asOf time.Time) (int, bool) {
	if birthday.IsZero() || asOf.IsZero() || asOf.Before(birthday) {
		return 0, false
	}
	years := asOf.Year() - birthday.Year()
	months := int(asOf.Month()) - int(birthday.Month())
	total := years*12 + months
	if asOf.Day() < birthday.Day() {
		total--
	}
	if total < 0 {
		return 0, false
	}
	return total, true
}

// BuildNormSubjectSnapshot freezes demographics for norm matching at asOf.
func BuildNormSubjectSnapshot(facts *port.NormSubjectFacts, asOf time.Time) *port.NormSubjectSnapshot {
	if facts == nil {
		return &port.NormSubjectSnapshot{}
	}
	snap := &port.NormSubjectSnapshot{Gender: facts.Gender}
	if facts.Birthday != nil && !asOf.IsZero() {
		if ageMonths, ok := AgeMonthsAt(*facts.Birthday, asOf); ok {
			snap.AgeMonths = &ageMonths
		}
	}
	return snap
}

func resolveNormSubject(
	ctx context.Context,
	reader port.NormSubjectReader,
	ref port.InputRef,
) (*port.NormSubjectSnapshot, error) {
	if reader == nil || ref.TesteeID == 0 {
		return nil, nil
	}
	facts, err := reader.ReadNormSubjectFacts(ctx, ref.TesteeID)
	if err != nil {
		return nil, port.NewDependencyResolveError(
			port.DependencyCategoryActor,
			err,
			"加载受试者常模信息依赖失败",
			"加载受试者常模信息失败",
		)
	}
	return BuildNormSubjectSnapshot(facts, ref.AsOf), nil
}
