//go:build !reliable_messaging_m4

package platform

import (
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"gorm.io/gorm"
)

func buildGapRecoveryStore(*gorm.DB) systemgov.GapRecoveryStore { return nil }
