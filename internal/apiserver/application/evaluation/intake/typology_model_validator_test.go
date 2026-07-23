package intake

import (
	"context"
	"errors"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type activeReaderStub struct {
	called   bool
	lastRef  port.Ref
	snapshot *port.PublishedModel
	err      error
}

func (s *activeReaderStub) GetActivePublishedModelByRef(_ context.Context, ref port.Ref) (*port.PublishedModel, error) {
	s.called = true
	s.lastRef = ref
	return s.snapshot, s.err
}

type frozenReleaseReaderStub struct {
	activeReaderStub
	retainedCalled bool
	retained       *port.PublishedModel
	retainedErr    error
}

func (s *frozenReleaseReaderStub) GetPublishedModelByRef(_ context.Context, ref port.Ref) (*port.PublishedModel, error) {
	s.retainedCalled = true
	s.lastRef = ref
	return s.retained, s.retainedErr
}

func TestPublishedModelValidatorFailsClosedForInvalidModeAndMissingSelectedReader(t *testing.T) {
	t.Parallel()

	modelRef := evalassessment.NewEvaluationModelRefWithIdentity(
		domainmodel.KindScale,
		domainmodel.SubKindEmpty,
		domainmodel.AlgorithmScaleDefault,
		0,
		meta.NewCode("MODEL-1"),
		"v1",
		"Model",
	)
	questionnaireRef := evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0")

	t.Run("invalid mode", func(t *testing.T) {
		validator := NewPublishedEvaluationModelValidator(&activeReaderStub{}, nil)
		err := validator.ValidateEvaluationModel(context.Background(), modelRef, questionnaireRef, ModelValidationMode("unknown"))
		if !cberrors.IsCode(err, errorCode.ErrInvalidArgument) {
			t.Fatalf("error = %v, want InvalidArgument", err)
		}
	})

	t.Run("active reader missing", func(t *testing.T) {
		validator := NewPublishedEvaluationModelValidator(nil, &frozenReleaseReaderStub{})
		err := validator.ValidateEvaluationModel(context.Background(), modelRef, questionnaireRef, ModelValidationModeActiveRelease)
		if !cberrors.IsCode(err, errorCode.ErrModuleInitializationFailed) {
			t.Fatalf("error = %v, want ModuleNotConfigured", err)
		}
	})

	t.Run("retained reader missing", func(t *testing.T) {
		validator := NewPublishedEvaluationModelValidator(&activeReaderStub{}, nil)
		err := validator.ValidateEvaluationModel(context.Background(), modelRef, questionnaireRef, ModelValidationModeRetainedExact)
		if !cberrors.IsCode(err, errorCode.ErrModuleInitializationFailed) {
			t.Fatalf("error = %v, want ModuleNotConfigured", err)
		}
	})
}

func TestPublishedModelValidatorChecksQuestionnaireInBothModes(t *testing.T) {
	t.Parallel()

	snapshot := &port.PublishedModel{
		Kind:                 domainmodel.KindScale,
		Algorithm:            domainmodel.AlgorithmScaleDefault,
		Code:                 "MODEL-1",
		Version:              "v1",
		QuestionnaireCode:    "Q-OTHER",
		QuestionnaireVersion: "1.0.0",
	}
	reader := &frozenReleaseReaderStub{
		activeReaderStub: activeReaderStub{snapshot: snapshot},
		retained:         snapshot,
	}
	modelRef := evalassessment.NewEvaluationModelRefWithIdentity(
		domainmodel.KindScale,
		domainmodel.SubKindEmpty,
		domainmodel.AlgorithmScaleDefault,
		0,
		meta.NewCode("MODEL-1"),
		"v1",
		"Model",
	)
	questionnaireRef := evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0")

	for _, mode := range []ModelValidationMode{ModelValidationModeActiveRelease, ModelValidationModeRetainedExact} {
		if err := NewPublishedEvaluationModelValidator(reader, reader).ValidateEvaluationModel(
			context.Background(),
			modelRef,
			questionnaireRef,
			mode,
		); !errors.Is(err, evalassessment.ErrEvaluationModelQuestionnaireMismatch) {
			t.Fatalf("mode %q error = %v, want questionnaire mismatch", mode, err)
		}
	}
}

func TestPublishedModelValidatorRejectsReturnedIdentityMismatchInBothModes(t *testing.T) {
	t.Parallel()

	snapshot := &port.PublishedModel{
		Kind:                 domainmodel.KindScale,
		Algorithm:            domainmodel.AlgorithmScaleDefault,
		Code:                 "MODEL-1",
		Version:              "v2",
		QuestionnaireCode:    "Q-1",
		QuestionnaireVersion: "1.0.0",
	}
	reader := &frozenReleaseReaderStub{
		activeReaderStub: activeReaderStub{snapshot: snapshot},
		retained:         snapshot,
	}
	modelRef := evalassessment.NewEvaluationModelRefWithIdentity(
		domainmodel.KindScale,
		domainmodel.SubKindEmpty,
		domainmodel.AlgorithmScaleDefault,
		0,
		meta.NewCode("MODEL-1"),
		"v1",
		"Model",
	)
	questionnaireRef := evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0")

	for _, mode := range []ModelValidationMode{ModelValidationModeActiveRelease, ModelValidationModeRetainedExact} {
		if err := NewPublishedEvaluationModelValidator(reader, reader).ValidateEvaluationModel(
			context.Background(),
			modelRef,
			questionnaireRef,
			mode,
		); !errors.Is(err, evalassessment.ErrEvaluationModelNotPublished) {
			t.Fatalf("mode %q error = %v, want identity mismatch rejection", mode, err)
		}
	}
}

func (s *frozenReleaseReaderStub) FindPublishedModelByQuestionnaire(context.Context, string, string) (*port.PublishedModel, error) {
	return nil, domainmodel.ErrNotFound
}

func TestPublishedModelValidatorAcceptsFrozenRetainedReleaseAfterArchive(t *testing.T) {
	t.Parallel()

	reader := &frozenReleaseReaderStub{
		activeReaderStub: activeReaderStub{err: domainmodel.ErrNotFound},
		retained: &port.PublishedModel{
			Kind:                 domainmodel.KindScale,
			Algorithm:            domainmodel.AlgorithmScaleDefault,
			Code:                 "MODEL-1",
			Version:              "v1",
			QuestionnaireCode:    "Q-1",
			QuestionnaireVersion: "1.0.0",
		},
	}
	validator := NewPublishedEvaluationModelValidator(reader, reader)

	err := validator.ValidateEvaluationModel(
		context.Background(),
		evalassessment.NewEvaluationModelRefWithIdentity(
			domainmodel.KindScale,
			domainmodel.SubKindEmpty,
			domainmodel.AlgorithmScaleDefault,
			0,
			meta.NewCode("MODEL-1"),
			"v1",
			"Model",
		),
		evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
		ModelValidationModeRetainedExact,
	)
	if err != nil {
		t.Fatalf("submit-time frozen retained release was rejected after archive: %v", err)
	}
	if !reader.retainedCalled {
		t.Fatal("frozen release validation did not use the retained exact-version reader")
	}
}

func TestPublishedModelValidatorUsesActiveReleaseBoundaryForEveryKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		kind      domainmodel.Kind
		subKind   domainmodel.SubKind
		algorithm domainmodel.Algorithm
	}{
		{name: "scale", kind: domainmodel.KindScale, algorithm: domainmodel.AlgorithmScaleDefault},
		{name: "typology", kind: domainmodel.KindTypology, subKind: domainmodel.SubKindTypology, algorithm: domainmodel.AlgorithmPersonalityTypology},
		{name: "cognitive", kind: domainmodel.KindCognitive, algorithm: domainmodel.AlgorithmSPM},
		{name: "behavioral-rating", kind: domainmodel.KindBehavioralRating, algorithm: domainmodel.AlgorithmSPMSensory},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := &activeReaderStub{err: domainmodel.ErrNotFound}
			validator := NewPublishedEvaluationModelValidator(reader, nil)
			err := validator.ValidateEvaluationModel(
				context.Background(),
				evalassessment.NewEvaluationModelRefWithIdentity(
					tc.kind, tc.subKind, tc.algorithm, 0, meta.NewCode("MODEL-1"), "v1", "Model",
				),
				evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
				ModelValidationModeActiveRelease,
			)
			if !reader.called || reader.lastRef.Kind != tc.kind {
				t.Fatalf("active release lookup = called:%t ref:%#v", reader.called, reader.lastRef)
			}
			if !errors.Is(err, evalassessment.ErrEvaluationModelNotPublished) {
				t.Fatalf("error = %v, want ErrEvaluationModelNotPublished", err)
			}
		})
	}
}
