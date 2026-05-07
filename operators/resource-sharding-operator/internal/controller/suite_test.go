package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/platform-mesh/resource-sharding-operator/api/v1alpha1"
	"github.com/stretchr/testify/suite"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
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
	scheme    *k8sruntime.Scheme
	mgrErr    chan error
}

func TestResourceShardingSuite(t *testing.T) {
	suite.Run(t, new(ResourceShardingSuite))
}

func (s *ResourceShardingSuite) SetupSuite() {
	s.scheme = k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s.scheme))
	utilruntime.Must(v1alpha1.AddToScheme(s.scheme))

	assetsDir := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsDir == "" {
		assetsDir = filepath.Join("..", "..", "bin", "k8s", fmt.Sprintf("1.29.0-%s-%s", runtime.GOOS, runtime.GOARCH))
	}

	s.env = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd"),
		},
		BinaryAssetsDirectory: assetsDir,
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

	err = SetupWithManager(s.mgr)
	s.Require().NoError(err)

	s.mgrErr = make(chan error, 1)
	go func() { s.mgrErr <- s.mgr.Start(s.ctx) }()
}

func (s *ResourceShardingSuite) TearDownSuite() {
	s.cancel()
	if err := <-s.mgrErr; err != nil && err != context.Canceled {
		s.NoError(err)
	}
	err := s.env.Stop()
	s.Require().NoError(err)
}
