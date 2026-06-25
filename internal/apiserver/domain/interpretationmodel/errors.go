package interpretationmodel

import stderrors "errors"

var ErrNotFound = stderrors.New("interpretation model not found")

func IsNotFound(err error) bool {
	return stderrors.Is(err, ErrNotFound)
}
