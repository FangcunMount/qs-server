package reporttemplate

import "errors"

var (
	ErrNotFound      = errors.New("report template not found")
	ErrAlreadyExists = errors.New("report template already exists")
)
