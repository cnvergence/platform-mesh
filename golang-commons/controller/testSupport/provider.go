package testSupport

import (
	"context"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

type FakeProvider struct {
	cfg *rest.Config
}

func NewFakeProvider(cfg *rest.Config) *FakeProvider {
	return &FakeProvider{cfg: cfg}
}

func (f FakeProvider) Get(context.Context, string) (cluster.Cluster, error) {
	return cluster.New(f.cfg, nil)
}

// IndexField indexes the given object by the given field on all engaged
// clusters, current and future.
func (f FakeProvider) IndexField(context.Context, client.Object, string, client.IndexerFunc) error {
	return nil
}
