package policy_services

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/machinebox/graphql"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	pmcontext "github.com/platform-mesh/golang-commons/context"
	"github.com/platform-mesh/golang-commons/jwt"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/middleware"
)

func createClient(ctx context.Context, iamApiUrl string) *graphql.Client {
	log := logger.LoadLoggerFromContext(ctx)

	hc := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	client := graphql.NewClient(iamApiUrl, graphql.WithHTTPClient(hc))

	if log != nil {
		client.Log = func(msg string) {
			log.ComponentLogger("graphql").Trace().Msg(msg)
		}
	}
	return client
}

func run(ctx context.Context, client GraphqlClient, request *graphql.Request, resp interface{}, timeout time.Duration) error {
	auth, err := pmcontext.GetAuthHeaderFromContext(ctx)
	if err != nil || len(auth) == 0 {
		return fmt.Errorf("the request context does not contain an auth header under the key %q. You can use authz.context to set it", jwt.AuthHeaderCtxKey)
	}
	request.Header.Add(middleware.AuthorizationHeader, auth)
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return client.Run(requestCtx, request, resp)
}
