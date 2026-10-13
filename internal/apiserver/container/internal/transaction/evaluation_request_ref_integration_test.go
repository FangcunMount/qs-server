//go:build integration && reliable_messaging && reliable_messaging_m4 && (reliable_messaging_m4_integration || reliable_messaging_m5)

package transaction

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

func createEvaluationRequestRefTable(ctx context.Context, db *sql.DB) error {
	path := os.Getenv("RM_QS_EVALUATION_REQUEST_REF_MIGRATION")
	if path == "" {
		path = "../../../../pkg/migration/migrations/mysql/000089_qs_evaluation_request_ref.up.sql"
	} else if path != "/tmp/m4-qs-bootstrap/mysql/000089_qs_evaluation_request_ref.up.sql" {
		return fmt.Errorf("copied invocation-owned evaluation request migration required")
	}
	ddl, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, string(ddl))
	return err
}

func createGapRecoveryRequestTable(ctx context.Context, db *sql.DB) error {
	ddl, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000090_qs_gap_recovery_request.up.sql")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, string(ddl))
	return err
}
