package aibridge

import (
	"context"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func evaluationCatalogPage(raw *pb.EvaluationCatalogPage, scope app.DraftScope, query app.EvaluationCatalogQuery) (app.EvaluationCatalogPage, error) {
	page := app.EvaluationCatalogPage{Items: make([]app.EvaluationSummary, 0)}
	if raw == nil || proto.Size(raw) > 256*1024 || len(raw.Items) > query.Limit {
		return page, app.ErrConflict
	}
	previousAt, previousID, valid := query.After(scope.OrganizationID)
	if !valid {
		return page, app.ErrInvalid
	}
	seen := make(map[string]bool)
	for _, row := range raw.Items {
		if row == nil {
			return app.EvaluationCatalogPage{}, app.ErrConflict
		}
		item := app.EvaluationSummary{
			RunID: row.RunId, OrganizationID: row.OrganizationId, Version: row.Version, Status: row.Status,
			CreatedAt: row.CreatedAt, RequestedBy: row.RequestedBy, ProfileID: row.ProfileId, ProfileVersion: row.ProfileVersion,
			PromptID: row.PromptId, PromptVersion: row.PromptVersion, ReleaseFingerprint: row.ReleaseFingerprint,
			UnresolvedResultUnknownCount: row.UnresolvedResultUnknownCount, ReviewCount: row.ReviewCount,
			RequiredCandidates: row.RequiredCandidates, AcceptedCandidates: row.AcceptedCandidates, ReviewReadyCandidates: row.ReviewReadyCandidates,
			LastCause: row.LastCause, LastReason: row.LastReason,
		}
		at, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
		if !item.Valid(scope.OrganizationID) || (query.Status != "" && item.Status != query.Status) || seen[item.RunID] ||
			(previousID != "" && !at.Before(previousAt) && (!at.Equal(previousAt) || item.RunID >= previousID)) {
			return app.EvaluationCatalogPage{}, app.ErrConflict
		}
		seen[item.RunID] = true
		page.Items = append(page.Items, item)
		previousAt, previousID = at, item.RunID
	}
	if raw.NextCursor != "" {
		next := query
		next.Cursor = raw.NextCursor
		at, id, ok := next.After(scope.OrganizationID)
		if !ok || len(page.Items) != query.Limit || id != previousID || !at.Equal(previousAt) {
			return app.EvaluationCatalogPage{}, app.ErrConflict
		}
	}
	page.NextCursor = raw.NextCursor
	return page, nil
}

func (c *EvaluationClient) ListEvaluations(ctx context.Context, scope app.DraftScope, query app.EvaluationCatalogQuery) (app.EvaluationCatalogPage, error) {
	if scope.OperatorUserID <= 0 || !query.Valid(scope.OrganizationID) {
		return app.EvaluationCatalogPage{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := c.RPC.List(ctx, &pb.EvaluationCatalogQuery{Scope: draftScope(scope), Status: query.Status, Limit: int32(query.Limit), Cursor: query.Cursor}, grpc.MaxCallRecvMsgSize(256*1024))
	if err != nil {
		return app.EvaluationCatalogPage{}, err
	}
	return evaluationCatalogPage(raw, scope, query)
}
