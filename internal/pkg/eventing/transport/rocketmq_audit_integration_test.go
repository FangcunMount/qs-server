//go:build integration

package transport

import (
	"context"
	"os"
	"strings"
	"testing"

	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

func TestRocketMQAuditRequiresOriginalEvidenceAndPreservesItOnRollback(t *testing.T) {
	if os.Getenv("MYSQL_DSN") == "" || os.Getenv("QS_RMQ_AUDIT_UP") == "" || os.Getenv("QS_RMQ_AUDIT_DOWN") == "" {
		t.Fatal("disposable MySQL and tracked migrations are required; no skip")
	}
	db := openIsolatedDeadLetterDatabase(t)
	up, err := os.ReadFile(os.Getenv("QS_RMQ_AUDIT_UP"))
	require.NoError(t, err)
	down, err := os.ReadFile(os.Getenv("QS_RMQ_AUDIT_DOWN"))
	require.NoError(t, err)
	_, err = db.Exec(string(up))
	require.NoError(t, err)
	rollback := func() error {
		conn, err := db.Conn(t.Context())
		if err != nil {
			return err
		}
		defer conn.Close()
		for _, statement := range strings.Split(string(down), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err = conn.ExecContext(t.Context(), statement); err != nil {
				return err
			}
		}
		return nil
	}
	require.NoError(t, rollback(), "empty evidence table can be rolled back")
	_, err = db.Exec(string(up))
	require.NoError(t, err)
	recorder, err := NewSQLDeadLetterRecorder(db)
	require.NoError(t, err)
	f := rmtransport.Failure{Provider: "rocketmq", Cause: "delivery budget exhausted", Message: rmtransport.Received{
		Provider: "rocketmq", Topic: "qs.evaluation.lifecycle", Channel: "qs-worker", PhysicalTopic: rmtransport.PhysicalTopic("qs.evaluation.lifecycle"),
		ID: "original-id", TransportID: "broker-id", Attempts: 8, Wire: []byte("original exact envelope"), Payload: []byte(`{"id":"original-id","data":{"org_id":7}}`), Metadata: map[string]string{"event_type": "known.fail"},
	}}
	handle := SDKRocketMQFailureHandler(recorder)
	require.NoError(t, handle(t.Context(), f))
	require.NoError(t, handle(t.Context(), f))
	var count int
	var wire, application string
	require.NoError(t, db.QueryRow("SELECT COUNT(*),MAX(original_wire),MAX(application_id) FROM event_delivery_transport_evidence").Scan(&count, &wire, &application))
	require.Equal(t, 1, count)
	require.Equal(t, string(f.Message.Wire), wire)
	require.Equal(t, f.Message.ID, application)
	f.Message.Wire = []byte("changed envelope")
	require.Error(t, handle(t.Context(), f), "conflicting wire cannot authorize ACK")
	require.Error(t, rollback(), "rollback preserves nonempty audit evidence")
	f.Message.ID = ""
	f.Message.TransportID = "malformed-broker-id"
	f.Message.Wire = []byte{0xff, 0x00, 0x80}
	f.Message.Payload = f.Message.Wire
	require.NoError(t, handle(t.Context(), f), "malformed bytes remain recoverable as bytes")
	require.NoError(t, db.QueryRow("SELECT application_id FROM event_delivery_transport_evidence WHERE transport_message_id='malformed-broker-id'").Scan(&application))
	require.Empty(t, application)
	_, err = db.Exec("DROP TABLE event_delivery_dead_letter")
	require.NoError(t, err)
	f.Message.TransportID = "new-uncommitted-failure"
	require.Error(t, handle(context.Background(), f))
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM event_delivery_transport_evidence WHERE transport_message_id='new-uncommitted-failure'").Scan(&count))
	require.Zero(t, count, "evidence and replay audit must commit atomically")
}
