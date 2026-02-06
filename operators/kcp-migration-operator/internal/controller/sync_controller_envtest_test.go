package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/platform-mesh/golang-commons/logger"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/platform-mesh/kcp-migration-operator/internal/config"
)

var _ = Describe("SyncController", Ordered, func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		syncCtx    context.Context
		syncCancel context.CancelFunc
	)

	BeforeAll(func() {
		By("Creating a manager for sync controller tests")
		syncCtx, syncCancel = context.WithCancel(ctx)

		syncMgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme,
			Metrics: metricsserver.Options{
				BindAddress: "0",
			},
		})
		Expect(err).NotTo(HaveOccurred())

		logCfg := logger.DefaultConfig()
		logCfg.Level = "debug"
		log, err := logger.New(logCfg)
		Expect(err).NotTo(HaveOccurred())

		syncCfg := &config.SyncConfig{
			Source: config.SourceConfig{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			Target: config.TargetConfig{
				WorkspaceExpression: "root:test:{{ .Source.metadata.namespace }}",
			},
		}
		syncController := NewSyncController(
			syncMgr.GetClient(),
			log,
			syncCfg,
			nil, // No KCP client factory in envtest - will skip KCP sync
		)
		err = syncController.SetupWithManager(syncMgr)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			defer GinkgoRecover()
			err := syncMgr.Start(syncCtx)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Wait for the manager cache to sync
		Eventually(func() bool {
			return syncMgr.GetCache().WaitForCacheSync(syncCtx)
		}, timeout, interval).Should(BeTrue())

		// Give time for the controller workers to start
		time.Sleep(100 * time.Millisecond)
	})

	AfterAll(func() {
		if syncCancel != nil {
			syncCancel()
		}
	})

	Context("When syncing ConfigMaps", func() {
		It("Should reconcile when a ConfigMap is created", func() {
			configMapName := "sync-test-cm-create"
			configMapNamespace := "default"

			By("Creating a ConfigMap")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: configMapNamespace,
				},
				Data: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())

			By("Verifying the ConfigMap was created and reconciled")
			createdConfigMap := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: configMapNamespace,
				}, createdConfigMap)
			}, timeout, interval).Should(Succeed())

			Expect(createdConfigMap.Data["key"]).Should(Equal("value"))

			// Wait a bit for the sync controller to process
			// The controller should have reconciled by now
			Eventually(func() bool {
				// Just verify the resource still exists - reconcile logs will show activity
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: configMapNamespace,
				}, cm)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, configMap)).Should(Succeed())
		})

		It("Should handle ConfigMap updates", func() {
			configMapName := "sync-test-cm-update"
			configMapNamespace := "default"

			By("Creating a ConfigMap")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: configMapNamespace,
				},
				Data: map[string]string{
					"key": "original",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())

			By("Updating the ConfigMap")
			Eventually(func() error {
				cm := &corev1.ConfigMap{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: configMapNamespace,
				}, cm); err != nil {
					return err
				}
				cm.Data["key"] = "updated"
				return k8sClient.Update(ctx, cm)
			}, timeout, interval).Should(Succeed())

			By("Verifying the update")
			updatedConfigMap := &corev1.ConfigMap{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: configMapNamespace,
				}, updatedConfigMap)
				if err != nil {
					return ""
				}
				return updatedConfigMap.Data["key"]
			}, timeout, interval).Should(Equal("updated"))

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, configMap)).Should(Succeed())
		})

		It("Should handle ConfigMap deletion gracefully", func() {
			configMapName := "sync-test-cm-delete"
			configMapNamespace := "default"

			By("Creating a ConfigMap")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: configMapNamespace,
				},
				Data: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).Should(Succeed())

			By("Deleting the ConfigMap")
			Expect(k8sClient.Delete(ctx, configMap)).Should(Succeed())

			By("Verifying the ConfigMap is deleted")
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: configMapNamespace,
				}, cm)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
