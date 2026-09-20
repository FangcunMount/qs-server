package aibridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

const runtimeSelect = `SELECT r.request_id,COALESCE(r.session_id,''),CAST(r.testee_id AS CHAR),r.status,r.version,DATE_FORMAT(r.created_at,'%Y-%m-%dT%H:%i:%s.%fZ'),DATE_FORMAT(r.updated_at,'%Y-%m-%dT%H:%i:%s.%fZ'),
 COALESCE((SELECT JSON_ARRAYAGG(CAST(a.assessment_id AS CHAR)) FROM ai_bridge_request_assessments a WHERE a.request_id=r.request_id),JSON_ARRAY()),
 (SELECT COUNT(*) FROM ai_bridge_commands c WHERE c.request_id=r.request_id AND c.delivered=FALSE),
 (SELECT COALESCE(SUM(c.attempts),0) FROM ai_bridge_commands c WHERE c.request_id=r.request_id)
 FROM ai_bridge_requests r WHERE r.organization_id=?`

type scanner interface{ Scan(...any) error }

func runtimeRow(row scanner) (app.RuntimeRequest, error) {
	var r app.RuntimeRequest
	var ids []byte
	var created, updated sql.NullString
	err := row.Scan(&r.RequestID, &r.SessionID, &r.TesteeID, &r.Status, &r.Version, &created, &updated, &ids, &r.CommandsPending, &r.CommandAttempts)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(ids, &r.AssessmentIDs); err != nil {
		return r, err
	}
	if created.Valid {
		v, e := time.Parse(time.RFC3339Nano, created.String)
		if e != nil {
			return r, e
		}
		r.CreatedAt = &v
	}
	if updated.Valid {
		v, e := time.Parse(time.RFC3339Nano, updated.String)
		if e != nil {
			return r, e
		}
		r.UpdatedAt = &v
	}
	return r, nil
}

func (s *Store) GetRuntime(ctx context.Context, org int64, id string) (app.RuntimeRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	r, err := runtimeRow(s.DB.QueryRowContext(ctx, runtimeSelect+" AND r.request_id=?", org, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, app.ErrNotFound
	}
	return r, err
}

func (s *Store) ListRuntime(ctx context.Context, org int64, q app.RuntimeQuery) (app.RuntimePage, error) {
	page := app.RuntimePage{Items: []app.RuntimeRequest{}, ObservedAt: time.Now().UTC()}
	if err := q.Normalize(page.ObservedAt); err != nil {
		return page, err
	}
	var cursor *app.RuntimeCursor
	if q.Cursor != "" {
		c, err := app.DecodeRuntimeCursor(q.Cursor)
		if err != nil || c.OrganizationID != org {
			return page, app.ErrInvalid
		}
		if q.RequestID != c.Query.RequestID || q.SessionID != c.Query.SessionID || q.AssessmentID != c.Query.AssessmentID || q.TesteeID != c.Query.TesteeID || q.SubjectID != c.Query.SubjectID || q.Status != c.Query.Status || q.History != c.Query.History || q.Limit != c.Query.Limit || (!q.Since.IsZero() && !q.Since.Equal(c.Query.Since)) || (!q.Until.IsZero() && !q.Until.Equal(c.Query.Until)) {
			return page, app.ErrInvalid
		}
		q = c.Query
		cursor = &c
	}
	sqlQuery := runtimeSelect
	args := []any{org}
	add := func(fragment string, value any) { sqlQuery += fragment; args = append(args, value) }
	if q.RequestID != "" {
		add(" AND r.request_id=?", q.RequestID)
	}
	if q.SessionID != "" {
		add(" AND r.session_id=?", q.SessionID)
	}
	if q.SubjectID != "" {
		add(" AND r.subject_id=?", q.SubjectID)
	}
	if q.TesteeID != "" {
		add(" AND r.testee_id=?", q.TesteeID)
	}
	if q.Status != "" {
		add(" AND r.status=?", q.Status)
	}
	if q.AssessmentID != "" {
		add(" AND EXISTS(SELECT 1 FROM ai_bridge_request_assessments a WHERE a.request_id=r.request_id AND a.assessment_id=?)", q.AssessmentID)
	}
	if q.History {
		sqlQuery += " AND r.created_at IS NULL"
	}
	if !q.Since.IsZero() {
		add(" AND r.created_at>=?", q.Since.UTC().Format("2006-01-02 15:04:05.999999"))
	}
	if !q.Until.IsZero() {
		add(" AND r.created_at<?", q.Until.UTC().Format("2006-01-02 15:04:05.999999"))
	}
	if cursor != nil {
		if cursor.CreatedAt == nil {
			add(" AND r.created_at IS NULL AND r.request_id<?", cursor.RequestID)
		} else {
			sqlQuery += " AND (r.created_at<? OR (r.created_at=? AND r.request_id<?) OR r.created_at IS NULL)"
			stamp := cursor.CreatedAt.UTC().Format("2006-01-02 15:04:05.999999")
			args = append(args, stamp, stamp, cursor.RequestID)
		}
	}
	sqlQuery += " ORDER BY r.created_at DESC,r.request_id DESC LIMIT ?"
	args = append(args, q.Limit+1)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := s.DB.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		r, e := runtimeRow(rows)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, r)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > q.Limit {
		page.Items = page.Items[:q.Limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = app.EncodeRuntimeCursor(app.RuntimeCursor{OrganizationID: org, Query: q, CreatedAt: last.CreatedAt, RequestID: last.RequestID})
	}
	return page, nil
}

// BackfillRuntimeIndexes is bounded and resumable. It never invents historical timestamps.
func (s *Store) BackfillRuntimeIndexes(ctx context.Context, limit int) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit < 1 || limit > 500 {
		return 0, app.ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT request_id,payload FROM ai_bridge_requests WHERE organization_id IS NULL ORDER BY request_id LIMIT ? FOR UPDATE", limit)
	if err != nil {
		return 0, err
	}
	requests := []app.Start{}
	for rows.Next() {
		var id string
		var raw []byte
		var r app.Start
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		if json.Unmarshal(raw, &r) != nil || r.RequestID != id || !validExternal(r.Actor.OrgID) || !validExternal(r.TesteeID) || r.Actor.SubjectID == "" || len(r.Actor.SubjectID) > 128 || strings.ContainsRune(r.Actor.SubjectID, 0) || len(r.AssessmentIDs) == 0 {
			rows.Close()
			return 0, app.ErrInvalid
		}
		for _, a := range r.AssessmentIDs {
			if !validExternal(a) {
				rows.Close()
				return 0, app.ErrInvalid
			}
		}
		requests = append(requests, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, r := range requests {
		if _, err = tx.ExecContext(ctx, "UPDATE ai_bridge_requests SET organization_id=?,subject_id=?,testee_id=? WHERE request_id=? AND organization_id IS NULL", r.Actor.OrgID, r.Actor.SubjectID, r.TesteeID, r.RequestID); err != nil {
			return 0, err
		}
		for _, a := range r.AssessmentIDs {
			if _, err = tx.ExecContext(ctx, "INSERT IGNORE INTO ai_bridge_request_assessments(request_id,assessment_id) VALUES(?,?)", r.RequestID, a); err != nil {
				return 0, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(requests), nil
}
func validExternal(v string) bool {
	n, e := strconv.ParseUint(v, 10, 64)
	return e == nil && n > 0 && strconv.FormatUint(n, 10) == v
}

func (s *Store) RuntimeBacklog(ctx context.Context, org int64) (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var pending, unbound int64
	err := s.DB.QueryRowContext(ctx, `SELECT
 (SELECT COUNT(*) FROM ai_bridge_commands c JOIN ai_bridge_requests r ON r.request_id=c.request_id WHERE r.organization_id=? AND c.delivered=FALSE),
 (SELECT COUNT(*) FROM ai_bridge_requests r WHERE r.organization_id=? AND r.session_id IS NULL)`, org, org).Scan(&pending, &unbound)
	return map[string]int64{"pending_commands": pending, "requests_without_session": unbound}, err
}
