package generation

import "errors"

var (
	ErrNotFound        = errors.New("report generation not found")
	ErrAlreadyExists   = errors.New("report generation already exists")
	ErrVersionConflict = errors.New("report generation version conflict")
)
