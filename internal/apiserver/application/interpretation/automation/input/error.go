package input

import (
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
)

type AdmissionError struct {
	Kind admission.Kind
	Err  error
}

func (e *AdmissionError) Error() string {
	if e == nil || e.Err == nil {
		return "interpretation input admission failed"
	}
	return e.Err.Error()
}

func (e *AdmissionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func classify(kind admission.Kind, err error, format string, args ...interface{}) error {
	if err == nil {
		err = errors.New(fmt.Sprintf(format, args...))
	} else if format != "" {
		err = fmt.Errorf(format+": %w", append(args, err)...)
	}
	return &AdmissionError{Kind: kind, Err: err}
}
