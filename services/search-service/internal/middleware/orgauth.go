package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"

	pmcontext "github.com/platform-mesh/golang-commons/context"

	appcontext "github.com/platform-mesh/search/internal/context"
	"github.com/platform-mesh/search/internal/service/search"
)

var issuerRegex = regexp.MustCompile(`^.*\/realms\/(.*?)\/?$`)

type OrgContextMiddleware struct {
	validator search.OrgAccessValidator
}

func NewOrgContextMiddleware(validator search.OrgAccessValidator) *OrgContextMiddleware {
	return &OrgContextMiddleware{validator: validator}
}

func (m *OrgContextMiddleware) SetRequestContext() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			org := extractSubdomain(r.Host)
			if org == "" {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			token, err := pmcontext.GetWebTokenFromContext(ctx)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}
			authHeader, err := pmcontext.GetAuthHeaderFromContext(ctx)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			allowed, err := m.validator.ValidateTokenForOrg(ctx, authHeader, org)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			user := strings.TrimSpace(token.Mail)
			if user == "" {
				user = strings.TrimSpace(token.Subject)
			}
			if user == "" {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			tenant, err := extractTenant(token.Issuer)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			rc := appcontext.RequestContext{Organization: org, User: user, IDMTenant: tenant}
			ctx = appcontext.WithRequestContext(ctx, rc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractSubdomain(host string) string {
	if host == "" {
		return ""
	}

	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else {
		host = strings.Split(host, ":")[0]
	}

	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func extractTenant(issuer string) (string, error) {
	match := issuerRegex.FindStringSubmatch(issuer)
	if len(match) < 2 || match[1] == "" {
		return "", fmt.Errorf("invalid issuer")
	}
	return match[1], nil
}

func InjectRequestContext(ctx context.Context, rc appcontext.RequestContext) context.Context {
	return appcontext.WithRequestContext(ctx, rc)
}
