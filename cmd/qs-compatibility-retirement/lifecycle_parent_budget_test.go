package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"
)

func TestLifecycleOriginalParentWindowBudget(t *testing.T) {
	for _, tc := range []struct {
		name     string
		duration time.Duration
		canceled bool
		allowed  bool
	}{
		{name: "original_ninety_minutes", duration: 90 * time.Minute, allowed: true},
		{name: "full_window_plus_margin", duration: 31 * time.Minute, allowed: true},
		{name: "preparation_consumed_over_sixty_minutes", duration: 29 * time.Minute},
		{name: "at_boundary_already_elapsed", duration: 30 * time.Minute},
		{name: "recovery_only_remaining", duration: 10 * time.Minute},
		{name: "expired", duration: -time.Second},
		{name: "canceled_even_with_full_budget", duration: 90 * time.Minute, canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), tc.duration)
			defer cancel()
			deadline, ok := parent.Deadline()
			if !ok {
				t.Fatal("test requires a real parent deadline")
			}
			if tc.canceled {
				cancel()
			}
			err := lifecycleRequireParentWindowBudget(parent)
			if (err == nil) != tc.allowed {
				t.Fatalf("parent budget allowed=%v, error=%v", tc.allowed, err)
			}
			if err != nil && lifecycleCategory(err) != "lifecycle_parent_window_budget_rejected" {
				t.Fatal("parent budget failure lost its fixed error category")
			}
			after, _ := parent.Deadline()
			if after != deadline || (tc.allowed && parent.Err() != nil) {
				t.Fatal("budget check renewed or canceled the original parent")
			}
		})
	}
	for _, parent := range []context.Context{nil, context.Background()} {
		if err := lifecycleRequireParentWindowBudget(parent); err == nil || lifecycleCategory(err) != "lifecycle_parent_window_budget_rejected" {
			t.Fatal("missing original parent deadline accepted")
		}
	}
}

// This checks production ordering only. No root Window, native service lease,
// stop, recovery or effect authority is constructed by this test.
func TestLifecycleParentBudgetGuardsOriginalStartAndManagementOrder(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "lifecycle.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var run *ast.FuncDecl
	for _, declaration := range f.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == "runLifecycleCLI" {
			run = fn
		}
	}
	if run == nil {
		t.Fatal("actual lifecycle caller missing")
	}
	var guards []token.Pos
	var start, open, forward, management, stop, closeInstalled token.Pos
	for _, statement := range run.Body.List {
		ast.Inspect(statement, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name, ok := call.Fun.(*ast.Ident); ok && name.Name == "lifecycleRequireParentWindowBudget" {
				if len(call.Args) != 1 {
					t.Fatal("budget guard does not receive the original parent")
				}
				parent, ok := call.Args[0].(*ast.Ident)
				if !ok || parent.Name != "ctx" {
					t.Fatal("budget guard uses a replacement/forward context")
				}
				guard, ok := statement.(*ast.IfStmt)
				if !ok || !lifecycleTestApplyGuard(guard.Cond) {
					t.Fatal("apply budget check changed verify/purge/recovery semantics")
				}
				guards = append(guards, call.Pos())
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "StartMaintenanceWindow":
				start = call.Pos()
			case "OpenMaintenanceWindow":
				open = call.Pos()
			case "ForwardContext":
				forward = call.Pos()
			case "OpenServiceManagement":
				management = call.Pos()
			case "StopAndDrain":
				stop = call.Pos()
			case "Close":
				if owner, ok := selector.X.(*ast.Ident); ok && owner.Name == "window" {
					if _, ok := statement.(*ast.DeferStmt); !ok {
						t.Fatal("original Window is not closed through its preserved defer")
					}
					closeInstalled = statement.Pos()
				}
			}
			return true
		})
	}
	if len(guards) != 2 || guards[0] >= start || start >= open || open >= closeInstalled || closeInstalled >= guards[1] || guards[1] >= forward || forward >= management || management >= stop {
		t.Fatal("original parent guards must surround native start, preserve Close, and precede all service management/stop")
	}
}

func lifecycleTestApplyGuard(condition ast.Expr) bool {
	binary, ok := condition.(*ast.BinaryExpr)
	if !ok || binary.Op != token.EQL {
		return false
	}
	name, nameOK := binary.X.(*ast.Ident)
	value, valueOK := binary.Y.(*ast.BasicLit)
	return nameOK && valueOK && name.Name == "stage" && value.Kind == token.STRING && value.Value == `"apply"`
}

// Source ordering checks protect the real fixed caller while its production
// effects gate remains closed; they do not mint a Window, lease or fence.
func TestLifecycleWriterKernelPreconditionsGuardStopAndFreshFence(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "lifecycle.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var run *ast.FuncDecl
	for _, declaration := range f.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == "runLifecycleCLI" {
			run = fn
		}
	}
	if run == nil {
		t.Fatal("actual lifecycle caller missing")
	}
	positions := map[string]token.Pos{}
	for _, statement := range run.Body.List {
		ast.Inspect(statement, func(node ast.Node) bool {
			if _, deferred := node.(*ast.FuncLit); deferred {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := selector.Sel.Name
			switch name {
			case "OpenServiceManagement", "CheckWriterPreconditions", "StopAndDrain", "CheckWholeWriterFence", "FinalDifferenceAndEOF", "PrepareTargetRecovery", "ApplyTargets":
				if positions[name] == 0 {
					positions[name] = call.Pos()
				}
			}
			if name == "CheckWriterPreconditions" || name == "StopAndDrain" || name == "CheckWholeWriterFence" {
				guard, ok := statement.(*ast.IfStmt)
				if !ok || guard.Init == nil || guard.Cond == nil || len(guard.Body.List) != 1 {
					t.Fatal("writer boundary is not an immediate failure guard")
				}
				failure, ok := guard.Cond.(*ast.BinaryExpr)
				if !ok || failure.Op != token.NEQ {
					t.Fatal("writer boundary no longer fails on error")
				}
				value, valueOK := failure.X.(*ast.Ident)
				zero, zeroOK := failure.Y.(*ast.Ident)
				if !valueOK || value.Name != "err" || !zeroOK || zero.Name != "nil" {
					t.Fatal("writer boundary result ignored")
				}
				if _, ok := guard.Body.List[0].(*ast.ReturnStmt); !ok {
					t.Fatal("failed writer boundary still advances")
				}
			}
			return true
		})
	}
	sequence := []string{"OpenServiceManagement", "CheckWriterPreconditions", "StopAndDrain", "CheckWholeWriterFence", "FinalDifferenceAndEOF", "PrepareTargetRecovery", "ApplyTargets"}
	var previous token.Pos
	for _, name := range sequence {
		if positions[name] == 0 || positions[name] <= previous {
			t.Fatalf("native kernel order violated at %s", name)
		}
		previous = positions[name]
	}
}
