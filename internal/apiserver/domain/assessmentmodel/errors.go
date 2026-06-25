package assessmentmodel

import stderrors "errors"

var (
	ErrNotFound         = stderrors.New("assessment model not found")
	ErrVersionRequired  = stderrors.New("assessment model version is required")
	ErrAmbiguousVersion = stderrors.New("multiple published assessment models matched")
)

func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

func IsVersionRequired(err error) bool {
	return stderrors.Is(err, ErrVersionRequired)
}
