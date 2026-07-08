package behavioral_rating

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
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
