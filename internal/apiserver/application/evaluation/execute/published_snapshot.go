package execute

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func publishedSnapshotFromInput(input *evaluationinput.InputSnapshot) (modelcatalog.PublishedModelSnapshot, bool) {
	return evaloutcome.PublishedSnapshotFromInput(input)
}
