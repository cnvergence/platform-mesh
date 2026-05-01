package controller

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/platform-mesh/resource-sharding-operator/api/v1alpha1"
)

const (
	testTimeout  = 30 * time.Second
	testInterval = 250 * time.Millisecond
)

type ResourceShardingSuite struct {
	suite.Suite
	ctx       context.Context
	cancel    context.CancelFunc
	env       *envtest.Environment
	k8sClient client.Client
	mgr       manager.Manager
	scheme    *runtime.Scheme
}

func TestResourceShardingSuite(t *testing.T) {
	suite.Run(t, new(ResourceShardingSuite))
}

func (s *ResourceShardingSuite) SetupSuite() {
	s.scheme = runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s.scheme))
	utilruntime.Must(v1alpha1.AddToScheme(s.scheme))

	s.env = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd"),
		},
		BinaryAssetsDirectory: filepath.Join("..", "..", "bin", "k8s", "1.29.0-darwin-arm64"),
	}

	cfg, err := s.env.Start()
	s.Require().NoError(err)

	s.k8sClient, err = client.New(cfg, client.Options{Scheme: s.scheme})
	s.Require().NoError(err)

	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.mgr, err = ctrl.NewManager(cfg, manager.Options{
		Scheme: s.scheme,
	})
	s.Require().NoError(err)

	err = SetupWithManager(s.mgr, nil)
	s.Require().NoError(err)

	go func() {
		err := s.mgr.Start(s.ctx)
		s.Require().NoError(err)
	}()
}

func (s *ResourceShardingSuite) TearDownSuite() {
	s.cancel()
	err := s.env.Stop()
	s.Require().NoError(err)
}
