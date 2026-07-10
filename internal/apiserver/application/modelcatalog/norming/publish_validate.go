package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func validatePublishedScoreNodes(model *port.PublishedModel) error {
	if model == nil {
		return nil
	}
	parsed, err := behavioralsnapshot.ParsePublishedPayload(
		model.PayloadFormat,
		model.Code,
		model.Version,
		model.Title,
		model.Status,
		model.Payload,
	)
	if err != nil {
		return err
	}
	measure := parsed.MeasureSpec()
	return factor.ValidateCalculationScoreNodesFromMeasureParts(measure.Factors, measure.FactorGraph, measure.Scoring)
}
