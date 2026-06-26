package platform

import codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"

// NewCodesService builds the default codes application service.
func NewCodesService() codesapp.CodesService {
	return codesapp.NewService()
}
