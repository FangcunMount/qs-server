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
		{name: "empty wrapper", raw: `{"option":""}`, want: `{"option":""}`, ok: true},
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
