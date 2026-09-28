//go:build integration && reliable_messaging && reliable_messaging_m4 && (reliable_messaging_m4_integration || reliable_messaging_m5)

package transaction

import (
	"context"
	"database/sql"
	"os"
)

func createEvaluationRequestRefTable(ctx context.Context, db *sql.DB) error {
	ddl, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000089_qs_evaluation_request_ref.up.sql")
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, string(ddl))
	return err
}
