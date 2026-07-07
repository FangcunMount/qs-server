// Package policy owns report visibility and audience policies.
package policy

// Audience identifies who may view a report.
type Audience string

const (
	AudienceParticipant Audience = "participant"
	AudienceClinician   Audience = "clinician"
	AudienceAdmin       Audience = "admin"
)

func (a Audience) String() string { return string(a) }

// VisibilityPolicy controls which report sections are visible to an audience.
type VisibilityPolicy struct {
	Audience Audience
	Hidden   []string
}

// DefaultVisibilityPolicy returns a policy that shows all sections.
func DefaultVisibilityPolicy(audience Audience) VisibilityPolicy {
	return VisibilityPolicy{Audience: audience}
}

// IsVisible reports whether a section is visible under the policy.
func (p VisibilityPolicy) IsVisible(section string) bool {
	for _, hidden := range p.Hidden {
		if hidden == section {
			return false
		}
	}
	return true
}
