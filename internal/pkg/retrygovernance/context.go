package retrygovernance

import "context"

type Authorization struct {
	EventID         string
	ExpectedAttempt int
	Origin          AttemptOrigin
	ActionRequestID string
	Mode            string
}

type authorizationContextKey struct{}

func WithAuthorization(ctx context.Context, authorization Authorization) context.Context {
	return context.WithValue(ctx, authorizationContextKey{}, authorization)
}

func AuthorizationFromContext(ctx context.Context) (Authorization, bool) {
	if ctx == nil {
		return Authorization{}, false
	}
	authorization, ok := ctx.Value(authorizationContextKey{}).(Authorization)
	return authorization, ok
}
