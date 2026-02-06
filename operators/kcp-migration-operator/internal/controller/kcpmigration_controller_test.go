package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	migrationv1alpha1 "github.com/platform-mesh/kcp-migration-operator/api/v1alpha1"
)

var _ = Describe("KCPMigration Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250

		migrationName      = "test-migration"
		migrationNamespace = "default"
	)

	Context("When creating a KCPMigration", func() {
		It("Should update status phase to Running", func() {
			By("Creating a new KCPMigration")
			migration := &migrationv1alpha1.KCPMigration{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "migration.platform-mesh.io/v1alpha1",
					Kind:       "KCPMigration",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      migrationName,
					Namespace: migrationNamespace,
				},
				Spec: migrationv1alpha1.KCPMigrationSpec{
					Source: migrationv1alpha1.SourceSpec{
						APIVersion: "apps.example.com/v1",
						Kind:       "MyApp",
					},
					Transform: migrationv1alpha1.TransformSpec{
						TargetWorkspace: migrationv1alpha1.WorkspaceExpression{
							Expression: "root:platform-mesh:{{ .Source.metadata.namespace }}",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, migration)).Should(Succeed())

			By("Checking that the KCPMigration status is updated to Running")
			migrationLookupKey := types.NamespacedName{Name: migrationName, Namespace: migrationNamespace}
			createdMigration := &migrationv1alpha1.KCPMigration{}

			// Wait for the controller to reconcile and update status to Running
			Eventually(func() migrationv1alpha1.Phase {
				err := k8sClient.Get(ctx, migrationLookupKey, createdMigration)
				if err != nil {
					return ""
				}
				return createdMigration.Status.Phase
			}, timeout, interval).Should(Equal(migrationv1alpha1.PhaseRunning))

			By("Verifying observedGeneration is set")
			Expect(createdMigration.Status.ObservedGeneration).Should(Equal(createdMigration.Generation))

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, migration)).Should(Succeed())
		})
	})

	Context("When creating a namespace for testing", func() {
		It("Should reconcile migration in migration-system namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "migration-system",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

			By("Creating migration in migration-system namespace")
			migration := &migrationv1alpha1.KCPMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "system-migration",
					Namespace: "migration-system",
				},
				Spec: migrationv1alpha1.KCPMigrationSpec{
					Source: migrationv1alpha1.SourceSpec{
						APIVersion: "apps.example.com/v1",
						Kind:       "ConfigMap",
					},
					Transform: migrationv1alpha1.TransformSpec{
						TargetWorkspace: migrationv1alpha1.WorkspaceExpression{
							Expression: "root:config",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, migration)).Should(Succeed())

			By("Waiting for status to be updated")
			migrationLookupKey := types.NamespacedName{Name: "system-migration", Namespace: "migration-system"}
			createdMigration := &migrationv1alpha1.KCPMigration{}

			Eventually(func() migrationv1alpha1.Phase {
				err := k8sClient.Get(ctx, migrationLookupKey, createdMigration)
				if err != nil {
					return ""
				}
				return createdMigration.Status.Phase
			}, timeout, interval).Should(Equal(migrationv1alpha1.PhaseRunning))

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, migration)).Should(Succeed())
		})
	})

	Context("When handling deletion", func() {
		It("Should handle not found gracefully", func() {
			By("Getting a non-existent migration")
			migration := &migrationv1alpha1.KCPMigration{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			}, migration)
			Expect(err).To(HaveOccurred())
		})
	})
})
