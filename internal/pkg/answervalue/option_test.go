package answervalue

import "testing"

func TestNormalizeSingleOption(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  any
		want string
		ok   bool
	}{
		{name: "plain code", raw: "5", want: "5", ok: true},
		{name: "quoted code", raw: `"5"`, want: "5", ok: true},
		{name: "option wrapper json", raw: `{"option":"5"}`, want: "5", ok: true},
		{name: "option wrapper map", raw: map[string]any{"option": "A"}, want: "A", ok: true},
		{name: "empty", raw: "", want: "", ok: false},
		{name: "int value", raw: int(2), want: "2", ok: true},
		{name: "float value", raw: float64(2), want: "2", ok: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := NormalizeSingleOption(tc.raw)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
