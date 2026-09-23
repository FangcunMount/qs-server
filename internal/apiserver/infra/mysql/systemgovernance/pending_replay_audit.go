package systemgovernance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
)

// ListPendingReplayAudits exposes only unresolved replay approvals in one
// organization. An ID cursor keeps pagination stable as audits are resolved.
func (s *ActionAuditStore) ListPendingReplayAudits(ctx context.Context, orgID int64, cursor string, limit int) (app.PendingReplayAuditPage, error) {
	if s == nil || s.db == nil || orgID <= 0 || limit < 1 || limit > 100 {
		return app.PendingReplayAuditPage{}, fmt.Errorf("pending replay audit query requires a database, organization, and limit from 1 to 100")
	}
	var beforeID uint64
	if cursor != "" {
		var err error
		beforeID, err = strconv.ParseUint(cursor, 10, 64)
		if err != nil || beforeID == 0 {
			return app.PendingReplayAuditPage{}, fmt.Errorf("invalid pending replay audit cursor")
		}
	}
	query := s.db.WithContext(ctx).Model(&actionRunPO{}).
		Where("org_id = ? AND action_id = ? AND status = ?", orgID, "events.replay_pending", app.ActionAuditStatusPendingReconciliation)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	var rows []actionRunPO
	if err := query.Order("id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return app.PendingReplayAuditPage{}, err
	}
	page := app.PendingReplayAuditPage{Items: make([]app.PendingReplayAudit, 0, min(limit, len(rows)))}
	if len(rows) > limit {
		page.NextCursor = strconv.FormatUint(rows[limit-1].ID, 10)
		rows = rows[:limit]
	}
	for _, row := range rows {
		decoder := json.NewDecoder(strings.NewReader(row.InputJSON))
		decoder.UseNumber()
		var input map[string]interface{}
		if err := decoder.Decode(&input); err != nil {
			return app.PendingReplayAuditPage{}, fmt.Errorf("decode pending replay audit %d: %w", row.ID, err)
		}
		store, _ := input["store"].(string)
		if store == "" {
			return app.PendingReplayAuditPage{}, fmt.Errorf("pending replay audit %d has no store", row.ID)
		}
		page.Items = append(page.Items, app.PendingReplayAudit{
			RequestID: row.RequestID, ActorUserID: strconv.FormatUint(row.ActorUserID, 10), Store: store,
			Input: input, StartedAt: row.StartedAt, UpdatedAt: row.UpdatedAt,
		})
	}
	return page, nil
}

var _ app.PendingReplayAuditReader = (*ActionAuditStore)(nil)
