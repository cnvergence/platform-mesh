package sentry

import (
	"context"

	"github.com/platform-mesh/golang-commons/context/keys"
)

func GetSentryTagsFromContext(ctx context.Context) map[string]string {
	return ctx.Value(keys.SentryTagsCtxKey).(map[string]string)
}
func ContextWithSentryTags(ctx context.Context, sentryTags map[string]string) context.Context {
	return context.WithValue(ctx, keys.SentryTagsCtxKey, sentryTags)
}
