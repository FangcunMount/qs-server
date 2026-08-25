package code

const ErrAuthorizationUnavailable = 103001

func init() {
	register(ErrAuthorizationUnavailable, 503, "Authorization temporarily unavailable")
}
