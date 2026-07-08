package report

import "errors"

var (
	ErrReportNotFound      = errors.New("report not found")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrReportAlreadyExists = errors.New("report already exists")
)
