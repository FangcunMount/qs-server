package aliyunoss

import "testing"

func TestNormalizeObjectKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "trims spaces and slashes", input: " /qrcodes/a.png/ ", want: "qrcodes/a.png"},
		{name: "keeps nested path", input: "plans/1/task.png", want: "plans/1/task.png"},
		{name: "rejects empty", input: " / ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeObjectKey(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalized key = %q, want %q", got, tt.want)
			}
		})
	}
}
