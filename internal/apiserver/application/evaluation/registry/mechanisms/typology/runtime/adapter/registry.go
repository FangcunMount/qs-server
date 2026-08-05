package adapter

import (
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// ModelAdapter 计算类型学载荷 通过 人格画像流水线。
type ModelAdapter interface {
	Score(
		payload *modeltypology.Payload,
		def *modeldefinition.Definition,
		sheet *evalinput.AnswerSheet,
	) (outcometypology.ScoringResult, error)
}
