package handler

import (
	"context"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

func (g *managementGateway) ListEvaluations(_ context.Context, s app.DraftScope, _ app.EvaluationCatalogQuery) (app.EvaluationCatalogPage, error) {
	g.calls++
	g.scope = app.EvaluationScope{OrganizationID: s.OrganizationID, OperatorUserID: s.OperatorUserID}
	return app.EvaluationCatalogPage{Items: []app.EvaluationSummary{}}, nil
}
