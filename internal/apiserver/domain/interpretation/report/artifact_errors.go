package report

import "errors"

var (
	ErrArtifactNotFound      = errors.New("interpretation report artifact not found")
	ErrArtifactAlreadyExists = errors.New("interpretation report artifact already exists")
)
