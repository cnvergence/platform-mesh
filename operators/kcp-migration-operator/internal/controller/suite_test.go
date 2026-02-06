package controller

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/platform-mesh/golang-commons/logger"
	migrationv1alpha1 "github.com/platform-mesh/kcp-migration-operator/api/v1alpha1"
	"github.com/platform-mesh/kcp-migration-operator/internal/config"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	scheme    = runtime.NewScheme()
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	// Setup scheme
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(migrationv1alpha1.AddToScheme(scheme))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create a manager to register our controller
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server in tests
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Create logger for tests
	logCfg := logger.DefaultConfig()
	logCfg.Level = "debug"
	log, err := logger.New(logCfg)
	Expect(err).NotTo(HaveOccurred())

	// Setup the KCPMigration controller
	operatorCfg := &config.OperatorConfig{}
	reconciler := NewKCPMigrationReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		log,
		operatorCfg,
	)
	err = reconciler.SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	// Start the manager in a goroutine
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
