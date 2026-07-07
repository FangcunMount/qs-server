// Package policy 负责report visibility 和 audience 策略。
package policy

// Audience 标识who may 视图 report。
type Audience string

const (
	AudienceParticipant Audience = "participant"
	AudienceClinician   Audience = "clinician"
	AudienceAdmin       Audience = "admin"
)

func (a Audience) String() string { return string(a) }

// VisibilityPolicy 控制which report sections 是 visible 到 audience。
type VisibilityPolicy struct {
	Audience Audience
	Hidden   []string
}

// 默认VisibilityPolicy 返回策略 that shows 全部sections。
func DefaultVisibilityPolicy(audience Audience) VisibilityPolicy {
	return VisibilityPolicy{Audience: audience}
}

// IsVisible 报告是否 section 是 visible under 策略。
func (p VisibilityPolicy) IsVisible(section string) bool {
	for _, hidden := range p.Hidden {
		if hidden == section {
			return false
		}
	}
	return true
}
