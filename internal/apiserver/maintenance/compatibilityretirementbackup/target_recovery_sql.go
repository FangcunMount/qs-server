package compatibilityretirementbackup

import (
	"context"
	"strconv"
	"strings"
)

// Shared native producer, deliberately package-private. A future fixed host
// executor must supply independent approval/fencing BEFORE calling it. JSON or
// an absence observation alone cannot reproduce the returned private proof.
//
//nolint:unused // Native recovery tests exercise this producer until the reviewed host executor is wired.
func executeTargetDrop(ctx context.Context, p *TargetRecoveryPlan, i int) error {
	if p == nil || p.self != p || i < 0 || i >= 4 {
		return ErrRecoveryBinding
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.drop[i] != nil || p.observations[i].State != "existing_exact" {
		return ErrRecoveryState
	}
	if e := p.checkBases(ctx, true); e != nil {
		return e
	}
	present, e := p.checkTarget(ctx, i)
	if e != nil || !present {
		return ErrRecoveryState
	}
	statement := "DROP TABLE " + quote(targetNames[i])
	if i == 3 {
		statement = "drop collection domain_event_outbox"
	}
	name := strconv.Itoa(i) + "-drop"
	intent, e := p.journal.write(name+"-intent", targetRecord(p, i, "drop", statement, "intent", 0))
	if e != nil {
		p.blocked = true
		return e
	}
	if i < 3 {
		_, e = p.borrowed.SQL.ExecContext(ctx, statement)
	} else {
		e = p.borrowed.Mongo.Collection(targetNames[3]).Drop(ctx)
	}
	if e != nil {
		_, _ = p.journal.write(name+"-result", targetRecord(p, i, "drop", statement, "unknown", 0))
		p.blocked = true
		return ErrRecoveryUnknown
	}
	present, e = p.checkTarget(ctx, i)
	if e != nil || present {
		_, _ = p.journal.write(name+"-result", targetRecord(p, i, "drop", statement, "unknown", 0))
		p.blocked = true
		return ErrRecoveryUnknown
	}
	result, e := p.journal.write(name+"-result", targetRecord(p, i, "drop", statement, "native_success_and_absent", 0))
	if e != nil {
		p.blocked = true
		return e
	}
	d := &targetDropProof{plan: p, index: i, intentHash: intent, resultHash: result, nativeAbsent: true}
	d.self = d
	p.drop[i] = d
	p.observations[i].State = "dropped_native"
	if e = p.checkBases(ctx, true); e != nil {
		p.blocked = true
		return e
	}
	return nil
}
func (p *TargetRecoveryPlan) recoverSQL(ctx context.Context, i int) error {
	if e := p.checkBases(ctx, true); e != nil {
		return e
	}
	ddl := p.archive.data.SQL[i].DDL
	if !strings.HasPrefix(ddl, "CREATE TABLE "+quote(targetNames[i])+" (") {
		return ErrStructure
	}
	name := strconv.Itoa(i)
	if _, e := p.journal.write(name+"-create-intent", targetRecord(p, i, "create", ddl, "intent", 0)); e != nil {
		return e
	}
	if _, e := p.borrowed.SQL.ExecContext(ctx, ddl); e != nil {
		_, _ = p.journal.write(name+"-create-result", targetRecord(p, i, "create", ddl, "unknown", 0))
		return ErrRecoveryUnknown
	}
	if _, e := p.journal.write(name+"-create-result", targetRecord(p, i, "create", ddl, "native_success", 0)); e != nil {
		return e
	}
	if _, e := p.journal.write(name+"-load-intent", targetRecord(p, i, "load", "bounded original source INSERT", "intent", 0)); e != nil {
		return e
	}
	metrics, e := loadSQLRows(ctx, p.borrowed.SQL, p.archive, i)
	if e != nil {
		_, _ = p.journal.write(name+"-load-result", targetRecord(p, i, "load", "bounded original source INSERT", "unknown", metrics.RestoredRecords))
		return ErrRecoveryUnknown
	}
	if _, e = p.journal.write(name+"-load-result", targetRecord(p, i, "load", "bounded original source INSERT", "native_success", metrics.RestoredRecords)); e != nil {
		return e
	}
	present, e := p.checkTarget(ctx, i)
	if e != nil || !present {
		return ErrContent
	}
	if _, e = p.journal.write(name+"-verify-result", targetRecord(p, i, "verify", "full original structure and content readback", "equal", metrics.RestoredRecords)); e != nil {
		return e
	}
	p.observations[i].State = "restored_exact"
	return nil
}
