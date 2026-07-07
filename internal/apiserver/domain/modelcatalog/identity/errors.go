package identity

import stderrors "errors"

// ErrInvalidArgument marks invalid identity or product-channel input.
var ErrInvalidArgument = stderrors.New("assessment model invalid argument")
