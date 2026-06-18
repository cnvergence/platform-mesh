package directive

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	pmpcontext "github.com/platform-mesh/golang-commons/context"
	"github.com/platform-mesh/golang-commons/logger"
)

func setTenantToContextForTechnicalUsers(ctx context.Context, l *logger.Logger) (context.Context, error) {
	spiffee, err := pmpcontext.GetSpiffeFromContext(ctx)
	hasSpiffee := err == nil && spiffee != ""
	if isTechnicalIssuer := pmpcontext.GetIsTechnicalIssuerFromContext(ctx); !isTechnicalIssuer && !hasSpiffee {
		return ctx, nil
	}

	fieldContext := graphql.GetFieldContext(ctx)
	var tenantID string
	switch tID := fieldContext.Args["tenantId"].(type) {
	case string:
		tenantID = tID
	case *string:
		if tID == nil {
			return nil, &gqlerror.Error{Message: "tenantId parameter is nil - bad request"}
		}
		tenantID = *tID
	}

	if tenantID == "" {
		return ctx, nil
	}

	ctx = pmpcontext.AddTenantToContext(ctx, tenantID)
	l.Debug().Str("tenantId", tenantID).Msg("Added a tenant id for technical user to the context")
	return ctx, nil
}
