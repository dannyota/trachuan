package oauth

import "errors"

// ErrScopeInvalid indicates an unsupported scope parameter.
var ErrScopeInvalid = errors.New("oauth scope invalid")
