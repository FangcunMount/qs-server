//go:build integration

package aibridge

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
)

func TestMQRetirementSchemaEnforcedAndIndexesRequired(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return RequireMessagingOperationSchema(context.Background(), tx) }))
	for _, values := range []string{
		"NULL,1,UTC_TIMESTAMP(6),FALSE,NULL,NULL,'',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1),UTC_TIMESTAMP(6),'',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1),UTC_TIMESTAMP(6),'accepted',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1),UTC_TIMESTAMP(6),'','invented receipt'",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1,'ownership_verified','true','responsibility_closed','true','business_terminal','true'),UTC_TIMESTAMP(6),'',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version','1','ownership_verified',CAST('true' AS JSON),'responsibility_closed',CAST('true' AS JSON),'business_terminal',CAST('true' AS JSON)),UTC_TIMESTAMP(6),'',NULL",
	} {
		_, err := f.db.Exec("INSERT INTO ai_messaging_operations(command_id,kind,organization_id,subject_id,resource_id,aggregate_key,body_sha256,aggregate_sequence,created_at,retired,retirement_evidence,retired_at,decision,receipt) VALUES(?,1,1,'42',?,?,"+values+")", uuid.NewString(), f.request.RequestID, f.request.RequestID)
		if err == nil {
			t.Fatal("invalid retirement shape passed CHECK", values)
		}
	}
}
