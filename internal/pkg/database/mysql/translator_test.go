package mysql

import (
	"errors"
	"testing"

	perrors "github.com/FangcunMount/component-base/pkg/errors"
	mysqldriver "github.com/go-sql-driver/mysql"
	pq "github.com/lib/pq"
)

func TestIsDuplicateError_DriverAndMessageCases(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "sqlite message", err: errors.New("UNIQUE constraint failed: table.col"), want: true},
		{name: "mysql code 1062", err: &mysqldriver.MySQLError{Number: 1062, Message: "Duplicate entry"}, want: true},
		{name: "postgres 23505", err: &pq.Error{Code: pq.ErrorCode("23505"), Message: "unique violation"}, want: true},
		{name: "non-duplicate error", err: errors.New("some other error"), want: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := IsDuplicateError(c.err)
			if got != c.want {
				t.Fatalf("IsDuplicateError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestNewDuplicateToTranslator_MapsOnlyDuplicates(t *testing.T) {
	mapper := func(err error) error {
		return perrors.WithCode(102200, "mapped duplicate")
	}

	trans := NewDuplicateToTranslator(mapper)

	// duplicate case
	dupErr := errors.New("UNIQUE constraint failed: table.col")
	mapped := trans(dupErr)
	if !perrors.IsCode(mapped, 102200) {
		t.Fatalf("expected mapped error to have code 102200, got: %v", mapped)
	}

	// non-duplicate case should return original error
	orig := errors.New("something else")
	out := trans(orig)
	if out != orig {
		t.Fatalf("expected non-duplicate error to be returned unchanged, got: %v", out)
	}
}
