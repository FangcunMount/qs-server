package aibridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

const runtimeSelect = `SELECT r.request_id,COALESCE(r.session_id,''),CAST(r.testee_id AS CHAR),r.status,r.version,DATE_FORMAT(r.created_at,'%Y-%m-%dT%H:%i:%s.%fZ'),DATE_FORMAT(r.updated_at,'%Y-%m-%dT%H:%i:%s.%fZ'),
 COALESCE((SELECT JSON_ARRAYAGG(CAST(a.assessment_id AS CHAR)) FROM ai_bridge_request_assessments a WHERE a.request_id=r.request_id),JSON_ARRAY()),
 ` + runtimePendingSelect + `,
 ` + runtimeAttemptsSelect + `
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
	if err := requireRuntimeMessagingIntegrity(ctx, s.DB, org, id); err != nil {
		return app.RuntimeRequest{}, err
	}
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
	if err := requireRuntimeMessagingIntegrity(ctx, s.DB, org, q.RequestID); err != nil {
		return page, err
	}
	rows, err := s.DB.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return page, err
	}
	defer func() { _ = rows.Close() }()
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

func (s *Store) RuntimeBacklog(ctx context.Context, org int64) (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := requireRuntimeMessagingIntegrity(ctx, s.DB, org, ""); err != nil {
		return nil, err
	}
	var pending, unbound int64
	err := s.DB.QueryRowContext(ctx, `SELECT
 (SELECT COUNT(*) FROM ai_messaging_operations o
 JOIN ai_bridge_requests r ON r.request_id=o.aggregate_key AND r.organization_id=o.organization_id
 JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai' AND b.message_id=o.command_id
 WHERE o.retired=FALSE AND o.organization_id=? AND o.kind IN (1,2,3)
 AND b.kind=o.kind AND b.topic='qs.ai.commands.v1' AND b.requires_receipt=TRUE
 AND b.organization_id=o.organization_id AND b.aggregate_key=o.aggregate_key
 AND o.decision NOT IN ('accepted','rejected')),
 (SELECT COUNT(*) FROM ai_bridge_requests r WHERE r.organization_id=? AND r.session_id IS NULL)`, org, org).Scan(&pending, &unbound)
	return map[string]int64{"pending_commands": pending, "requests_without_session": unbound}, err
}
