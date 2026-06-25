package ruleset

import stderrors "errors"

var (
	ErrNotFound         = stderrors.New("ruleset not found")
	ErrVersionRequired  = stderrors.New("ruleset version is required")
	ErrAmbiguousVersion = stderrors.New("multiple published rulesets matched")
)

func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}

func IsVersionRequired(err error) bool {
	return stderrors.Is(err, ErrVersionRequired)
}
