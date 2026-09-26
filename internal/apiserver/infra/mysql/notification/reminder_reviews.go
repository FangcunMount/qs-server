package notification

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	appnotification "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
)

// ListReminderReviews exposes only the fields needed to locate and reconcile
// an uncertain external call. It never returns an OpenID, claim token, or
// bearer entry URL, and it never changes delivery state.
func (s *ReminderDeliveryLedger) ListReminderReviews(
	ctx context.Context, orgID int64, sendingOlderThan time.Time, cursor string, limit int,
) (systemgov.ReminderReviewPage, error) {
	if s == nil || s.db == nil || orgID <= 0 || sendingOlderThan.IsZero() || limit < 1 || limit > 100 {
		return systemgov.ReminderReviewPage{}, fmt.Errorf("reminder review requires a database, organization, cutoff, and limit from 1 to 100")
	}
	query := s.db.WithContext(ctx).Model(&reminderDeliveryPO{}).Where(
		"org_id = ? AND ((state = ? AND updated_at <= ?) OR state = ?)",
		orgID, appnotification.ReminderSending, sendingOlderThan, appnotification.ReminderManualRequired,
	)
	if cursor != "" {
		updatedAt, id, err := parseReminderReviewCursor(cursor)
		if err != nil {
			return systemgov.ReminderReviewPage{}, err
		}
		query = query.Where("(updated_at < ? OR (updated_at = ? AND id < ?))", updatedAt, updatedAt, id)
	}
	var rows []reminderDeliveryPO
	if err := query.Order("updated_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return systemgov.ReminderReviewPage{}, fmt.Errorf("list reminder reviews: %w", err)
	}
	page := systemgov.ReminderReviewPage{Items: make([]systemgov.ReminderReview, 0, min(limit, len(rows)))}
	if len(rows) > limit {
		last := rows[limit-1]
		page.NextCursor = reminderReviewCursor(last.UpdatedAt, last.ID)
		rows = rows[:limit]
	}
	for _, row := range rows {
		page.Items = append(page.Items, systemgov.ReminderReview{
			DeliveryID: row.ID,
			TaskID:     row.TaskID, OpeningEventID: row.OpeningEventID,
			ScheduleRevision: row.ScheduleRevision, UserID: row.UserID,
			State: row.State, ExternalCallStartedAt: row.ExternalCallStartedAt,
			ResolutionCode: row.ResolutionCode, UpdatedAt: row.UpdatedAt,
		})
	}
	return page, nil
}

func reminderReviewCursor(updatedAt time.Time, id uint64) string {
	// Keep the database connection's wall-time offset for DATETIME cursor
	// comparisons; converting to UTC could shift the compared SQL value.
	value := updatedAt.Format(time.RFC3339Nano) + "|" + strconv.FormatUint(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func parseReminderReviewCursor(cursor string) (time.Time, uint64, error) {
	if len(cursor) > 140 {
		return time.Time{}, 0, fmt.Errorf("invalid reminder review cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(raw) > 100 {
		return time.Time{}, 0, fmt.Errorf("invalid reminder review cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid reminder review cursor")
	}
	updatedAt, timeErr := time.Parse(time.RFC3339Nano, parts[0])
	id, idErr := strconv.ParseUint(parts[1], 10, 64)
	if timeErr != nil || idErr != nil || updatedAt.IsZero() || id == 0 {
		return time.Time{}, 0, fmt.Errorf("invalid reminder review cursor")
	}
	return updatedAt, id, nil
}

var _ systemgov.ReminderReviewReader = (*ReminderDeliveryLedger)(nil)
