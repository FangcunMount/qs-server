package service

import (
	"testing"
	"time"
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

func TestParseAssessmentListDate(t *testing.T) {
	t.Parallel()

	got, err := parseAssessmentListDate("2026-04-22", true)
	if err != nil {
		t.Fatalf("parseAssessmentListDate returned error: %v", err)
	}
	if got == nil || got.Format("2006-01-02") != "2026-04-23" {
		t.Fatalf("unexpected end-exclusive date: %v", got)
	}

	got, err = parseAssessmentListDate(time.Date(2026, 4, 22, 8, 0, 0, 0, time.UTC).Format(time.RFC3339), false)
	if err != nil || got == nil {
		t.Fatalf("expected RFC3339 date to parse, got %v, %v", got, err)
	}
	if _, err := parseAssessmentListDate("bad-date", false); err == nil {
		t.Fatal("expected invalid date error")
	}
}
