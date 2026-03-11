package context

import (
	"context"
	"errors"
)

type key string

const requestContextKey key = "request-context"

type RequestContext struct {
	Organization string
	User         string
	IDMTenant    string
}

func WithRequestContext(ctx context.Context, rc RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey, rc)
}

func GetRequestContext(ctx context.Context) (RequestContext, error) {
	v := ctx.Value(requestContextKey)
	if v == nil {
		return RequestContext{}, errors.New("request context missing")
	}
	rc, ok := v.(RequestContext)
	if !ok {
		return RequestContext{}, errors.New("request context has invalid type")
	}
	if rc.Organization == "" || rc.User == "" {
		return RequestContext{}, errors.New("request context is incomplete")
	}
	return rc, nil
}
