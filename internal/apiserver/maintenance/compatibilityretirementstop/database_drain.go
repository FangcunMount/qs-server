package compatibilityretirementstop

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var ErrDatabaseDrain = errors.New("retirement_stop_database_drain_unproven")

// DrainObservation contains two actual global transaction/operation reads. It
// cannot prove that a currently idle external writer cannot write in the future.
// The caller must separately hold a live writer fence; no JSON boolean is used.
type DrainObservation struct {
	self            *DrainObservation
	first, second   time.Time
	sqlConnectionID uint64
	samples         int
}

func (*DrainObservation) MarshalJSON() ([]byte, error) { return nil, ErrDatabaseDrain }
func (*DrainObservation) String() string {
	return "opaque two-sample global database drain observation; no future writer fence"
}

type DrainSummary struct {
	Samples                int       `json:"samples"`
	FirstAt                time.Time `json:"first_at"`
	SecondAt               time.Time `json:"second_at"`
	WholeWriterFenceProven bool      `json:"whole_writer_fence_proven"`
}

func (v *DrainObservation) Summary() DrainSummary {
	if v == nil || v.self != v {
		return DrainSummary{}
	}
	return DrainSummary{Samples: v.samples, FirstAt: v.first, SecondAt: v.second}
}
func hasGlobalProcess(ctx context.Context, c *sql.Conn) error {
	rows, e := c.QueryContext(ctx, "SHOW GRANTS FOR CURRENT_USER")
	if e != nil {
		return ErrDatabaseDrain
	}
	proven := false
	for rows.Next() {
		var grant string
		if rows.Scan(&grant) != nil {
			_ = rows.Close()
			return ErrDatabaseDrain
		}
		g := strings.ToUpper(grant)
		pieces := strings.SplitN(g, " ON *.* ", 2)
		if len(pieces) != 2 {
			continue
		}
		p := strings.TrimPrefix(pieces[0], "GRANT ")
		for _, priv := range strings.Split(p, ",") {
			priv = strings.TrimSpace(priv)
			if priv == "PROCESS" || priv == "ALL PRIVILEGES" {
				proven = true
			}
		}
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !proven {
		return ErrDatabaseDrain
	}
	return nil
}
func sqlDrained(ctx context.Context, c *sql.Conn) (uint64, error) {
	if c == nil || hasGlobalProcess(ctx, c) != nil {
		return 0, ErrDatabaseDrain
	}
	var id uint64
	var autocommit, count uint64
	if c.QueryRowContext(ctx, "SELECT CONNECTION_ID(), @@autocommit").Scan(&id, &autocommit) != nil || id == 0 || autocommit != 1 {
		return 0, ErrDatabaseDrain
	}
	// PROCESS is checked first; a user-limited empty view is never sufficient.
	if c.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.innodb_trx").Scan(&count) != nil || count != 0 {
		return 0, ErrDatabaseDrain
	}
	rows, e := c.QueryContext(ctx, "SELECT ID FROM information_schema.PROCESSLIST WHERE ID <> CONNECTION_ID() AND COMMAND <> 'Sleep'")
	if e != nil {
		return 0, ErrDatabaseDrain
	}
	active := rows.Next()
	re, ce := rows.Err(), rows.Close()
	if active || re != nil || ce != nil {
		return 0, ErrDatabaseDrain
	}
	return id, nil
}
func readOnlyCommand(cmd bson.M) bool {
	mutations := map[string]bool{"insert": true, "update": true, "delete": true, "findandmodify": true, "bulkwrite": true, "create": true, "drop": true, "dropdatabase": true, "renamecollection": true, "applyops": true, "committransaction": true, "aborttransaction": true, "preparetransaction": true, "mapreduce": true, "eval": true, "createindexes": true, "dropindexes": true, "collmod": true, "convertToCapped": true, "replsetreconfig": true, "shutdown": true}
	recognized := 0
	for key := range cmd {
		lower := strings.ToLower(key)
		if mutations[lower] {
			return false
		}
		switch lower {
		case "find", "getmore", "listcollections", "listindexes", "hello", "ismaster", "ping", "currentop", "connectionstatus", "buildinfo", "serverstatus", "replsetgetstatus", "killcursors", "aggregate":
			recognized++
		}
	}
	if recognized != 1 {
		return false
	}
	for key := range cmd {
		switch strings.ToLower(key) {
		case "find", "getmore", "listcollections", "listindexes", "hello", "ismaster", "ping", "currentop", "connectionstatus", "buildinfo", "serverstatus", "replsetgetstatus", "killcursors":
			return true
		case "aggregate":
			raw, ok := cmd["pipeline"].(bson.A)
			if !ok {
				return false
			}
			allowed := map[string]bool{"$currentOp": true, "$match": true, "$project": true, "$limit": true, "$sort": true, "$group": true, "$count": true, "$skip": true, "$unwind": true, "$addFields": true, "$set": true, "$unset": true, "$replaceRoot": true, "$replaceWith": true, "$facet": true, "$lookup": true, "$graphLookup": true, "$unionWith": true, "$sample": true, "$bucket": true, "$bucketAuto": true, "$sortByCount": true, "$setWindowFields": true, "$densify": true, "$fill": true, "$redact": true}
			for _, rawStage := range raw {
				stage, ok := rawStage.(bson.M)
				if !ok || len(stage) != 1 {
					return false
				}
				for name := range stage {
					if !allowed[name] {
						return false
					}
				}
			}
			// Nested pipelines can contain $out/$merge; inspect all keys recursively.
			return !containsWriteStage(raw)
		}
	}
	return false
}
func containsWriteStage(v any) bool {
	switch x := v.(type) {
	case bson.M:
		for k, v := range x {
			if k == "$out" || k == "$merge" {
				return true
			}
			if containsWriteStage(v) {
				return true
			}
		}
	case bson.A:
		for _, v := range x {
			if containsWriteStage(v) {
				return true
			}
		}
	case bson.D:
		for _, v := range x {
			if v.Key == "$out" || v.Key == "$merge" || containsWriteStage(v.Value) {
				return true
			}
		}
	}
	return false
}
func mongoDrained(ctx context.Context, db *mongo.Database) error {
	if db == nil || mongo.SessionFromContext(ctx) != nil {
		return ErrDatabaseDrain
	}
	admin := db.Client().Database("admin")
	// allUsers requires actual inprog privilege. An authorization/network error
	// fails; there is no fallback to a user-scoped currentOp view.
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$currentOp", Value: bson.D{{Key: "allUsers", Value: true}, {Key: "idleSessions", Value: true}, {Key: "idleConnections", Value: true}, {Key: "localOps", Value: true}}}},
		bson.D{{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{bson.M{"active": true}, bson.M{"transaction": bson.M{"$exists": true}}}}}}},
	}
	cur, e := admin.Aggregate(ctx, pipeline)
	if e != nil {
		return ErrDatabaseDrain
	}
	count := 0
	blocked := false
	for cur.Next(ctx) {
		count++
		if count > 10000 {
			blocked = true
			break
		}
		var doc bson.M
		if cur.Decode(&doc) != nil {
			blocked = true
			break
		}
		if _, ok := doc["transaction"]; ok {
			blocked = true
			break
		}
		op, _ := doc["op"].(string)
		if op == "none" {
			continue
		}
		cmd, ok := doc["command"].(bson.M)
		if !ok || !readOnlyCommand(cmd) {
			blocked = true
			break
		}
	}
	re, ce := cur.Err(), cur.Close(ctx)
	if blocked || re != nil || ce != nil || ctx.Err() != nil {
		return ErrDatabaseDrain
	}
	return nil
}

// ObserveDatabaseDrain only borrows dedicated autocommit SQL and nontransactional
// Mongo handles. It opens/closes only its own rows/cursor, never either pool,
// connection, session, or host transaction. No data or credentials are logged.
func ObserveDatabaseDrain(ctx context.Context, c *sql.Conn, db *mongo.Database) (*DrainObservation, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrDatabaseDrain
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, ErrDatabaseDrain
	}
	id, e := sqlDrained(ctx, c)
	if e != nil || mongoDrained(ctx, db) != nil {
		return nil, ErrDatabaseDrain
	}
	first := time.Now().UTC()
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ErrDatabaseDrain
	case <-timer.C:
	}
	secondID, e := sqlDrained(ctx, c)
	if e != nil || secondID != id || mongoDrained(ctx, db) != nil {
		return nil, ErrDatabaseDrain
	}
	out := &DrainObservation{first: first, second: time.Now().UTC(), sqlConnectionID: id, samples: 2}
	out.self = out
	return out, nil
}
