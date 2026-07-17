package intake

import (
	"context"
	"errors"
	"testing"

	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type activeReaderStub struct {
	called  bool
	lastRef port.Ref
	err     error
}

func (s *activeReaderStub) GetActivePublishedModelByRef(_ context.Context, ref port.Ref) (*port.PublishedModel, error) {
	s.called = true
	s.lastRef = ref
	return nil, s.err
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
		{name: "typology", kind: domainmodel.KindTypology, subKind: domainmodel.SubKindTypology, algorithm: domainmodel.AlgorithmMBTI},
		{name: "cognitive", kind: domainmodel.KindCognitive, algorithm: domainmodel.AlgorithmSPM},
		{name: "behavioral-rating", kind: domainmodel.KindBehavioralRating, algorithm: domainmodel.AlgorithmSPMSensory},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := &activeReaderStub{err: domainmodel.ErrNotFound}
			validator := NewPublishedEvaluationModelValidator(reader)
			err := validator.ValidateEvaluationModel(
				context.Background(),
				evalassessment.NewEvaluationModelRefWithIdentity(
					tc.kind, tc.subKind, tc.algorithm, 0, meta.NewCode("MODEL-1"), "v1", "Model",
				),
				evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
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
