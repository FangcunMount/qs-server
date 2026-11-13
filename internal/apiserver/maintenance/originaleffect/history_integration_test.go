//go:build integration

package originaleffect

import (
	"os"
	"strings"
	"testing"
	"time"

	domainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/reliable-messaging/message"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOriginalRetryReadsArchivedSourceNativeMySQL(t *testing.T) {
	dsn := os.Getenv("QS_F06_HISTORY_MYSQL_DSN")
	if dsn == "" {
		t.Skip("dedicated local history fixture required")
	}
	cfg, err := driver.ParseDSN(dsn)
	require.NoError(t, err)
	require.Equal(t, "qs_f06_history", cfg.DBName)
	require.Equal(t, "tcp", cfg.Net)
	require.True(t, strings.HasPrefix(cfg.Addr, "127.0.0.1:"))
	cfg.MultiStatements, cfg.ParseTime, cfg.Loc = true, true, time.UTC
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	ctx := t.Context()
	var tables int
	require.NoError(t, pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE()").Scan(&tables))
	require.Zero(t, tables)
	require.NoError(t, db.AutoMigrate(&evaluation.AssessmentPO{}, &evaluation.EvaluationOutcomePO{}, &checkpoint.RuntimeCheckpointPO{}))
	schema, err := os.ReadFile("../../../pkg/migration/migrations/mysql/000084_standard_reliable_outbox.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(schema)).Error)

	assessment, run, data, now := retryFixture()
	run.StartedAt = now.Add(-time.Minute)
	require.NoError(t, db.Create(&assessment).Error)
	require.NoError(t, db.Create(&run).Error)
	event := domainevent.NewRetryRequestedEvent(domainevent.RequestedInput{
		OrgID: data.OrgID, AssessmentID: data.AssessmentID, TesteeID: data.TesteeID, EventID: *run.RetryEventID,
		QuestionnaireCode: data.QuestionnaireCode, QuestionnaireVer: data.QuestionnaireVer, AnswerSheetID: data.AnswerSheetID,
		ModelKind: data.ModelKind, ModelAlgorithm: data.ModelAlgorithm, ModelCode: data.ModelCode, ModelVersion: data.ModelVersion,
		RequestedAt: now, ExpectedAttempt: data.ExpectedAttempt, AttemptOrigin: data.AttemptOrigin, Mode: data.Mode,
	})
	wire, err := eventmessage.EncodeWire(event, "qs-apiserver")
	require.NoError(t, err)
	msg, err := message.New(message.Input{Producer: "qs-server", ID: event.EventID(), Destination: "qs.evaluation.lifecycle",
		EventType: EvaluationRetry, SchemaVersion: "v1", Scope: "org:3", ContentType: "application/json", OccurredAt: now.Format(time.RFC3339Nano), Payload: wire})
	require.NoError(t, err)
	in := msg.Input()
	fingerprint := msg.Fingerprint()
	require.NoError(t, db.Exec(`INSERT INTO rm_outbox
		(producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,next_attempt_at,version,attempt_count,transport_confirmed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,'published',?,2,1,?)`, in.Producer, in.ID, in.Destination, in.EventType, in.SchemaVersion, in.Scope,
		in.ContentType, in.OccurredAt, in.Payload, fingerprint[:], now.UTC(), now.UTC()).Error)
	req := Request{EventID: in.ID, AssessmentID: assessment.ID.Uint64(), OrgID: uint64(assessment.OrgID), AcceptedBefore: now}
	reader := Reader{SQL: db}
	before, err := reader.Capture(ctx, req, now)
	require.NoError(t, err)
	require.Equal(t, fingerprint, before.Message().Fingerprint())
	for _, statement := range []string{
		"CREATE TABLE isolated_effect_archive LIKE rm_outbox",
		"INSERT INTO isolated_effect_archive SELECT * FROM rm_outbox",
		"CREATE SQL SECURITY INVOKER VIEW qs_rm_outbox_history AS SELECT * FROM isolated_effect_archive",
		"RENAME TABLE rm_outbox TO isolated_original_rm_outbox",
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	after, err := reader.Capture(ctx, req, now)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, wire, after.Message().Input().Payload)

	require.NoError(t, db.Exec("UPDATE isolated_effect_archive SET payload=CONCAT(payload,' ') WHERE message_id=?", in.ID).Error)
	_, err = reader.Capture(ctx, req, now)
	require.ErrorIs(t, err, ErrUnsafeSource)
	var original []byte
	require.NoError(t, pool.QueryRowContext(ctx, "SELECT payload FROM isolated_original_rm_outbox WHERE message_id=?", in.ID).Scan(&original))
	require.Equal(t, wire, original)
}
