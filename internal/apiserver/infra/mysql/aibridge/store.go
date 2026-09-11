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
func (s *Store) StageStart(ctx context.Context, r app.Start) error {
	raw, hash, err := encode(r)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload) VALUES(?,?,?) ON DUPLICATE KEY UPDATE request_id=request_id", r.RequestID, hash, raw)
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
	if err = stage(ctx, tx, r.RequestID, r.RequestID, "start", raw, hash); err != nil {
		return err
	}
	return tx.Commit()
}
func stage(ctx context.Context, tx *sql.Tx, id, requestID, kind string, raw []byte, hash string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE command_id=command_id", id, requestID, kind, raw, hash)
	if err != nil {
		return err
	}
	var oldHash, oldRequest string
	err = tx.QueryRowContext(ctx, "SELECT payload_hash,request_id FROM ai_bridge_commands WHERE command_id=? FOR UPDATE", id).Scan(&oldHash, &oldRequest)
	if err != nil {
		return err
	}
	if oldHash != hash || oldRequest != requestID {
		return app.ErrConflict
	}
	return nil
}
func (s *Store) StageChange(ctx context.Context, id string, r app.Change) error {
	raw, hash, err := encode(r)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var original []byte
	var session sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT payload,session_id FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", id).Scan(&original, &session)
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
	if err = stage(ctx, tx, r.CommandID, id, r.Action, raw, hash); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Pending(ctx context.Context, limit int) ([]app.Command, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT command_id,request_id,kind,payload FROM ai_bridge_commands WHERE delivered=FALSE AND available_at<=UTC_TIMESTAMP(6) ORDER BY available_at,command_id LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []app.Command{}
	for rows.Next() {
		var c app.Command
		if err = rows.Scan(&c.ID, &c.RequestID, &c.Kind, &c.Payload); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
func (s *Store) Acknowledge(ctx context.Context, c app.Command, r app.Receipt) error {
	if r.SessionID == "" || r.Version < 1 {
		return app.ErrConflict
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var bound sql.NullString
	if err = tx.QueryRowContext(ctx, "SELECT session_id FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", c.RequestID).Scan(&bound); err != nil {
		return err
	}
	if bound.Valid && bound.String != r.SessionID {
		return app.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "UPDATE ai_bridge_requests SET session_id=? WHERE request_id=?", r.SessionID, c.RequestID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE ai_bridge_commands SET delivered=TRUE WHERE command_id=?", c.ID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Retry(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, "UPDATE ai_bridge_commands SET available_at=TIMESTAMPADD(SECOND,LEAST(60,POW(2,LEAST(attempts,6))),UTC_TIMESTAMP(6)),attempts=attempts+1 WHERE command_id=? AND delivered=FALSE", id)
	return err
}
func (s *Store) Accept(ctx context.Context, e app.Event) error {
	raw, hash, err := encode(e)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var original []byte
	var bound sql.NullString
	var version int64
	var state string
	err = tx.QueryRowContext(ctx, "SELECT payload,session_id,version,status FROM ai_bridge_requests WHERE request_id=? FOR UPDATE", e.RequestID).Scan(&original, &bound, &version, &state)
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
	var oldHash string
	err = tx.QueryRowContext(ctx, "SELECT payload_hash FROM ai_bridge_events WHERE event_id=? OR (request_id=? AND version=?)", e.EventID, e.RequestID, e.Version).Scan(&oldHash)
	if err == nil {
		if oldHash != hash {
			return app.ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if state == "cancelled" && e.Version > version && e.Status != "cancelled" {
		return app.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO ai_bridge_events(event_id,request_id,version,payload_hash) VALUES(?,?,?,?)", e.EventID, e.RequestID, e.Version, hash); err != nil {
		return err
	}
	if e.Version > version {
		_, err = tx.ExecContext(ctx, "UPDATE ai_bridge_requests SET session_id=?,version=?,status=?,projection=? WHERE request_id=?", e.SessionID, e.Version, e.Status, raw, e.RequestID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) Projection(ctx context.Context, id string) (*app.Event, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT projection FROM ai_bridge_requests WHERE request_id=?", id).Scan(&raw)
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
