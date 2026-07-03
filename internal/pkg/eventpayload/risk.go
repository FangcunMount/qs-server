package eventpayload

import "github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"

// IsHighRiskCode reports whether code maps to high or severe risk.
func IsHighRiskCode(code string) bool {
	return eventoutcome.IsHighRiskCode(code)
}
