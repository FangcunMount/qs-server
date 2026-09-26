package systemgovernance

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDeliveryResolutionRequiresCompleteEvidenceAndCommitsAtomicallyMySQL(t *testing.T) {
	dsn := os.Getenv("QS_SERVER_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_ACTION_AUDIT_REQUIRE_MYSQL") == "true" {
			t.Fatal("QS_SERVER_TEST_MYSQL_DSN is required")
		}
		t.Skip("requires isolated MySQL")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	for _, path := range []string{
		"../../../../../internal/pkg/migration/migrations/mysql/000048_add_system_governance_action_runs.up.sql",
		"../../../../../internal/pkg/migration/migrations/mysql/000049_add_retry_governance.up.sql",
	} {
		ddl, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		source := string(ddl)
		if strings.Contains(path, "000049") {
			source = source[strings.Index(source, "CREATE TABLE `event_delivery_dead_letter`"):]
		}
		if err := db.Exec(strings.Replace(source, "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)).Error; err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().Add(-10 * time.Minute)
	original := actionRunPO{
		RequestID: "original-1", ActionID: "events.replay_delivery", OrgID: 7,
		ActorUserID: 11, InputJSON: `{"targets":[{"id":41,"expected_delivery_attempts":8}],"reason":"operator replay"}`,
		Status: "failed", ResultJSON: "null", StartedAt: start,
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO event_delivery_dead_letter
		(id,message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,retry_disposition,replay_request_id,failed_at)
		VALUES (41,'message-41','event-41',7,'nsq','topic','channel',8,'{"id":"event-41"}','automatic','original-1',?)`, start).Error; err != nil {
		t.Fatal(err)
	}
	store := NewActionAuditStore(db)
	req := DeliveryResolutionRequest{
		OrgID: 7, ActorUserID: 22, RequestID: "resolution-1", OriginalReplayRequestID: "original-1",
		DeadLetterID: 41, EventID: "event-41", ExpectedDeliveryAttempts: 8, Reason: "verified effects",
	}
	full := func(_ context.Context, _ *gorm.DB, subject DeliveryResolutionSubject) (DeliveryResolutionEvidence, error) {
		if subject.OrgID != 7 || subject.DeadLetterID != 41 || subject.EventID != "event-41" ||
			subject.OriginalRequestID != "original-1" || subject.PayloadJSON != `{"id":"event-41"}` {
			t.Fatalf("wrong locked subject: %+v", subject)
		}
		return DeliveryResolutionEvidence{Kind: "business_fact", Reference: "proof-41", AllEffectsConfirmed: true}, nil
	}
	if err := store.ResolveDelivery(t.Context(), req, nil); err == nil {
		t.Fatal("missing verifier must reject resolution")
	}
	partial := func(context.Context, *gorm.DB, DeliveryResolutionSubject) (DeliveryResolutionEvidence, error) {
		return DeliveryResolutionEvidence{Kind: "attention_projection", Reference: "ledger-41"}, nil
	}
	if err := store.ResolveDelivery(t.Context(), req, partial); err == nil {
		t.Fatal("one projection cannot close the entire delivery")
	}
	wrongOrg := req
	wrongOrg.OrgID = 8
	if err := store.ResolveDelivery(t.Context(), wrongOrg, full); err == nil {
		t.Fatal("another organization must not resolve this delivery")
	}
	wrongAttempts := req
	wrongAttempts.ExpectedDeliveryAttempts++
	if err := store.ResolveDelivery(t.Context(), wrongAttempts, full); err == nil {
		t.Fatal("changed physical delivery attempts must not resolve")
	}
	if err := db.Model(&actionRunPO{}).Where("id = ?", original.ID).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveDelivery(t.Context(), req, full); err == nil {
		t.Fatal("old running audit may still own a live publisher")
	}
	if err := db.Model(&actionRunPO{}).Where("id = ?", original.ID).Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:reject_delivery_resolution", func(tx *gorm.DB) {
		if tx.Statement.Table == "event_delivery_dead_letter" {
			_ = tx.AddError(errors.New("injected settlement failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveDelivery(t.Context(), req, full); err == nil {
		t.Fatal("settlement failure must abort the transaction")
	}
	if err := db.Callback().Update().Remove("test:reject_delivery_resolution"); err != nil {
		t.Fatal(err)
	}
	var auditCount int64
	if err := db.Model(&actionRunPO{}).Where("org_id = ? AND request_id = ?", 7, req.RequestID).Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 0 {
		t.Fatal("failed settlement left a committed resolution audit")
	}
	assertResolutionState(t, db, "automatic", 0)
	if err := store.ResolveDelivery(t.Context(), req, full); err != nil {
		t.Fatal(err)
	}
	assertResolutionState(t, db, deliveryResolutionDisposition, 1)
	var originalAfter actionRunPO
	if err := db.Where("org_id = ? AND request_id = ?", 7, "original-1").Take(&originalAfter).Error; err != nil {
		t.Fatal(err)
	}
	if originalAfter.Status != "failed" || originalAfter.ResultJSON != "null" {
		t.Fatal("resolution rewrote the original failed replay audit")
	}
	verifyAgain := func(context.Context, *gorm.DB, DeliveryResolutionSubject) (DeliveryResolutionEvidence, error) {
		return DeliveryResolutionEvidence{}, errors.New("idempotent retry must not verify or publish again")
	}
	if err := store.ResolveDelivery(t.Context(), req, verifyAgain); err != nil {
		t.Fatalf("same resolution request must be idempotent: %v", err)
	}
	changedInput := req
	changedInput.Reason = "different reason"
	if err := store.ResolveDelivery(t.Context(), changedInput, full); err == nil {
		t.Fatal("same request ID with changed input must conflict")
	}
	second := req
	second.RequestID = "resolution-2"
	if err := store.ResolveDelivery(t.Context(), second, full); err == nil {
		t.Fatal("resolved dead letter must reject a second resolution")
	}
	assertResolutionState(t, db, deliveryResolutionDisposition, 1)
}

func assertResolutionState(t *testing.T, db *gorm.DB, disposition string, auditCount int64) {
	t.Helper()
	var deadLetter deliveryResolutionDeadLetter
	if err := db.Where("id = ?", 41).Take(&deadLetter).Error; err != nil {
		t.Fatal(err)
	}
	if deadLetter.RetryDisposition != disposition || deadLetter.ReplayRequestID == nil || *deadLetter.ReplayRequestID != "original-1" {
		t.Fatalf("dead letter lost original claim: %+v", deadLetter)
	}
	var count int64
	if err := db.Model(&actionRunPO{}).Where("org_id = ? AND action_id = ?", 7, deliveryResolutionActionID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != auditCount {
		t.Fatalf("resolution audit count = %d, want %d", count, auditCount)
	}
}
