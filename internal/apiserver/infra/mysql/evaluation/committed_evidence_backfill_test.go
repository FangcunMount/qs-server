package evaluation

import (
	"context"
	"database/sql"
	"testing"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

func TestCommittedEvidenceBackfillRejectsInvalidTransactionWithoutPanicking(t *testing.T) {
	var nilPrepared *gorm.PreparedStmtTX
	var nilSQL *sql.Tx
	for _, test := range []struct {
		name string
		tx   *gorm.DB
	}{
		{"no_statement", &gorm.DB{}},
		{"typed_nil_prepared", &gorm.DB{Statement: &gorm.Statement{ConnPool: nilPrepared}}},
		{"typed_nil_sql", &gorm.DB{Statement: &gorm.Statement{ConnPool: nilSQL}}},
		{"prepared_nil_sql", &gorm.DB{Statement: &gorm.Statement{ConnPool: &gorm.PreparedStmtTX{Tx: nilSQL}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := BackfillCommittedEventEvidence(hostmysql.WithTx(context.Background(), test.tx), EvaluationOutcomePO{}, nil); err == nil {
				t.Fatal("invalid borrowed transaction was accepted")
			}
		})
	}
}
