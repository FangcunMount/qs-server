package mysql

import (
	"errors"
	"strings"

	gosql "database/sql"

	perrors "github.com/FangcunMount/component-base/pkg/errors"
	mysqldriver "github.com/go-sql-driver/mysql"
	pq "github.com/lib/pq"
)

// ErrDuplicate signals a DB-level uniqueness/duplicate-entry violation.
var ErrDuplicate = errors.New("duplicate record")

// IsDuplicateError attempts to detect whether the provided error is caused by
// a unique-constraint / duplicate-entry violation. It is driver-aware but
// also falls back to message substring checks for SQLite and unknown drivers.
func IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}

	// Unwrap perrors if wrapped
	if perrors.IsCode(err, 0) {
		// perrors doesn't provide a direct unwrap; fallthrough to string check
	}

	switch e := err.(type) {
	case *mysqldriver.MySQLError:
		// MySQL duplicate entry error number
		if e.Number == 1062 {
			return true
		}
	case *pq.Error:
		// Postgres unique_violation
		if string(e.Code) == "23505" {
			return true
		}
	}

	// Check for known driver error types by string matching on the error text
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint failed") || strings.Contains(msg, "uniqueindex") || strings.Contains(msg, "duplicate entry") || strings.Contains(msg, "unique") {
		return true
	}

	// database/sql wrapped errors may contain driver errors. Try to unwrap common wrappers.
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		return IsDuplicateError(unwrapped)
	}

	// As a last resort, check for sql.ErrNoRows-like identity; duplicate is not that,
	// so return false.
	if errors.Is(err, gosql.ErrNoRows) {
		return false
	}

	return false
}

// NewDuplicateToTranslator returns a translator function that maps DB-level
// duplicate errors (detected by IsDuplicateError) into a business-level error
// produced by mapper. Non-duplicate errors are returned unchanged.
func NewDuplicateToTranslator(mapper func(error) error) func(error) error {
	return func(err error) error {
		if IsDuplicateError(err) {
			return mapper(err)
		}
		return err
	}
}
