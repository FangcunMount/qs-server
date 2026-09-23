//go:build reliable_messaging_m4

package process

import (
	"context"
	"fmt"

	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"go.mongodb.org/mongo-driver/mongo"
)

// The host migrations must already exist before any M4 writer is selected.
// This is a shape/index preflight, not a substitute for M4-07 query-plan and
// performance evidence or the M5 old-intent cutover audit.
func preflightM4StandardStorage(ctx context.Context, opts eventsubsystem.Options, selected options.StandardOutboxOptions) error {
	if !selected.Mongo && !selected.Assessment {
		return nil
	}
	if opts.MySQLDB == nil {
		return fmt.Errorf("M4 standard replay requires a MySQL governance audit database")
	}
	auditDB, err := opts.MySQLDB.DB()
	if err != nil {
		return fmt.Errorf("resolve M4 governance audit pool: %w", err)
	}
	auditRows, err := auditDB.QueryContext(ctx, `SELECT org_id,request_id,action_id,actor_user_id,component,target_instance,
input_json,status,result_json,started_at,finished_at FROM system_governance_action_runs LIMIT 0`)
	if err != nil {
		return fmt.Errorf("M4 governance audit schema is incomplete: %w", err)
	}
	if err := auditRows.Close(); err != nil {
		return fmt.Errorf("close M4 governance audit preflight query: %w", err)
	}
	var auditKeyCount int
	if err := auditDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema=DATABASE() AND table_name='system_governance_action_runs'
AND index_name='uk_system_governance_action_runs_org_request' AND non_unique=0`).Scan(&auditKeyCount); err != nil {
		return fmt.Errorf("inspect M4 governance audit request key: %w", err)
	}
	if auditKeyCount != 2 {
		return fmt.Errorf("M4 governance audit requires the unique org/request key; found %d/2 columns", auditKeyCount)
	}
	if selected.Assessment {
		db := auditDB
		for _, query := range []string{
			"SELECT state,next_attempt_at,lease_until,version,attempt_count,failure_count,created_at,updated_at,manual_replay_request_id,manual_replay_version FROM rm_outbox LIMIT 0",
			"SELECT org_id,request_id,store_name,reason,input_hash FROM qs_rm_replay_requests LIMIT 0",
			"SELECT org_id,request_id,ordinal,event_id,expected_failure_count,authorized,reason FROM qs_rm_replay_items LIMIT 0",
		} {
			rows, err := db.QueryContext(ctx, query)
			if err != nil {
				return fmt.Errorf("M4 MySQL standard schema is incomplete: %w", err)
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("close M4 MySQL preflight query: %w", err)
			}
		}
		var indexCount int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics
WHERE table_schema=DATABASE() AND table_name='rm_outbox'
AND index_name IN ('due_idx','lease_idx','ix_rm_outbox_message_id','ix_rm_outbox_scope_governance')`).Scan(&indexCount); err != nil {
			return fmt.Errorf("inspect M4 MySQL standard indexes: %w", err)
		}
		if indexCount != 4 {
			return fmt.Errorf("M4 MySQL standard outbox requires due, lease, message and governance indexes; found %d/4", indexCount)
		}
	}
	if selected.Mongo {
		db := opts.MongoDB
		for _, check := range []struct {
			collection string
			indexes    []string
		}{
			{"rm_outbox", []string{"ix_rm_outbox_due", "ix_rm_outbox_lease", "ix_rm_outbox_message_id", "ix_rm_outbox_scope_governance"}},
			{"qs_rm_replay_requests", []string{"ix_qs_rm_replay_requests_org_time"}},
		} {
			if err := requireM4MongoIndexes(ctx, db.Collection(check.collection), check.indexes); err != nil {
				return fmt.Errorf("M4 Mongo standard collection %s: %w", check.collection, err)
			}
		}
	}
	return nil
}

func requireM4MongoIndexes(ctx context.Context, collection *mongo.Collection, required []string) error {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	found := make(map[string]bool, len(required))
	for cursor.Next(ctx) {
		var row struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		found[row.Name] = true
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	for _, name := range required {
		if !found[name] {
			return fmt.Errorf("required index %q is missing", name)
		}
	}
	return nil
}
