package interpretengine

import "errors"

var (
	errNoInterpretRules = errors.New("no interpret rules provided")
	errInvalidConfig    = errors.New("invalid interpret config")
)
