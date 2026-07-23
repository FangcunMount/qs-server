package evaluationinput

import (
	"context"
	"errors"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type canonicalReaderStub struct {
	err error
}

func (s canonicalReaderStub) GetPublishedModelByRef(context.Context, rulesetport.Ref) (*rulesetport.PublishedModel, error) {
	return nil, s.err
}

func (s canonicalReaderStub) FindPublishedModelByQuestionnaire(context.Context, string, string) (*rulesetport.PublishedModel, error) {
	return nil, s.err
}

func assertModelCatalogDependency(t *testing.T, err error) {
	t.Helper()
	var failure port.FailureKindCarrier
	if !errors.As(err, &failure) || failure.FailureKind() != port.FailureKindDependencyUnavailable {
		t.Fatalf("error = %T %v, want dependency_unavailable", err, err)
	}
	var retryable port.RetryableCarrier
	if !errors.As(err, &retryable) || !retryable.Retryable() {
		t.Fatalf("error = %T %v, want retryable", err, err)
	}
	var category port.DependencyCategoryCarrier
	if !errors.As(err, &category) || category.DependencyCategory() != port.DependencyCategoryModelCatalog {
		t.Fatalf("error = %T %v, want modelcatalog dependency", err, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want wrapped deadline", err)
	}
}

func TestCanonicalAttachClassifiesAllModelCatalogFailuresAsRetryable(t *testing.T) {
	t.Parallel()

	reader := canonicalReaderStub{err: context.DeadlineExceeded}
	ref := port.InputRef{ModelRef: port.ModelRef{
		Algorithm: string(domain.AlgorithmBrief2),
		Code:      "MODEL-1",
		Version:   "1.0.0",
	}}
	tests := []struct {
		name   string
		attach func() error
	}{
		{name: "scale", attach: func() error {
			return attachScaleCanonical(context.Background(), reader, ref, &port.InputSnapshot{})
		}},
		{name: "typology", attach: func() error {
			return attachTypologyCanonical(context.Background(), reader, ref, domain.AlgorithmPersonalityTypology, &port.InputSnapshot{})
		}},
		{name: "behavioral", attach: func() error {
			return attachBehavioralCanonical(context.Background(), reader, ref, &port.InputSnapshot{})
		}},
		{name: "cognitive", attach: func() error {
			return attachCognitiveCanonical(context.Background(), reader, ref, &port.InputSnapshot{})
		}},
	}
	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertModelCatalogDependency(t, tc.attach())
		})
	}
}

func TestCanonicalAttachKeepsNotFoundAsTerminalSemanticAbsence(t *testing.T) {
	t.Parallel()

	err := attachScaleCanonical(
		context.Background(),
		canonicalReaderStub{err: domain.ErrNotFound},
		port.InputRef{ModelRef: port.ModelRef{Code: "MODEL-1", Version: "1.0.0"}},
		&port.InputSnapshot{},
	)
	if err != nil {
		t.Fatalf("not found error = %v, want semantic absence to continue", err)
	}
}
