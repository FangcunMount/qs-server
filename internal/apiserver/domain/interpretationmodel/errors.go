package interpretationmodel

import stderrors "errors"

var (
	ErrNotFound         = stderrors.New("interpretation model not found")
	ErrVersionRequired  = stderrors.New("model version is required")
	ErrAmbiguousVersion = stderrors.New("multiple published models matched")
)

func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

func IsVersionRequired(err error) bool {
	return stderrors.Is(err, ErrVersionRequired)
}
