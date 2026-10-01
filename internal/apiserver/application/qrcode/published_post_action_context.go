package qrcode

import "context"

type publishedPostActionKey struct{}

// WithPublishedPostAction identifies automatic QR work after publication.
// Direct generation and operator recovery do not carry this marker.
func WithPublishedPostAction(ctx context.Context) context.Context {
	return context.WithValue(ctx, publishedPostActionKey{}, true)
}

func IsPublishedPostAction(ctx context.Context) bool {
	marked, _ := ctx.Value(publishedPostActionKey{}).(bool)
	return marked
}
