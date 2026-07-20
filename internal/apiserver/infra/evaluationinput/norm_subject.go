package evaluationinput

import (
	"context"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// AgeMonthsAt computes completed age in months at asOf from birthday.
// Returns 0 when inputs are incomplete or asOf is before birthday.
func AgeMonthsAt(birthday, asOf time.Time) int {
	if birthday.IsZero() || asOf.IsZero() || asOf.Before(birthday) {
		return 0
	}
	years := asOf.Year() - birthday.Year()
	months := int(asOf.Month()) - int(birthday.Month())
	total := years*12 + months
	if asOf.Day() < birthday.Day() {
		total--
	}
	if total < 0 {
		return 0
	}
	return total
}

// BuildNormSubjectSnapshot freezes demographics for norm matching at asOf.
func BuildNormSubjectSnapshot(facts *port.NormSubjectFacts, asOf time.Time) *port.NormSubjectSnapshot {
	if facts == nil {
		return &port.NormSubjectSnapshot{}
	}
	snap := &port.NormSubjectSnapshot{Gender: facts.Gender}
	if facts.Birthday != nil && !asOf.IsZero() {
		snap.AgeMonths = AgeMonthsAt(*facts.Birthday, asOf)
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
		return nil, err
	}
	return BuildNormSubjectSnapshot(facts, ref.AsOf), nil
}
