package grpc

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProtoGoPackageContractAllowsOnlyDeclaredLegacyPaths(t *testing.T) {
	t.Parallel()

	protoRoot := filepath.Clean("../../interface/grpc/proto")
	wantPrefix := "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/"
	legacyAllowlist := map[string]string{
		"answersheet/answersheet.proto":     "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/answersheet",
		"questionnaire/questionnaire.proto": "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/grpc/proto/questionnaire",
	}
	re := regexp.MustCompile(`option go_package = "([^"]+)"`)

	err := filepath.WalkDir(protoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".proto") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		match := re.FindStringSubmatch(string(data))
		if len(match) != 2 {
			t.Fatalf("%s missing option go_package", path)
		}
		rel, err := filepath.Rel(protoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if legacy, ok := legacyAllowlist[rel]; ok {
			if match[1] != legacy {
				t.Fatalf("%s go_package = %q, want legacy allowlist %q", rel, match[1], legacy)
			}
			return nil
		}
		if !strings.HasPrefix(match[1], wantPrefix) {
			t.Fatalf("%s go_package = %q, want prefix %q", rel, match[1], wantPrefix)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGRPCRegistryHasConstructorForEveryProtoService(t *testing.T) {
	t.Parallel()

	registryData, err := os.ReadFile("registry.go")
	if err != nil {
		t.Fatal(err)
	}
	registry := string(registryData)
	protoRoot := filepath.Clean("../../interface/grpc/proto")
	serviceRe := regexp.MustCompile(`(?m)^service\s+(\w+)`)
	constructorByService := map[string]string{
		"ActorService":         "NewActorService",
		"AnswerSheetService":   "NewAnswerSheetService",
		"EvaluationService":    "NewEvaluationService",
		"InternalService":      "NewInternalService",
		"PlanCommandService":   "NewPlanCommandService",
		"QuestionnaireService": "NewQuestionnaireService",
		"ScaleService":         "NewScaleService",
	}

	err = filepath.WalkDir(protoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".proto") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range serviceRe.FindAllStringSubmatch(string(data), -1) {
			serviceName := match[1]
			constructor := constructorByService[serviceName]
			if constructor == "" {
				t.Fatalf("%s declares service %s without registry constructor contract", path, serviceName)
			}
			if !strings.Contains(registry, constructor+"(") {
				t.Fatalf("registry.go does not construct proto service %s via %s", serviceName, constructor)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGRPCRegistryImportsTransportOwnedServiceFacade(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("registry.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if strings.Contains(source, "internal/apiserver/interface/grpc/service") {
		t.Fatal("transport/grpc registry must import transport/grpc/service, not legacy interface/grpc/service")
	}
	if !strings.Contains(source, "internal/apiserver/transport/grpc/service") {
		t.Fatal("transport/grpc registry should depend on the transport-owned service facade")
	}
}
