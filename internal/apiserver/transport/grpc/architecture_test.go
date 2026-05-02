package grpc

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestSurveyScaleGRPCServicesUseApplicationDTOs(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel":        "application query DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel":         "application query DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire": "application query DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale":                "application query DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation":          "application result DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation":           "application result DTOs",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode":          "survey/scale application QR-code use cases",
	}
	for _, path := range []string{
		filepath.Join("service", "answersheet.go"),
		filepath.Join("service", "internal.go"),
		filepath.Join("service", "questionnaire.go"),
		filepath.Join("service", "scale.go"),
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if replacement, ok := forbiddenImports[importPath]; ok {
				t.Fatalf("%s imports %s; survey/scale gRPC services should use %s", path, importPath, replacement)
			}
		}
	}
}

func TestGRPCTransportDoesNotHoldScaleDomainRepository(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"registry.go",
		filepath.Join("service", "internal.go"),
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale" {
				t.Fatalf("%s imports %s; gRPC transport must use scale application ports, not scale repositories", path, importPath)
			}
		}
	}
}

func TestEvaluationGRPCTransportDoesNotHoldEvaluationDomainRepository(t *testing.T) {
	t.Parallel()

	forbiddenImports := map[string]string{
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment": "evaluation application service",
		"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report":     "evaluation application service",
		"github.com/FangcunMount/qs-server/internal/apiserver/infra/waiter":                 "evaluation application wait service",
		"github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access":     "evaluation application access query service",
	}
	for _, path := range []string{
		"registry.go",
		filepath.Join("service", "evaluation.go"),
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if replacement, ok := forbiddenImports[importPath]; ok {
				t.Fatalf("%s imports %s; gRPC evaluation transport must use %s", path, importPath, replacement)
			}
		}
	}
}
