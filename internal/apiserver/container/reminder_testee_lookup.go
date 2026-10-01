package container

import (
	"context"
	"fmt"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

// reminderTesteeLookup reads only the facts needed by a trusted, already
// admitted Task opening. A background consumer has no operator authorization
// snapshot, so it must not call the operator-facing TesteeQuery service.
type reminderTesteeLookup struct {
	reader interface {
		GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error)
	}
}

func (r reminderTesteeLookup) GetByID(ctx context.Context, id uint64) (*testeeApp.TesteeResult, error) {
	if r.reader == nil || id == 0 {
		return nil, fmt.Errorf("reminder testee reader and ID are required")
	}
	row, err := r.reader.GetTestee(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil || row.ID != id {
		return nil, fmt.Errorf("reminder testee is unavailable")
	}
	return &testeeApp.TesteeResult{
		ID: row.ID, OrgID: row.OrgID, ProfileID: row.ProfileID, Source: row.Source,
	}, nil
}
