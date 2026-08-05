package actor

import (
	"reflect"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

func TestTesteePODoesNotMapDeprecatedAssessmentSnapshotColumns(t *testing.T) {
	poType := reflect.TypeOf(TesteePO{})
	for _, fieldName := range []string{"TotalAssessments", "LastAssessmentAt", "LastRiskLevel"} {
		if _, ok := poType.FieldByName(fieldName); ok {
			t.Fatalf("TesteePO still maps deprecated assessment snapshot field %s", fieldName)
		}
	}
}

func TestTesteeMapperCarriesExplicitCreatedAt(t *testing.T) {
	createdAt := time.Date(2025, 1, 1, 8, 42, 0, 0, time.UTC)
	testee := domain.NewTestee(1, "testee", domain.GenderMale, nil)
	testee.SetCreatedAt(createdAt)

	po := NewTesteeMapper().ToPO(testee)
	if !po.CreatedAt.Equal(createdAt) {
		t.Fatalf("PO created_at=%s, want %s", po.CreatedAt, createdAt)
	}
}

func TestTesteeMapperLeavesOrdinaryNewTesteeCreatedAtUnset(t *testing.T) {
	testee := domain.NewTestee(1, "testee", domain.GenderMale, nil)
	if got := NewTesteeMapper().ToPO(testee).CreatedAt; !got.IsZero() {
		t.Fatalf("ordinary PO created_at=%s, want zero for GORM autoCreateTime", got)
	}
}
