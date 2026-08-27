package artifact

import "errors"

var (
	ErrNotFound      = errors.New("AI explanation artifact not found")
	ErrAlreadyExists = errors.New("AI explanation artifact already exists")
)
