package evaluation

import (
	evidence "github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

type historicalSQLRow map[string]*string

func historicalDatabase(tx *gorm.DB) (string, string, error) {
	var identity struct {
		Server   string
		Database string
	}
	if err := tx.Raw("SELECT @@server_uuid AS server,DATABASE() AS `database`").Scan(&identity).Error; err != nil {
		return "", "", err
	}
	if identity.Server == "" || identity.Database == "" {
		return "", "", evidence.ErrHistoricalReferenceInvalid
	}
	return identity.Server, identity.Database, nil
}

func historicalAllowed(table, eventType string) bool {
	if table == "evaluation_outcome" {
		return eventType == "evaluation.outcome.committed"
	}
	return table == "assessment" && (eventType == "evaluation.requested" || eventType == "evaluation.retry.requested" || eventType == "evaluation.failed")
}

func historicalDecode(raw *string) (*evidence.HistoricalReferenceSetV1, error) {
	if raw == nil {
		return nil, nil
	}
	return evidence.DecodeHistoricalReferenceSetJSON([]byte(*raw))
}
