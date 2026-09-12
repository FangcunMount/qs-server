package evaluation

import (
	"encoding/json"
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"time"
)

type conductingContextPO struct {
	ID               uint64    `json:"id"`
	StartedAt        time.Time `json:"started_at"`
	StoreID          *uint64   `json:"conducting_store_id"`
	OwnershipVersion uint32    `json:"ownership_version"`
	Version          uint32    `json:"version"`
}

func encodeConductingContext(c assessment.ConductingContext) *string {
	if c.ID() == 0 {
		return nil
	}
	payload, _ := json.Marshal(conductingContextPO{c.ID(), c.StartedAt(), c.StoreID(), c.OwnershipVersion(), c.Version()})
	text := string(payload)
	return &text
}
func decodeConductingContext(raw *string) (assessment.ConductingContext, error) {
	if raw == nil {
		return assessment.ConductingContext{}, nil
	}
	var po conductingContextPO
	if err := json.Unmarshal([]byte(*raw), &po); err != nil {
		return assessment.ConductingContext{}, fmt.Errorf("corrupt conducting context: %w", err)
	}
	return assessment.NewConductingContext(po.ID, po.StartedAt, po.StoreID, po.OwnershipVersion, po.Version)
}
