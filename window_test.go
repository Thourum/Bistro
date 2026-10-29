package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Native getters synchronously dispatch to the thread running shutdown hooks.
// Guard their lock ordering without requiring a GUI/main-thread test harness.
func TestWindowSaveReadsNativeStateBeforeLocking(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "window.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var save *ast.FuncLit
	ast.Inspect(file, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		name, ok := assignment.Lhs[0].(*ast.Ident)
		if ok && name.Name == "save" {
			save, _ = assignment.Rhs[0].(*ast.FuncLit)
		}
		return true
	})
	if save == nil {
		t.Fatal("window save callback not found")
	}
	calls := map[string]token.Pos{}
	ast.Inspect(save.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok {
			calls[selector.Sel.Name] = call.Pos()
		}
		return true
	})
	for _, getter := range []string{"Bounds", "IsMaximised"} {
		if calls[getter] == token.NoPos || calls["Lock"] == token.NoPos || calls[getter] >= calls["Lock"] {
			t.Fatalf("%s must run before locking the shutdown mutex", getter)
		}
	}
	if calls["SetWindow"] <= calls["Lock"] {
		t.Fatal("persisting bounds must remain behind the shutdown/generation guard")
	}
}
