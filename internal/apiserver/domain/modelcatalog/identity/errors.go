package identity

import stderrors "errors"

// ErrInvalidArgument 标记无效 身份 或 product-channel input。
var ErrInvalidArgument = stderrors.New("assessment model invalid argument")
