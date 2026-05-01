package testee

import (
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

var assessmentRiskTags = []Tag{TagRiskHigh, TagRiskSevere, TagRiskMedium}

// RiskTagPolicy applies assessment risk labels to a testee aggregate.
type RiskTagPolicy interface {
	ApplyAssessmentResult(testee *Testee, riskLevel string, markKeyFocus bool) (*RiskTagDecision, error)
}

// RiskTagDecision describes the aggregate changes made by RiskTagPolicy.
type RiskTagDecision struct {
	TagsAdded      []Tag
	TagsRemoved    []Tag
	KeyFocusMarked bool
}

type riskTagPolicy struct{}

// NewRiskTagPolicy creates an assessment risk tagging policy.
func NewRiskTagPolicy() RiskTagPolicy {
	return &riskTagPolicy{}
}

func (p *riskTagPolicy) ApplyAssessmentResult(testee *Testee, riskLevel string, markKeyFocus bool) (*RiskTagDecision, error) {
	if testee == nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "testee cannot be nil")
	}

	decision := &RiskTagDecision{
		TagsAdded:   make([]Tag, 0),
		TagsRemoved: make([]Tag, 0),
	}

	for _, tag := range assessmentRiskTags {
		if testee.HasTag(tag) {
			testee.removeTag(tag)
			decision.TagsRemoved = append(decision.TagsRemoved, tag)
		}
	}

	normalizedRiskLevel := strings.ToLower(riskLevel)
	for _, tag := range riskTagsForLevel(normalizedRiskLevel) {
		testee.addTag(tag)
		decision.TagsAdded = append(decision.TagsAdded, tag)
	}

	isHighRisk := normalizedRiskLevel == "high" || normalizedRiskLevel == "severe"
	switch {
	case isHighRisk && markKeyFocus:
		testee.isKeyFocus = true
	case !isHighRisk && testee.isKeyFocus && !markKeyFocus:
		testee.isKeyFocus = false
	}
	decision.KeyFocusMarked = testee.isKeyFocus

	return decision, nil
}

func riskTagsForLevel(riskLevel string) []Tag {
	switch riskLevel {
	case "high":
		return []Tag{TagRiskHigh}
	case "severe":
		return []Tag{TagRiskHigh, TagRiskSevere}
	case "medium":
		return []Tag{TagRiskMedium}
	default:
		return nil
	}
}
