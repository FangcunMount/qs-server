package aibridge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

type Store struct{ DB *sql.DB }

func encode(v any) ([]byte, string, error) {
	raw, err := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), err
}

// The original projection store is query-only for command submission. Old
// callers are refused before storage; only MessagingCommandStore can submit.
func (*Store) StageStart(context.Context, app.Start) error { return app.ErrManagementUnavailable }
func (*Store) StageChange(context.Context, string, app.Change) error {
	return app.ErrManagementUnavailable
}

// persistStart binds the original request hash and evidence on the host root transaction.
func persistStart(ctx context.Context, tx *sql.Tx, r app.Start, raw []byte, hash string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload,organization_id,subject_id,testee_id,created_at,updated_at) VALUES(?,?,CONVERT(CAST(? AS BINARY) USING utf8mb4),?,?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE request_id=request_id", r.RequestID, hash, raw, r.Actor.OrgID, r.Actor.SubjectID, r.TesteeID)
	if err != nil {
		return err
	}
	var stored string
	if err = tx.QueryRowContext(ctx, "SELECT request_hash FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", r.RequestID).Scan(&stored); err != nil {
		return err
	}
	if stored != hash {
		return app.ErrConflict
	}
	for _, assessmentID := range r.AssessmentIDs {
		if _, err = tx.ExecContext(ctx, "INSERT IGNORE INTO ai_bridge_request_assessments(request_id,assessment_id) VALUES(?,?)", r.RequestID, assessmentID); err != nil {
			return err
		}
	}
	return nil
}

func validateChange(ctx context.Context, tx *sql.Tx, id string, r app.Change) error {
	var original []byte
	var session sql.NullString
	err := tx.QueryRowContext(ctx, "SELECT CAST(payload AS BINARY),session_id FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", id).Scan(&original, &session)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrNotFound
	}
	if err != nil {
		return err
	}
	var request app.Start
	if err = json.Unmarshal(original, &request); err != nil {
		return err
	}
	if request.Actor != r.Actor || !session.Valid || session.String != r.SessionID {
		return app.ErrConflict
	}
	return nil
}

// acceptInTransaction borrows the MQ receiver's transaction; business projection,
// Inbox and final acknowledgement are committed by that one caller.
func acceptInTransaction(ctx context.Context, tx *sql.Tx, e app.Event) error {
	artifact, err := app.ValidateArtifact(e)
	if err != nil {
		return err
	}
	raw, hash, err := encode(e)
	if err != nil {
		return err
	}
	return acceptPreparedInTransaction(ctx, tx, e, artifact, raw, hash)
}

func acceptPreparedInTransaction(ctx context.Context, tx *sql.Tx, e app.Event, artifact *app.Artifact, raw []byte, hash string) error {
	var original []byte
	var bound sql.NullString
	var version int64
	var state string
	err := tx.QueryRowContext(ctx, "SELECT CAST(payload AS BINARY),session_id,version,status FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", e.RequestID).Scan(&original, &bound, &version, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrNotFound
	}
	if err != nil {
		return err
	}
	var request app.Start
	if err = json.Unmarshal(original, &request); err != nil {
		return err
	}
	if request.Actor != e.Actor || request.TesteeID != e.TesteeID || (bound.Valid && bound.String != e.SessionID) {
		return app.ErrConflict
	}
	if artifact != nil {
		if len(request.Evidence) != 1 || len(request.AssessmentIDs) != 1 {
			return app.ErrConflict
		}
		source := request.Evidence[0]
		if artifact.AssessmentID != request.AssessmentIDs[0] || artifact.AssessmentID != source.AssessmentID || artifact.ReportID != source.ReportID || artifact.SourceVersion != source.SourceVersion {
			return app.ErrConflict
		}
	}
	var oldHash string
	err = tx.QueryRowContext(ctx, "SELECT payload_hash FROM ai_bridge_events WHERE event_id=? OR (request_id=? AND version=?)", e.EventID, e.RequestID, e.Version).Scan(&oldHash)
	if err == nil {
		if oldHash != hash {
			return app.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (state == "cancelled" || state == "completed") && e.Version > version {
		return app.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO ai_bridge_events(event_id,request_id,version,payload_hash) VALUES(?,?,?,?)", e.EventID, e.RequestID, e.Version, hash); err != nil {
		return err
	}
	if e.Version > version {
		// Decode the original UTF-8 bytes explicitly for the JSON column. The
		// borrowed host pool may negotiate utf8mb3; it cannot represent four-byte
		// Unicode. Keep the original bytes/hash and the host connection unchanged.
		_, err = tx.ExecContext(ctx, "UPDATE ai_bridge_requests SET session_id=?,version=?,status=?,projection=CONVERT(CAST(? AS BINARY) USING utf8mb4),updated_at=UTC_TIMESTAMP(6) WHERE request_id=?", e.SessionID, e.Version, e.Status, raw, e.RequestID)
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) Projection(ctx context.Context, id string) (*app.Event, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT CAST(projection AS BINARY) FROM ai_bridge_requests WHERE request_id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, app.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var result app.Event
	err = json.Unmarshal(raw, &result)
	return &result, err
}

func (s *Store) Original(ctx context.Context, id string) (*app.Start, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT CAST(payload AS BINARY) FROM ai_bridge_requests WHERE request_id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, app.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var result app.Start
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
