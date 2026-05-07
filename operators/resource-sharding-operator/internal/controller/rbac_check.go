package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func CheckTargetPermissions(ctx context.Context, c client.Client, gvr schema.GroupVersionResource) error {
	requiredVerbs := []string{"get", "list", "watch", "patch"}

	for _, verb := range requiredVerbs {
		sar := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Group:    gvr.Group,
					Resource: gvr.Resource,
					Verb:     verb,
				},
			},
		}

		if err := c.Create(ctx, sar); err != nil {
			return fmt.Errorf("creating SSAR for verb %q: %w", verb, err)
		}

		if !sar.Status.Allowed {
			return fmt.Errorf("missing permission: %s on %s", verb, gvr.String())
		}
	}

	return nil
}
