package profile

import "errors"

var (
	ErrNotFound          = errors.New("AI explanation profile not found")
	ErrAlreadyExists     = errors.New("AI explanation profile already exists")
	ErrConflict          = errors.New("AI explanation profile state conflict")
	ErrAmbiguousSelector = errors.New("AI explanation profile selector is ambiguous")
)
