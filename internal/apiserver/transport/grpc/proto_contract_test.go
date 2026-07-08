package grpc

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProtoGoPackageContractUsesSharedGRPCGenPath(t *testing.T) {
	t.Parallel()

	protoRoot := filepath.Clean("../../../../api/grpc/proto")
	wantPrefix := "github.com/FangcunMount/qs-server/api/grpc/gen/"
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
	protoRoot := filepath.Clean("../../../../api/grpc/proto")
	serviceRe := regexp.MustCompile(`(?m)^service\s+(\w+)`)
	constructorByService := map[string]string{
		"ActorService":         "NewActorService",
		"AnswerSheetService":   "NewAnswerSheetService",
		"EvaluationService":    "NewEvaluationService",
		"InternalService":      "NewInternalService",
		"PlanCommandService":   "NewPlanCommandService",
		"QuestionnaireService": "NewQuestionnaireService",
		"ScaleService":         "NewScaleService",
		"TypologyModelService": "NewTypologyModelService",
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

func TestInternalProtoHasNoTagTesteeRPC(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../api/grpc/proto/internalapi/internal.proto")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{
		"rpc TagTestee(",
		"message TagTesteeRequest",
		"message TagTesteeResponse",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("internal.proto still contains legacy TagTestee contract: %s", forbidden)
		}
	}
}

func TestInternalProtoEvaluateAssessmentResponseHasNoLegacyFields(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../api/grpc/proto/internalapi/internal.proto")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	msgStart := strings.Index(source, "message EvaluateAssessmentResponse {")
	if msgStart < 0 {
		t.Fatal("missing EvaluateAssessmentResponse message")
	}
	msgEnd := strings.Index(source[msgStart:], "\n}")
	if msgEnd < 0 {
		t.Fatal("unterminated EvaluateAssessmentResponse message")
	}
	body := source[msgStart : msgStart+msgEnd]
	for _, forbidden := range []string{
		"total_score = 4",
		"risk_level = 5",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("EvaluateAssessmentResponse still contains legacy field: %s", forbidden)
		}
	}
}

func TestListTypologyModelsRequestHasNoAlgorithmFilter(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../api/grpc/proto/typologymodel/typology_model.proto")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	msgStart := strings.Index(source, "message ListTypologyModelsRequest {")
	if msgStart < 0 {
		t.Fatal("missing ListTypologyModelsRequest message")
	}
	msgEnd := strings.Index(source[msgStart:], "\n}")
	if msgEnd < 0 {
		t.Fatal("unterminated ListTypologyModelsRequest message")
	}
	body := source[msgStart : msgStart+msgEnd]
	if strings.Contains(body, "algorithm =") {
		t.Fatal("ListTypologyModelsRequest still contains legacy algorithm filter")
	}
}

func TestListMyAssessmentsRequestHasNoModelAlgorithmFilter(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../api/grpc/proto/evaluation/evaluation.proto")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	msgStart := strings.Index(source, "message ListMyAssessmentsRequest {")
	if msgStart < 0 {
		t.Fatal("missing ListMyAssessmentsRequest message")
	}
	msgEnd := strings.Index(source[msgStart:], "\n}")
	if msgEnd < 0 {
		t.Fatal("unterminated ListMyAssessmentsRequest message")
	}
	body := source[msgStart : msgStart+msgEnd]
	if strings.Contains(body, "model_algorithm =") {
		t.Fatal("ListMyAssessmentsRequest still contains legacy model_algorithm filter")
	}
}

func TestEvaluationProtoAssessmentOutcomeHasNoLegacyFields(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../api/grpc/proto/evaluation/evaluation.proto")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, msgName := range []string{"AssessmentSummary", "AssessmentDetail", "AssessmentReport"} {
		msgStart := strings.Index(source, "message "+msgName+" {")
		if msgStart < 0 {
			t.Fatalf("missing %s message", msgName)
		}
		msgEnd := strings.Index(source[msgStart:], "\n}")
		if msgEnd < 0 {
			t.Fatalf("unterminated %s message", msgName)
		}
		body := source[msgStart : msgStart+msgEnd]
		for _, forbidden := range []string{
			"scale_code =",
			"scale_name =",
			"total_score =",
			"risk_level =",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s still contains legacy outcome field: %s", msgName, forbidden)
			}
		}
	}
}

func TestEvaluationProtoHasNoDeprecatedAnswerSheetDetailRPC(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../api/grpc/proto/evaluation/evaluation.proto")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{
		"rpc GetMyAssessmentByAnswerSheetID",
		"message GetMyAssessmentByAnswerSheetIDRequest",
		"message GetMyAssessmentByAnswerSheetIDResponse",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("evaluation.proto still contains deprecated answer-sheet detail contract: %s", forbidden)
		}
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
