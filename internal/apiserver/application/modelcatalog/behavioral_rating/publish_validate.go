package behavioral_rating

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
)

func validatePublishedScoreNodes(snapshot *domain.PublishedModelSnapshot) error {
	if snapshot == nil {
		return nil
	}
	parsed, err := behavioralsnapshot.ParsePublishedPayload(
		snapshot.PayloadFormat,
		snapshot.Model.Code,
		snapshot.Model.Version,
		snapshot.Model.Title,
		snapshot.Model.Status,
		snapshot.Payload,
	)
	if err != nil {
		return err
	}
	return factor.ValidateCalculationScoreNodes(parsed.Factors)
}
