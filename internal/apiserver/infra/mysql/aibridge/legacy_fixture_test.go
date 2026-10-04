package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

// Historical inputs are fixture data, not a retained admission or delivery API.
// Plain INSERT deliberately has no retry, deduplication or receipt settlement.
func seedLegacyStartFixture(ctx context.Context, r app.Start, s *Store) error {
	raw, hash, err := encode(r)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = persistStart(ctx, tx, r, raw, hash); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash,available_at) VALUES(?,?,'start',CONVERT(CAST(? AS BINARY) USING utf8mb4),?,UTC_TIMESTAMP(6))", r.RequestID, r.RequestID, raw, hash); err != nil {
		return err
	}
	return tx.Commit()
}

func seedLegacyChangeFixture(ctx context.Context, requestID string, r app.Change, s *Store) error {
	raw, hash, err := encode(r)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "INSERT INTO ai_bridge_commands(command_id,request_id,kind,payload,payload_hash,available_at) VALUES(?,?,?,CONVERT(CAST(? AS BINARY) USING utf8mb4),?,UTC_TIMESTAMP(6))", r.CommandID, requestID, r.Action, raw, hash)
	return err
}

func persistOriginalFixture(ctx context.Context, r app.Start, s *Store) error {
	raw, hash, err := encode(r)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = persistStart(ctx, tx, r, raw, hash); err != nil {
		return err
	}
	return tx.Commit()
}

func TestMQOriginalProjectionStoreCannotSubmitOrDeliverLegacyCommands(t *testing.T) {
	s := &Store{DB: (*sql.DB)(nil)}
	if err := s.StageStart(t.Context(), app.Start{}); !errors.Is(err, app.ErrManagementUnavailable) {
		t.Fatal("old start reached storage", err)
	}
	if err := s.StageChange(t.Context(), "", app.Change{}); !errors.Is(err, app.ErrManagementUnavailable) {
		t.Fatal("old change reached storage", err)
	}
	for _, client := range []any{s, (*MessagingCommandStore)(nil)} {
		for _, method := range []string{"Pending", "Acknowledge", "Retry"} {
			if _, exists := reflect.TypeOf(client).MethodByName(method); exists {
				t.Fatalf("legacy delivery exposed: %T.%s", client, method)
			}
		}
	}
}
