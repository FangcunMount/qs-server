package service

import (
	"testing"
)

func TestDecodeAnswerValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		qType   string
		raw     string
		want    interface{}
		wantErr bool
	}{
		{name: "checkbox json", qType: questionTypeCheckbox, raw: `["A","B"]`, want: []string{"A", "B"}},
		{name: "checkbox blank", qType: questionTypeCheckbox, raw: ``, want: []string{}},
		{name: "number json", qType: questionTypeNumber, raw: `12`, want: float64(12)},
		{name: "number string", qType: questionTypeNumber, raw: `"12.5"`, want: float64(12.5)},
		{name: "text raw", qType: "Text", raw: `hello`, want: "hello"},
		{name: "radio option wrapper", qType: "Radio", raw: `{"option":"5"}`, want: "5"},
		{name: "number invalid", qType: questionTypeNumber, raw: `abc`, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := decodeAnswerValue(tc.qType, tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeAnswerValue returned error: %v", err)
			}
			switch want := tc.want.(type) {
			case []string:
				gotSlice, ok := got.([]string)
				if !ok || len(gotSlice) != len(want) {
					t.Fatalf("got %#v, want %#v", got, want)
				}
				for i := range want {
					if gotSlice[i] != want[i] {
						t.Fatalf("got %#v, want %#v", gotSlice, want)
					}
				}
			default:
				if got != tc.want {
					t.Fatalf("got %#v, want %#v", got, tc.want)
				}
			}
		})
	}
}

func TestValueToString(t *testing.T) {
	t.Parallel()

	svc := &AnswerSheetService{}
	if got := svc.valueToString(nil); got != "" {
		t.Fatalf("nil value encoded as %q", got)
	}
	if got := svc.valueToString(12); got != "12" {
		t.Fatalf("int encoded as %q", got)
	}
	if got := svc.valueToString([]string{"A", "B"}); got != "[A B]" {
		t.Fatalf("slice encoded as %q", got)
	}
}
