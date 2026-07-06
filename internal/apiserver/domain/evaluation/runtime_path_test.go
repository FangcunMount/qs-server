package evaluation_test

import (
	"testing"

	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestExecutionPathForDescriptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desc evaldomain.ModelDescriptor
		want modelcatalog.ExecutionPath
	}{
		{desc: evaldomain.ScaleModelDescriptor(), want: modelcatalog.ExecutionPathScaleDescriptor},
		{
			desc: evaldomain.ModelDescriptor{
				Key:  evaldomain.EvaluatorKeyPersonalityTypology,
				Kind: evaldomain.ModelKindTypology,
			},
			want: modelcatalog.ExecutionPathTypologyDescriptor,
		},
		{desc: evaldomain.BehavioralRatingModelDescriptor(), want: modelcatalog.ExecutionPathBehavioralRatingDescriptor},
	}
	for _, tc := range tests {
		got, err := evaldomain.ExecutionPathForDescriptor(tc.desc)
		if err != nil {
			t.Fatalf("ExecutionPathForDescriptor(%#v): %v", tc.desc, err)
		}
		if got != tc.want {
			t.Fatalf("ExecutionPathForDescriptor(%#v) = %q, want %q", tc.desc, got, tc.want)
		}
	}
}
