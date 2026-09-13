package aibridge

import (
	"context"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"time"
)

func (c *EvaluationClient) GetEvaluationCapacity(ctx context.Context, scope app.DraftScope) (app.EvaluationCapacity, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, err := c.RPC.GetCapacity(ctx, draftScope(scope), grpc.MaxCallRecvMsgSize(128*1024))
	if err != nil {
		return app.EvaluationCapacity{}, err
	}
	if r == nil || r.OrganizationId != scope.OrganizationID || r.DailyProviderCalls < 1 ||
		r.ReservedProviderCalls < 0 || r.RemainingProviderCalls < 0 || r.FullRunProviderCalls < 1 ||
		r.RemainingFullRuns < 0 || r.MaxActiveRuns < 1 || r.ActiveRuns < 0 || r.ReservationCount < 0 ||
		len(r.Reservations) > 100 || r.ReservationCount < int64(len(r.Reservations)) ||
		r.ReservationsTruncated != (r.ReservationCount > int64(len(r.Reservations))) {
		return app.EvaluationCapacity{}, app.ErrConflict
	}
	if day, e := time.Parse("2006-01-02", r.BudgetDay); e != nil || day.Format("2006-01-02") != r.BudgetDay {
		return app.EvaluationCapacity{}, app.ErrConflict
	}
	// AI owns budget policy and admission. QS validates only identity and wire bounds.
	receipts := make([]app.EvaluationCapacityReservation, 0, len(r.Reservations))
	seen := map[string]bool{}
	for _, item := range r.Reservations {
		if item == nil {
			return app.EvaluationCapacity{}, app.ErrConflict
		}
		id, idErr := uuid.Parse(item.RunId)
		at, atErr := time.Parse(time.RFC3339Nano, item.ReservedAt)
		if idErr != nil || id == uuid.Nil || id.String() != item.RunId || seen[item.RunId] ||
			atErr != nil || at.UTC().Format("2006-01-02") != r.BudgetDay || item.ProviderCalls < 1 || !creationActor.MatchString(item.RequestedBy) {
			return app.EvaluationCapacity{}, app.ErrConflict
		}
		seen[item.RunId] = true
		receipts = append(receipts, app.EvaluationCapacityReservation{RunID: item.RunId, ProviderCalls: item.ProviderCalls, RequestedBy: item.RequestedBy, ReservedAt: item.ReservedAt})
	}
	return app.EvaluationCapacity{OrganizationID: r.OrganizationId, BudgetDay: r.BudgetDay,
		DailyProviderCalls: r.DailyProviderCalls, ReservedProviderCalls: r.ReservedProviderCalls,
		RemainingProviderCalls: r.RemainingProviderCalls, FullRunProviderCalls: r.FullRunProviderCalls,
		RemainingFullRuns: r.RemainingFullRuns, MaxActiveRuns: r.MaxActiveRuns, ActiveRuns: r.ActiveRuns,
		ReservationCount: r.ReservationCount, Reservations: receipts, ReservationsTruncated: r.ReservationsTruncated}, nil
}
