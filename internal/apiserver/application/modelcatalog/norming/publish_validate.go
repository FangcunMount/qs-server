package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
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
