package result

import evaluationscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scoring"

type ScoringSnapshotStore = evaluationscoring.ScoringSnapshotStore

type MemoryScoringSnapshotStore = evaluationscoring.MemoryScoringSnapshotStore

func NewMemoryScoringSnapshotStore() *evaluationscoring.MemoryScoringSnapshotStore {
	return evaluationscoring.NewMemoryScoringSnapshotStore()
}
