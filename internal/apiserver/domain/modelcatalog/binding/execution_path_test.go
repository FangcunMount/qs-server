package binding

import "testing"

func TestFamilyCapabilityExecutionPathMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind Kind
		want ExecutionPath
	}{
		{KindScale, ExecutionPathScaleDescriptor},
		{KindTypology, ExecutionPathTypologyDescriptor},
		{KindBehavioralRating, ExecutionPathBehavioralRatingDescriptor},
		{KindCognitive, ExecutionPathCognitiveDescriptor},
	}
	for _, tc := range tests {
		got, ok := FamilyCapabilityByKind(tc.kind)
		if !ok {
			t.Fatalf("FamilyCapabilityByKind(%s) ok = false", tc.kind)
		}
		if got.ExecutionPath != tc.want {
			t.Fatalf("FamilyCapabilityByKind(%s).ExecutionPath = %q, want %q", tc.kind, got.ExecutionPath, tc.want)
		}
	}
}
