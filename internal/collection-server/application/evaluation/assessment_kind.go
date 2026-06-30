package evaluation

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidAssessmentKind = errors.New("invalid assessment_kind")

// NormalizeAssessmentKind maps REST assessment_kind to evaluation_model_kind.
func NormalizeAssessmentKind(raw string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "":
		return "", nil
	case "medical":
		return "scale", nil
	case "personality":
		return "personality", nil
	default:
		return "", fmt.Errorf("%w: %q (allowed: medical, personality)", ErrInvalidAssessmentKind, raw)
	}
}
