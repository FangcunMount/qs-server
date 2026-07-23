package inputinvariant_test

import (
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/inputinvariant"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestValidateRejectsQuestionnaireVersionMismatch(t *testing.T) {
	t.Parallel()
	a := submittedAssessment("Q-1", "1.0.0", "M-1", "1.0.0")
	err := inputinvariant.Validate(inputinvariant.Input{
		Assessment: a,
		Snapshot: &evaluationinput.InputSnapshot{
			Model: &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "M-1", Version: "1.0.0"},
			ModelPayload: evaluationinput.TypologyModelPayload{Payload: &typology.Payload{
				Code: "M-1", Version: "1.0.0", QuestionnaireCode: "Q-1", QuestionnaireVersion: "2.0.0", Status: "published",
			}},
			AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 9, QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0"},
			Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-1", Version: "1.0.0"},
		},
		DescriptorKey: "factor_classification",
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want version mismatch")
	}
	var inv *inputinvariant.Error
	if !asInvariantError(err, &inv) || inv.Code != "input.questionnaire.version_mismatch" {
		t.Fatalf("Validate() = %v, want input.questionnaire.version_mismatch", err)
	}
	if inv.AnswerSheetID != 9 || !strings.Contains(inv.Error(), "factor_classification") {
		t.Fatalf("error context = %#v", inv)
	}
}

func TestValidateAcceptsAlignedTypologyInput(t *testing.T) {
	t.Parallel()
	a := submittedAssessment("Q-1", "1.0.0", "M-1", "1.0.0")
	err := inputinvariant.Validate(inputinvariant.Input{
		Assessment: a,
		Snapshot: &evaluationinput.InputSnapshot{
			Model: &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "M-1", Version: "1.0.0"},
			ModelPayload: evaluationinput.TypologyModelPayload{Payload: &typology.Payload{
				Code: "M-1", Version: "1.0.0", QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0", Status: "published",
			}},
			AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 1, QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0"},
			Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-1", Version: "1.0.0"},
		},
		DescriptorKey: "factor_classification",
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsModelVersionMismatch(t *testing.T) {
	t.Parallel()
	a := submittedAssessment("Q-1", "1.0.0", "M-1", "1.0.0")
	err := inputinvariant.Validate(inputinvariant.Input{
		Assessment: a,
		Snapshot: &evaluationinput.InputSnapshot{
			Model:         &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "M-1", Version: "2.0.0"},
			AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 1, QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0"},
			Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-1", Version: "1.0.0"},
		},
		DescriptorKey: "factor_classification",
	})
	if err == nil {
		t.Fatal("Validate() error = nil, want model version mismatch")
	}
	var inv *inputinvariant.Error
	if !asInvariantError(err, &inv) || inv.Code != "input.model.version_mismatch" {
		t.Fatalf("Validate() = %v, want input.model.version_mismatch", err)
	}
}

func TestValidateRejectsExactModelIdentityMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*evaluationinput.ModelSnapshot)
		code   string
	}{
		{name: "kind", mutate: func(m *evaluationinput.ModelSnapshot) { m.Kind = evaluationinput.EvaluationModelKindScale }, code: "input.model.kind_mismatch"},
		{name: "algorithm", mutate: func(m *evaluationinput.ModelSnapshot) { m.Algorithm = "brief2" }, code: "input.model.algorithm_mismatch"},
		{name: "code", mutate: func(m *evaluationinput.ModelSnapshot) { m.Code = "OTHER" }, code: "input.model.code_mismatch"},
		{name: "version", mutate: func(m *evaluationinput.ModelSnapshot) { m.Version = "2.0.0" }, code: "input.model.version_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &evaluationinput.ModelSnapshot{Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmPersonalityTypology), Code: "M-1", Version: "1.0.0"}
			tt.mutate(model)
			err := inputinvariant.Validate(inputinvariant.Input{
				Assessment: submittedAssessment("Q-1", "1.0.0", "M-1", "1.0.0"),
				Snapshot: &evaluationinput.InputSnapshot{
					Model:         model,
					AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 7, QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0"},
					Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-1", Version: "1.0.0"},
				},
				DescriptorKey: "factor_classification",
			})
			var inv *inputinvariant.Error
			if !asInvariantError(err, &inv) || inv.Code != tt.code {
				t.Fatalf("Validate() = %v, want %s", err, tt.code)
			}
			if inv.AnswerSheetID != 7 || inv.DescriptorKey != "factor_classification" || inv.ModelRef == "" || inv.QuestionnaireRef == "" {
				t.Fatalf("error context = %#v", inv)
			}
		})
	}
}

func TestValidateRequiresDescriptorAndModelSnapshot(t *testing.T) {
	t.Parallel()
	a := submittedAssessment("Q-1", "1.0.0", "M-1", "1.0.0")
	base := &evaluationinput.InputSnapshot{
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{ID: 1, QuestionnaireCode: "Q-1", QuestionnaireVersion: "1.0.0"},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-1", Version: "1.0.0"},
	}
	var inv *inputinvariant.Error
	if err := inputinvariant.Validate(inputinvariant.Input{Assessment: a, Snapshot: base}); !asInvariantError(err, &inv) || inv.Code != "input.descriptor.required" {
		t.Fatalf("descriptor error = %v", err)
	}
	if err := inputinvariant.Validate(inputinvariant.Input{Assessment: a, Snapshot: base, DescriptorKey: "factor_classification"}); !asInvariantError(err, &inv) || inv.Code != "input.model_snapshot.required" {
		t.Fatalf("model snapshot error = %v", err)
	}
}

func submittedAssessment(qCode, qVersion, modelCode, modelVersion string) *assessment.Assessment {
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindTypology,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmPersonalityTypology,
		meta.ID(0),
		meta.NewCode(modelCode),
		modelVersion,
		"Model",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode(qCode), qVersion),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		panic(err)
	}
	if err := a.Submit(); err != nil {
		panic(err)
	}
	return a
}

func asInvariantError(err error, target **inputinvariant.Error) bool {
	if err == nil {
		return false
	}
	inv, ok := err.(*inputinvariant.Error)
	if !ok {
		return false
	}
	*target = inv
	return true
}
