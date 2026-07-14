package rest

import (
	"context"
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/gin-gonic/gin"
)

type normTableRouteServiceStub struct{}

func (normTableRouteServiceStub) Import(context.Context, modelcatalog.ActorContext, *domain.Norm) (*modelcatalog.NormTableDetail, error) {
	return nil, nil
}
func (normTableRouteServiceStub) Get(context.Context, modelcatalog.ActorContext, string) (*modelcatalog.NormTableDetail, error) {
	return nil, nil
}
func (normTableRouteServiceStub) List(context.Context, modelcatalog.ActorContext, modelcatalog.ListNormTablesDTO) (*modelcatalog.NormTableListResult, error) {
	return nil, nil
}

func TestRegisterNormTableProtectedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router := &Router{deps: Deps{AssessmentModel: AssessmentModelDeps{NormTables: normTableRouteServiceStub{}}}}
	router.registerNormTableProtectedRoutes(engine.Group("/api/v1"))

	want := map[string]bool{
		"GET /api/v1/norm-tables":          false,
		"GET /api/v1/norm-tables/:version": false,
		"POST /api/v1/norm-tables":         false,
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, registered := range want {
		if !registered {
			t.Errorf("route %s was not registered", route)
		}
	}
}
