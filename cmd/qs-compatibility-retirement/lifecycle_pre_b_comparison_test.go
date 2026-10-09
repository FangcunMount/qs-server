package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"go.mongodb.org/mongo-driver/mongo"
)

// These field-only fixtures test process identity refusal; neither the zero
// Window nor any unissued plan/pair/client below passes native verification.
func preBComparisonBindingFixture() (*lifecycleFixedHost, lifecycleRequest, string) {
	r := lifecycleRequest{ToolSourceSHA: strings.Repeat("a", 40), OriginalSourceSHA: strings.Repeat("b", 40), OperationID: "17-1", ActualRunID: "18-1", ManifestSHA256: strings.Repeat("c", 64),
		ServiceControl: &lifecycleServiceControl{LocalDescriptorSHA256: strings.Repeat("d", 64), SSHChannelSHA256: strings.Repeat("e", 64)}, requestSHA256: strings.Repeat("f", 64), prepareRoot: "/exact/current/run"}
	h := &lifecycleFixedHost{owner: &lifecyclePreparationOwner{originalConn: new(sql.Conn), originalMongo: new(mongo.Client), originalDB: new(mongo.Database)}, services: &lifecycleServiceController{window: new(fence.MaintenanceWindow)},
		api: new(lifecycleAPITransition), dataBaseline: new(backup.NonTargetDataBaseline), acceptancePlan: new(backup.TargetRecoveryPlan), acceptancePair: new(migration.CompatibilityPairMigrationProof), comparisonAttempted: true}
	h.api.self = h.api
	h.api.bCID = strings.Repeat("1", 64)
	start := strings.Repeat("2", 64)
	f := &lifecyclePreBDataComparison{host: h, owner: h.owner, services: h.services, api: h.api, window: h.services.window, baseline: h.dataBaseline, plan: h.acceptancePlan, pair: h.acceptancePair,
		conn: h.owner.originalConn, client: h.owner.originalMongo, db: h.owner.originalDB, binding: lifecyclePreBComparisonExpected(r, start, h.acceptancePair)}
	f.self, f.seal, h.preBComparison = f, digest(f.binding), f
	return h, r, start
}

func TestPreBComparisonFactRejectsDifferentNativeOwnersAndRequest(t *testing.T) {
	for name, mutate := range map[string]func(*lifecycleFixedHost, *lifecycleRequest, *string){
		"host": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			h.preBComparison.host = new(lifecycleFixedHost)
		},
		"owner": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) { h.owner = new(lifecyclePreparationOwner) },
		"services": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			h.services = new(lifecycleServiceController)
		},
		"api": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) { h.api = new(lifecycleAPITransition) },
		"window": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			h.services.window = new(fence.MaintenanceWindow)
		},
		"baseline": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			h.dataBaseline = new(backup.NonTargetDataBaseline)
		},
		"plan": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			h.acceptancePlan = new(backup.TargetRecoveryPlan)
		},
		"pair": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			h.acceptancePair = new(migration.CompatibilityPairMigrationProof)
		},
		"SQL handle":     func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) { h.owner.originalConn = new(sql.Conn) },
		"Mongo client":   func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) { h.owner.originalMongo = new(mongo.Client) },
		"Mongo database": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) { h.owner.originalDB = new(mongo.Database) },
		"start":          func(_ *lifecycleFixedHost, _ *lifecycleRequest, s *string) { *s = strings.Repeat("3", 64) },
		"run":            func(_ *lifecycleFixedHost, r *lifecycleRequest, _ *string) { r.ActualRunID = "19-1" },
		"operation":      func(_ *lifecycleFixedHost, r *lifecycleRequest, _ *string) { r.OperationID = "20-1" },
		"source":         func(_ *lifecycleFixedHost, r *lifecycleRequest, _ *string) { r.ToolSourceSHA = strings.Repeat("4", 40) },
		"manifest": func(_ *lifecycleFixedHost, r *lifecycleRequest, _ *string) {
			r.ManifestSHA256 = strings.Repeat("5", 64)
		},
		"request bytes": func(_ *lifecycleFixedHost, r *lifecycleRequest, _ *string) { r.requestSHA256 = strings.Repeat("6", 64) },
		"request body":  func(_ *lifecycleFixedHost, r *lifecycleRequest, _ *string) { r.JournalDirectory = "/different/journal" },
		"prepare root":  func(_ *lifecycleFixedHost, r *lifecycleRequest, _ *string) { r.prepareRoot = "/different/current/run" },
		"seal": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			h.preBComparison.seal = strings.Repeat("7", 64)
		},
		"attempt": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) { h.comparisonAttempted = false },
		"copy": func(h *lifecycleFixedHost, _ *lifecycleRequest, _ *string) {
			copied := *h.preBComparison
			h.preBComparison = &copied
		},
	} {
		t.Run(name, func(t *testing.T) {
			h, r, start := preBComparisonBindingFixture()
			if !h.preBComparison.matches(h, r, start) {
				t.Fatal("field-only identity fixture is inconsistent")
			}
			mutate(h, &r, &start)
			if h.preBComparison.matches(h, r, start) {
				t.Fatal("different owner or binding accepted")
			}
		})
	}
	h, r, _ := preBComparisonBindingFixture()
	if h.verifyPreBDataComparison(t.Context(), r) == nil || h.acceptedMaterials != nil {
		t.Fatal("field fixture became a real Window or purge capability")
	}
}

func TestPreBComparisonFactIsNotSerializableOrImported(t *testing.T) {
	h, _, _ := preBComparisonBindingFixture()
	if _, e := json.Marshal(h.preBComparison); e == nil {
		t.Fatal("comparison fact serialized")
	}
	var imported lifecyclePreBDataComparison
	if json.Unmarshal([]byte(`{"comparison_success":true}`), &imported) == nil || imported.self != nil {
		t.Fatal("JSON minted a comparison fact")
	}
}

func TestPreBComparisonFailureRollsBackOriginalSQLScopeAndCannotPublishFact(t *testing.T) {
	for _, rollbackFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback-completes", true: "rollback-unknown"}[rollbackFails], func(t *testing.T) {
			pool, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = pool.Close() })
			conn, e := pool.Conn(t.Context())
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = conn.Close() })
			mock.ExpectBegin()
			rollback := mock.ExpectRollback()
			if rollbackFails {
				rollback.WillReturnError(errors.New("native driver rollback result unknown"))
			}
			// A disconnected real driver client cannot open a Mongo session and
			// performs no network operation. The already-open native SQL scope
			// must still be rolled back, including the failed cleanup result.
			client, e := mongo.NewClient() //nolint:staticcheck // A disconnected native client is required; Connect would start network work.
			if e != nil {
				t.Fatal(e)
			}
			h := &lifecycleFixedHost{owner: &lifecyclePreparationOwner{originalConn: conn, originalMongo: client, originalDB: client.Database("never-connected")}}
			if e = h.verifyCompleteDataBeforeInternalResume(t.Context(), new(migration.CompatibilityPairMigrationProof)); e == nil || h.preBComparison != nil {
				t.Fatal("failed RO scope produced a successful comparison fact")
			}
			if e = mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func preBComparisonProductionCalls(t *testing.T, file, function string) []string {
	t.Helper()
	parsed, e := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	var calls []string
	for _, declaration := range parsed.Decls {
		f, ok := declaration.(*ast.FuncDecl)
		if !ok || f.Name.Name != function {
			continue
		}
		ast.Inspect(f.Body, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if s, ok := c.Fun.(*ast.SelectorExpr); ok {
				calls = append(calls, s.Sel.Name)
			}
			return true
		})
	}
	if len(calls) == 0 {
		t.Fatal("actual production function was not inspected")
	}
	return calls
}

// A focused regression for the original P1: a normal B start must never
// precede the full stopped comparison. This does not simulate native DROP,
// root Window, full fence, Mongo cleanup or Docker/service acceptance.
func TestPreBComparisonActualCallerOrderPrecedesNormalBStartup(t *testing.T) {
	calls := preBComparisonProductionCalls(t, "lifecycle_fixed_host.go", "DeployBInline")
	seen := false
	for _, c := range calls {
		if c == "compareCompleteDataBeforeB" {
			seen = true
		}
		if c == "deploy" && !seen {
			t.Fatal("normal B startup preceded complete stopped data comparison")
		}
	}
	if !seen {
		t.Fatal("normal B startup lost its full comparison")
	}
	// The completed fact's producer must preserve the real stop/drop/pair
	// preconditions, then return from the full scope before any post-readback.
	calls = preBComparisonProductionCalls(t, "lifecycle_pre_b_comparison.go", "compareCompleteDataBeforeB")
	comparison := -1
	for i, c := range calls {
		if c == "verifyCompleteDataBeforeInternalResume" {
			if comparison >= 0 {
				t.Fatal("stopped comparison was repeated")
			}
			comparison = i
		}
	}
	for _, required := range []string{"Check", "CheckWholeWriterFence", "VerifyDroppedTargets", "VerifyAfter"} {
		before, after := false, false
		for i, c := range calls {
			if c == required {
				before = before || i < comparison
				after = after || i > comparison
			}
		}
		if comparison < 0 || !before || !after {
			t.Fatal("full stopped/drop/pair/fence condition lost around actual comparison: " + required)
		}
	}
	calls = preBComparisonProductionCalls(t, "lifecycle_native_acceptance.go", "verifyNativeAcceptance")
	seen = false
	for _, c := range calls {
		if c == "verifyCompleteDataBeforeInternalResume" {
			t.Fatal("acceptance rescanned a stopped snapshot after B background writers started")
		}
		if c == "verifyPreBDataComparison" {
			seen = true
		}
		if c == "observeAcceptance" && !seen {
			t.Fatal("runtime acceptance ignored the original comparison fact")
		}
	}
	if !seen {
		t.Fatal("runtime acceptance lost its process-bound comparison")
	}
}

func TestPreBComparisonOriginalFailureDoesNotRetryDeployment(t *testing.T) {
	h, r, _ := preBComparisonBindingFixture()
	for _, attempted := range []bool{false, true} {
		h.comparisonAttempted = attempted
		if h.DeployBInline(context.Background(), r, h.acceptancePair, h.services.window) == nil {
			t.Fatal("existing comparison or unissued Window enabled repeated deployment")
		}
	}
	if lifecycleEffectsPreflight(t.Context()) == nil {
		t.Fatal("ordering repair opened incomplete production effects")
	}
}
