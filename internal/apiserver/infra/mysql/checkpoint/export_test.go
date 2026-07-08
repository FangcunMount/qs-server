package checkpoint

import evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"

// RunToPOForTest exposes run mapping for infra tests.
func RunToPOForTest(run evalrun.EvaluationRun) *RuntimeCheckpointPO {
	return runToPO(run)
}

// RunFromPOForTest exposes run mapping for infra tests.
func RunFromPOForTest(po RuntimeCheckpointPO) evalrun.EvaluationRun {
	return runFromPO(po)
}
