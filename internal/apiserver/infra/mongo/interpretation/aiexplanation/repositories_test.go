package aiexplanation

import (
	"errors"
	"testing"
	"time"

	appsubjectexport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/subjectexport"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"go.mongodb.org/mongo-driver/bson"
)

func TestParticipantSubjectExportQueryIsScopedAndKeysetPaginated(t *testing.T) {
	snapshot := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	after := snapshot.Add(-time.Hour)
	filter, findOptions, err := participantSubjectExportQuery(appsubjectexport.ReadQuery{
		Subject:    appsubjectexport.Subject{OrgID: 12, TesteeID: meta.FromUint64(34)},
		SnapshotAt: snapshot, AfterGeneratedAt: after, AfterArtifactID: meta.FromUint64(56), Limit: 51,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filter["source.association.org_id"] != int64(12) || filter["source.association.testee_id"] != uint64(34) || filter["audience"] != "participant" {
		t.Fatalf("subject export scope = %#v", filter)
	}
	generated, ok := filter["generated_at"].(bson.M)
	if !ok || generated["$lte"] != snapshot {
		t.Fatalf("subject export snapshot = %#v", filter["generated_at"])
	}
	keyset, ok := filter["$or"].(bson.A)
	if !ok || len(keyset) != 2 {
		t.Fatalf("subject export keyset = %#v", filter["$or"])
	}
	if findOptions.Limit == nil || *findOptions.Limit != 51 {
		t.Fatalf("subject export limit = %#v", findOptions.Limit)
	}
	sort, ok := findOptions.Sort.(bson.D)
	if !ok || len(sort) != 2 || sort[0].Key != "generated_at" || sort[0].Value != -1 || sort[1].Key != "domain_id" || sort[1].Value != -1 {
		t.Fatalf("subject export sort = %#v", findOptions.Sort)
	}

	if _, _, err := participantSubjectExportQuery(appsubjectexport.ReadQuery{
		Subject: appsubjectexport.Subject{OrgID: 12, TesteeID: meta.FromUint64(34)}, SnapshotAt: snapshot,
		AfterGeneratedAt: after, Limit: 51,
	}); err == nil {
		t.Fatal("expected incomplete keyset cursor rejection")
	}
}

func TestExpiredPreparedEvaluationFilterNeverSelectsDispatching(t *testing.T) {
	at := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	filter := expiredPreparedEvaluationFilter(at)
	if filter["status"] != string(domainevaluation.StatusCollecting) {
		t.Fatalf("status filter = %#v", filter["status"])
	}
	if filter["execution.phase"] != string(domainevaluation.AttemptExecutionPrepared) {
		t.Fatalf("phase filter = %#v", filter["execution.phase"])
	}
	lease, ok := filter["execution.lease_expires_at"].(bson.M)
	if !ok || lease["$lte"] != at {
		t.Fatalf("lease filter = %#v", filter["execution.lease_expires_at"])
	}
	legacy, ok := filter["evidence_version"].(bson.M)
	if !ok || legacy["$exists"] != false {
		t.Fatalf("v1 expired preparation filter can select v2 evidence: %#v", filter)
	}
}

func TestExpiredPreparedEvaluationV2FilterSelectsOnlyV2PreparedCheckpoints(t *testing.T) {
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	filter := expiredPreparedEvaluationV2Filter(at)
	if filter["evidence_version"] != PromptEvaluationEvidenceVersionV2 ||
		filter["status"] != string(domainevaluation.EvidenceStatusCollecting) ||
		filter["execution.phase"] != string(domainevaluation.AttemptExecutionPrepared) {
		t.Fatalf("v2 expired preparation filter = %#v", filter)
	}
	lease, ok := filter["execution.lease_expires_at"].(bson.M)
	if !ok || lease["$lte"] != at {
		t.Fatalf("v2 lease filter = %#v", filter["execution.lease_expires_at"])
	}
}

func TestLegacyPromptEvaluationFilterCannotReadV2Evidence(t *testing.T) {
	original := bson.M{"domain_id": meta.ID(91)}
	filter := legacyPromptEvaluationFilter(original)
	legacy, ok := filter["evidence_version"].(bson.M)
	if !ok || legacy["$exists"] != false || filter["domain_id"] != meta.ID(91) {
		t.Fatalf("legacy Prompt evaluation filter = %#v", filter)
	}
	if _, changed := original["evidence_version"]; changed {
		t.Fatalf("legacy filter mutated caller input: %#v", original)
	}
}

func TestDailyBudgetReservationUsesAtomicCeilingAndAuditAppend(t *testing.T) {
	reservedAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	reservation := domainevaluation.DailyCapacityReservation{
		RunID: meta.ID(901), OrgID: 12, RequestedBy: " user:42 ",
		BudgetDay: time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC), ProviderInvocations: 70, DailyLimit: 140,
		ReservedAt: reservedAt,
	}
	filter := dailyBudgetReservationFilter(reservation)
	remaining, ok := filter["reserved_provider_invocations"].(bson.M)
	if !ok || remaining["$lte"] != 70 || filter["org_id"] != int64(12) || filter["budget_day"] != reservation.BudgetDay {
		t.Fatalf("daily budget filter = %#v", filter)
	}
	duplicate, ok := filter["reservations.run_id"].(bson.M)
	if !ok || duplicate["$ne"] != reservation.RunID {
		t.Fatalf("run reservation guard = %#v", filter["reservations.run_id"])
	}

	update := dailyBudgetReservationUpdate(reservation)
	inc, ok := update["$inc"].(bson.M)
	if !ok || inc["reserved_provider_invocations"] != 70 {
		t.Fatalf("daily budget increment = %#v", update["$inc"])
	}
	push, ok := update["$push"].(bson.M)
	if !ok {
		t.Fatalf("daily budget audit append = %#v", update["$push"])
	}
	audit, ok := push["reservations"].(PromptEvaluationBudgetReservationPO)
	if !ok || audit.RunID != reservation.RunID || audit.RequestedBy != "user:42" || !audit.ReservedAt.Equal(reservedAt.UTC()) {
		t.Fatalf("daily budget audit = %#v", push["reservations"])
	}
}

func TestActiveOrganizationDuplicateClassificationIsIndexSpecific(t *testing.T) {
	activeOrg := errors.New("E11000 duplicate key index: uk_ai_explanation_prompt_evaluation_active_org_execution")
	if !isActiveOrgExecutionDuplicate(activeOrg) {
		t.Fatal("expected active organization index duplicate")
	}
	activeRelease := errors.New("E11000 duplicate key index: uk_ai_explanation_prompt_evaluation_active_release")
	if isActiveOrgExecutionDuplicate(activeRelease) || isActiveOrgExecutionDuplicate(nil) {
		t.Fatal("non-organization duplicate was misclassified")
	}
}

func TestDailyCapacityUsageMapperRejectsLedgerDrift(t *testing.T) {
	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	po := &PromptEvaluationDailyBudgetPO{
		OrgID: 12, BudgetDay: day, ReservedProviderInvocations: 70,
		Reservations: []PromptEvaluationBudgetReservationPO{{
			RunID: meta.ID(901), RequestedBy: " user:42 ", ProviderInvocations: 70, ReservedAt: day.Add(time.Hour),
		}},
	}
	usage, err := dailyCapacityUsageFromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if usage.OrgID != 12 || usage.ReservedProviderInvocations != 70 || len(usage.Reservations) != 1 || usage.Reservations[0].RequestedBy != "user:42" {
		t.Fatalf("capacity usage = %#v", usage)
	}

	po.ReservedProviderInvocations = 140
	if _, err := dailyCapacityUsageFromPO(po); err == nil {
		t.Fatal("expected inconsistent persisted total to be rejected")
	}
}

func TestParticipantDailyBudgetReservationUsesAllAtomicCeilings(t *testing.T) {
	reservedAt := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	reservation := domaingeneration.ParticipantDailyCapacityReservation{
		ReservationID: domaingeneration.ParticipantCapacityReservationID(meta.ID(902), 1),
		GenerationID:  meta.ID(902), Attempt: 1, Origin: retrygovernance.AttemptOriginInitial,
		OrgID: 12, UserID: " user-42 ", AssessmentID: meta.ID(501),
		BudgetDay: reservedAt.Truncate(24 * time.Hour), ProviderInvocations: 1,
		Policy: domaingeneration.ParticipantCapacityPolicy{
			DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
			DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
			MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
		},
		ReservedAt: reservedAt,
	}
	filter := participantDailyBudgetReservationFilter(reservation)
	orgCeiling, ok := filter["reserved_provider_invocations"].(bson.M)
	if !ok || orgCeiling["$lte"] != 499 {
		t.Fatalf("participant organization ceiling = %#v", filter["reserved_provider_invocations"])
	}
	duplicate, ok := filter["reservations.reservation_id"].(bson.M)
	if !ok || duplicate["$ne"] != reservation.ReservationID {
		t.Fatalf("participant duplicate guard = %#v", filter["reservations.reservation_id"])
	}
	expression, ok := filter["$expr"].(bson.M)
	if !ok {
		t.Fatalf("participant dimension ceilings = %#v", filter["$expr"])
	}
	ceilings, ok := expression["$and"].(bson.A)
	if !ok || len(ceilings) != 2 {
		t.Fatalf("participant user/Assessment ceilings = %#v", expression["$and"])
	}

	update := participantDailyBudgetReservationUpdate(reservation)
	push := update["$push"].(bson.M)
	audit, ok := push["reservations"].(ParticipantBudgetReservationPO)
	if !ok || audit.ReservationID != reservation.ReservationID || audit.GenerationID != reservation.GenerationID ||
		audit.Attempt != 1 || audit.Origin != string(retrygovernance.AttemptOriginInitial) || audit.UserID != "user-42" ||
		audit.AssessmentID != reservation.AssessmentID || audit.ProviderInvocations != 1 || !audit.ReservedAt.Equal(reservedAt) {
		t.Fatalf("participant budget audit = %#v", push["reservations"])
	}
}

func TestParticipantDailyCapacityUsageMapperRejectsLedgerDrift(t *testing.T) {
	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	po := &ParticipantDailyBudgetPO{
		OrgID: 12, BudgetDay: day, ReservedProviderInvocations: 1,
		Reservations: []ParticipantBudgetReservationPO{{
			ReservationID: domaingeneration.ParticipantCapacityReservationID(meta.ID(902), 1),
			GenerationID:  meta.ID(902), Attempt: 1, Origin: string(retrygovernance.AttemptOriginInitial),
			UserID: " user-42 ", AssessmentID: meta.ID(501),
			ProviderInvocations: 1, ReservedAt: day.Add(time.Hour),
		}},
	}
	usage, err := participantDailyCapacityUsageFromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if usage.OrgID != 12 || usage.ReservedProviderInvocations != 1 || len(usage.Reservations) != 1 ||
		usage.Reservations[0].UserID != "user-42" || usage.Reservations[0].AssessmentID != meta.ID(501) {
		t.Fatalf("participant capacity usage = %#v", usage)
	}

	po.ReservedProviderInvocations = 2
	if _, err := participantDailyCapacityUsageFromPO(po); err == nil {
		t.Fatal("expected inconsistent participant persisted total to be rejected")
	}
}

func TestParticipantActiveSlotUsesAllAtomicCeilings(t *testing.T) {
	at := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	slot := domaingeneration.ParticipantActiveSlot{
		GenerationID: meta.ID(902), RunID: meta.ID(903), OrgID: 12, UserID: " user-42 ", AssessmentID: meta.ID(501),
		Policy: domaingeneration.ParticipantCapacityPolicy{
			DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
			DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
			MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
		},
		AcquiredAt: at,
	}
	filter := participantActiveSlotFilter(slot)
	orgCeiling, ok := filter["active_executions"].(bson.M)
	if !ok || orgCeiling["$lte"] != 9 {
		t.Fatalf("participant active organization ceiling = %#v", filter["active_executions"])
	}
	expression, ok := filter["$expr"].(bson.M)
	if !ok {
		t.Fatalf("participant active dimension ceilings = %#v", filter["$expr"])
	}
	ceilings, ok := expression["$and"].(bson.A)
	if !ok || len(ceilings) != 2 {
		t.Fatalf("participant active user/Assessment ceilings = %#v", expression["$and"])
	}

	update := participantActiveSlotAcquireUpdate(slot)
	push := update["$push"].(bson.M)
	audit, ok := push["reservations"].(ParticipantActiveCapacityReservationPO)
	if !ok || audit.GenerationID != slot.GenerationID || audit.RunID != slot.RunID || audit.UserID != "user-42" ||
		audit.AssessmentID != slot.AssessmentID || !audit.AcquiredAt.Equal(at) {
		t.Fatalf("participant active audit = %#v", push["reservations"])
	}
}

func TestParticipantActiveCapacityUsageMapperRejectsLedgerDrift(t *testing.T) {
	at := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	po := &ParticipantActiveCapacityPO{
		OrgID: 12, ActiveExecutions: 1,
		Reservations: []ParticipantActiveCapacityReservationPO{{
			GenerationID: meta.ID(902), RunID: meta.ID(903), UserID: " user-42 ",
			AssessmentID: meta.ID(501), AcquiredAt: at,
		}},
	}
	usage, err := participantActiveCapacityUsageFromPO(po)
	if err != nil {
		t.Fatal(err)
	}
	if usage.OrgID != 12 || usage.ActiveExecutions != 1 || len(usage.Reservations) != 1 ||
		usage.Reservations[0].RunID != meta.ID(903) || usage.Reservations[0].UserID != "user-42" {
		t.Fatalf("participant active capacity usage = %#v", usage)
	}
	po.ActiveExecutions = 2
	if _, err := participantActiveCapacityUsageFromPO(po); err == nil {
		t.Fatal("expected inconsistent participant active total to be rejected")
	}
}
