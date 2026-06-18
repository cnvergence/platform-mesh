package client

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var defaultBackoff = wait.Backoff{Duration: 500 * time.Millisecond, Factor: 2, Steps: 5}

type UpdateFunc func(client.Object) client.Object

func RetryStatusUpdate(ctx context.Context, adjust UpdateFunc, obj client.Object, cl client.Client) error {
	operation := func() error {
		err := cl.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return err
		}
		return cl.Status().Update(ctx, adjust(obj))
	}
	return retry.RetryOnConflict(defaultBackoff, operation)
}

func RetryUpdate(ctx context.Context, adjust UpdateFunc, obj client.Object, cl client.Client) error {
	operation := func() error {
		err := cl.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if err != nil {
			return err
		}
		return cl.Update(ctx, adjust(obj))
	}
	return retry.RetryOnConflict(defaultBackoff, operation)
}
