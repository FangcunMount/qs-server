package evaluationinput_test

import (
	"context"
	"errors"
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

type invalidBehavioralCatalog struct{ err error }

func (c invalidBehavioralCatalog) GetBehavioralRatingByRef(context.Context, port.ModelRef) (*behavioralpayload.Snapshot, error) {
	return nil, c.err
}

func (c invalidBehavioralCatalog) FindBehavioralRatingByQuestionnaire(context.Context, string, string) (*behavioralpayload.Snapshot, error) {
	return nil, c.err
}

type invalidCognitiveCatalog struct{ err error }

func (c invalidCognitiveCatalog) GetCognitiveByRef(context.Context, port.ModelRef) (*cognitivepayload.Snapshot, error) {
	return nil, c.err
}

func (c invalidCognitiveCatalog) FindCognitiveByQuestionnaire(context.Context, string, string) (*cognitivepayload.Snapshot, error) {
	return nil, c.err
}

func TestModelProvidersDoNotDisguiseInvalidNormAsDependencyFailure(t *testing.T) {
	t.Parallel()

	invalidNorm := calcnorm.NewInvalidError("total", errors.New("bad lookup"))
	tests := []struct {
		name    string
		resolve func() error
	}{
		{
			name: "behavioral",
			resolve: func() error {
				provider := evaluationinputinfra.NewBehavioralRatingModelInputProvider(modelcatalog.AlgorithmBrief2, invalidBehavioralCatalog{err: invalidNorm}, nil, nil, nil, nil)
				_, err := provider.ResolveInput(context.Background(), port.InputRef{ModelRef: port.ModelRef{Algorithm: string(modelcatalog.AlgorithmBrief2)}})
				return err
			},
		},
		{
			name: "cognitive",
			resolve: func() error {
				provider := evaluationinputinfra.NewCognitiveModelInputProvider(modelcatalog.AlgorithmSPM, invalidCognitiveCatalog{err: invalidNorm}, nil, nil, nil, nil)
				_, err := provider.ResolveInput(context.Background(), port.InputRef{ModelRef: port.ModelRef{Algorithm: string(modelcatalog.AlgorithmSPM)}})
				return err
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resolve()
			kind, ok := calcnorm.ErrorKindOf(err)
			if !ok || kind != calcnorm.ErrorKindInvalid {
				t.Fatalf("error = %T %v, want norm_invalid", err, err)
			}
			var retryable port.RetryableCarrier
			if errors.As(err, &retryable) && retryable.Retryable() {
				t.Fatalf("invalid norm became retryable dependency: %v", err)
			}
		})
	}
}
