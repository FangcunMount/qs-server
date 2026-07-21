package eventpayload

import "testing"

func TestClassifyPayloadGate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data EvaluationRequestedData
		want PayloadGateClass
	}{
		{"complete model", EvaluationRequestedData{AssessmentID: 1, ModelCode: "M"}, PayloadGateComplete},
		{"complete legacy scale", EvaluationRequestedData{AssessmentID: 1, ScaleCode: "S"}, PayloadGateComplete},
		{"legacy incomplete", EvaluationRequestedData{AssessmentID: 1}, PayloadGateLegacyIncomplete},
		{"invalid zero id", EvaluationRequestedData{ModelCode: "M"}, PayloadGateInvalid},
		{"invalid negative id", EvaluationRequestedData{AssessmentID: -1, ModelCode: "M"}, PayloadGateInvalid},
	}
	for _, tc := range cases {
		if got := tc.data.ClassifyPayloadGate(); got != tc.want {
			t.Fatalf("%s: gate = %s, want %s", tc.name, got, tc.want)
		}
	}
}
