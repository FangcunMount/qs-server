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

// ReportProfile 标识报告呈现形态，用于 builder 路由的扩展键。
type ReportProfile string

const (
	// ReportProfileDefault 表示未指定 profile，路由时作为通配符回落到 broad builder。
	ReportProfileDefault ReportProfile = ""
)

func (p ReportProfile) String() string { return string(p) }
