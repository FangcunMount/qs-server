package report

import "errors"

var (
	ErrInterpretReportNotFound      = errors.New("interpretation report not found")
	ErrInterpretReportAlreadyExists = errors.New("interpretation report already exists")
)
