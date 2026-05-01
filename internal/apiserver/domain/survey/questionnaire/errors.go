package questionnaire

import (
	stderrors "errors"
	"fmt"
)

// ErrorKind classifies questionnaire domain failures without depending on API error codes.
type ErrorKind string

const (
	ErrorKindInvalidCode      ErrorKind = "invalid_code"
	ErrorKindInvalidTitle     ErrorKind = "invalid_title"
	ErrorKindInvalidInput     ErrorKind = "invalid_input"
	ErrorKindInvalidQuestion  ErrorKind = "invalid_question"
	ErrorKindQuestionExists   ErrorKind = "question_exists"
	ErrorKindQuestionNotFound ErrorKind = "question_not_found"
	ErrorKindArchived         ErrorKind = "archived"
	ErrorKindInvalidStatus    ErrorKind = "invalid_status"
	ErrorKindOptionEmpty      ErrorKind = "option_empty"
)

// DomainError is a domain-native error that application services map to API codes.
type DomainError struct {
	kind    ErrorKind
	message string
	cause   error
}

func newError(kind ErrorKind, format string, args ...interface{}) error {
	return &DomainError{kind: kind, message: fmt.Sprintf(format, args...)}
}

func wrapError(kind ErrorKind, err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return &DomainError{kind: kind, message: fmt.Sprintf(format, args...), cause: err}
}

func (e *DomainError) Error() string {
	if e == nil {
		return ""
	}
	if e.cause == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.cause)
}

func (e *DomainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *DomainError) Kind() ErrorKind {
	if e == nil {
		return ""
	}
	return e.kind
}

// ErrorKindOf returns the first questionnaire domain error kind in an error chain.
func ErrorKindOf(err error) (ErrorKind, bool) {
	var domainErr *DomainError
	if !stderrors.As(err, &domainErr) {
		return "", false
	}
	return domainErr.kind, true
}
