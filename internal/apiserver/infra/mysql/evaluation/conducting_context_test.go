package evaluation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"testing"
	"time"
)

func TestConductingContextRoundtripAndCorruption(t *testing.T) {
	store := uint64(10)
	c, err := assessment.NewConductingContext(99, time.Now().UTC().Truncate(time.Millisecond), &store, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := decodeConductingContext(encodeConductingContext(c))
	if err != nil || !restored.Equal(c) {
		t.Fatal("frozen context changed")
	}
	legacy, err := decodeConductingContext(nil)
	if err != nil || legacy.ID() != 0 {
		t.Fatal("legacy incompatible")
	}
	for _, text := range []string{`null`, `{}`, `{"id":99,"version":2}`, `invalid`} {
		if _, err := decodeConductingContext(&text); err == nil {
			t.Fatal("corruption became unknown")
		}
	}
}
