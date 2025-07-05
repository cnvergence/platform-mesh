package controllerruntime

import (
	"context"
	goerrors "errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/platform-mesh/golang-commons/controller/lifecycle"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/conditions"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/spread"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
	operrors "github.com/platform-mesh/golang-commons/errors"

	pmtesting "github.com/platform-mesh/golang-commons/controller/testSupport"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/logger/testlogger"
	"github.com/platform-mesh/golang-commons/sentry"
)

func TestLifecycle(t *testing.T) {
	namespace := "bar"
	name := "foo"
	request := controllerruntime.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		},
	}
	testApiObject := &pmtesting.TestApiObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	ctx := context.Background()

	t.Run("Lifecycle with a not found object", func(t *testing.T) {
		// Arrange
		fakeClient := pmtesting.CreateFakeClient(t, &pmtesting.TestApiObject{})

		mgr, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)

		// Act
		result, err := mgr.Reconcile(ctx, request, &pmtesting.TestApiObject{})

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		logMessages, err := log.GetLogMessages()
		assert.NoError(t, err)
		assert.Equal(t, len(logMessages), 2)
		assert.Equal(t, logMessages[0].Message, "start reconcile")
		assert.Contains(t, logMessages[1].Message, "instance not found")
	})

	t.Run("Lifecycle with a finalizer - add finalizer", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FinalizerSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(instance.Finalizers))
	})

	t.Run("Lifecycle with a finalizer - finalization", func(t *testing.T) {
		// Arrange
		now := &metav1.Time{Time: time.Now()}
		finalizers := []string{pmtesting.SubroutineFinalizer}
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        finalizers,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FinalizerSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, 0, len(instance.Finalizers))
	})

	t.Run("Lifecycle with a finalizer - finalization(requeue)", func(t *testing.T) {
		// Arrange
		now := &metav1.Time{Time: time.Now()}
		finalizers := []string{pmtesting.SubroutineFinalizer}
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        finalizers,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FinalizerSubroutine{
				Client:       fakeClient,
				RequeueAfter: 1 * time.Second,
			},
		}, fakeClient)

		// Act
		res, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(instance.Finalizers))
		assert.Equal(t, time.Duration(1*time.Second), res.RequeueAfter)
	})

	t.Run("Lifecycle with a finalizer - finalization(requeueAfter)", func(t *testing.T) {
		// Arrange
		now := &metav1.Time{Time: time.Now()}
		finalizers := []string{pmtesting.SubroutineFinalizer}
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        finalizers,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FinalizerSubroutine{
				Client:       fakeClient,
				RequeueAfter: 2 * time.Second,
			},
		}, fakeClient)

		// Act
		res, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(instance.Finalizers))
		assert.Equal(t, 2*time.Second, res.RequeueAfter)
	})

	t.Run("Lifecycle with a finalizer - skip finalization if the finalizer is not in there", func(t *testing.T) {
		// Arrange
		now := &metav1.Time{Time: time.Now()}
		finalizers := []string{"other-finalizer"}
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        finalizers,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FinalizerSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, 1, len(instance.Finalizers))
	})
	t.Run("Lifecycle with a finalizer - failing finalization subroutine", func(t *testing.T) {
		// Arrange
		now := &metav1.Time{Time: time.Now()}
		finalizers := []string{pmtesting.SubroutineFinalizer}
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         namespace,
				DeletionTimestamp: now,
				Finalizers:        finalizers,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FinalizerSubroutine{
				Client: fakeClient,
				Err:    fmt.Errorf("some error"),
			},
		}, fakeClient)

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.Error(t, err)
		assert.Equal(t, 1, len(instance.Finalizers))
	})

	t.Run("Lifecycle without changing status", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: pmtesting.TestStatus{Some: "string"},
		}
		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)

		// Act
		result, err := mgr.Reconcile(ctx, request, instance)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		logMessages, err := log.GetLogMessages()
		assert.NoError(t, err)
		assert.Equal(t, len(logMessages), 3)
		assert.Equal(t, logMessages[0].Message, "start reconcile")
		assert.Equal(t, logMessages[1].Message, "skipping status update, since they are equal")
		assert.Equal(t, logMessages[2].Message, "end reconcile")
	})

	t.Run("Lifecycle with changing status", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.TestApiObject{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: pmtesting.TestStatus{Some: "string"},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, log := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.ChangeStatusSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)

		// Act
		result, err := mgr.Reconcile(ctx, request, instance)

		// Assert
		assert.NoError(t, err)
		assert.NotNil(t, result)
		logMessages, err := log.GetLogMessages()
		assert.NoError(t, err)
		assert.Equal(t, len(logMessages), 7)
		assert.Equal(t, logMessages[0].Message, "start reconcile")
		assert.Equal(t, logMessages[1].Message, "start subroutine")
		assert.Equal(t, logMessages[2].Message, "processing instance")
		assert.Equal(t, logMessages[3].Message, "processed instance")
		assert.Equal(t, logMessages[4].Message, "end subroutine")

		serverObject := &pmtesting.TestApiObject{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, serverObject)
		assert.NoError(t, err)
		assert.Equal(t, serverObject.Status.Some, "other string")
	})

	t.Run("Lifecycle with spread reconciles", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.ChangeStatusSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, instance.Generation, instance.Status.ObservedGeneration)
	})

	t.Run("Lifecycle with spread reconciles on deleted object", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					Namespace:         namespace,
					Generation:        2,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{pmtesting.ChangeStatusSubroutineFinalizer},
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 2,
					NextReconcileTime:  metav1.Time{Time: time.Now().Add(2 * time.Hour)},
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.ChangeStatusSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)
		assert.NoError(t, err)
		assert.Len(t, instance.Finalizers, 0)

	})

	t.Run("Lifecycle with spread reconciles skips if the generation is the same", func(t *testing.T) {
		// Arrange
		nextReconcileTime := metav1.NewTime(time.Now().Add(1 * time.Hour))
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 1,
					NextReconcileTime:  nextReconcileTime,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{RequeAfter: false}}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		result, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, int64(1), instance.Status.ObservedGeneration)
		assert.GreaterOrEqual(t, 12*time.Hour, result.RequeueAfter)
	})

	t.Run("Lifecycle with spread reconciles and processing fails (no-retry)", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{Retry: false, RequeAfter: false}}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, int64(1), instance.Status.ObservedGeneration)
	})

	t.Run("Lifecycle with spread reconciles and processing fails (retry)", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{Retry: true, RequeAfter: false}}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.Error(t, err)
		assert.Equal(t, int64(0), instance.Status.ObservedGeneration)
	})

	t.Run("Lifecycle with spread reconciles and processing needs requeue", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{RequeAfter: true}}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, int64(0), instance.Status.ObservedGeneration)
	})

	t.Run("Lifecycle with spread reconciles and processing needs requeueAfter", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}
		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{RequeAfter: true}}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, int64(0), instance.Status.ObservedGeneration)
	})

	t.Run("Lifecycle with spread not implementing the interface", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.NotImplementingSpreadReconciles{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  namespace,
				Generation: 1,
			},
			Status: pmtesting.TestStatus{
				Some:               "string",
				ObservedGeneration: 0,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.ChangeStatusSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)
		mgr.WithSpreadingReconciles()

		// Act
		assert.Panics(t, func() {
			_, _ = mgr.Reconcile(ctx, request, instance)
		})
	})

	t.Run("Should setup with manager", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.TestApiObject{}
		fakeClient := pmtesting.CreateFakeClient(t, instance)
		log, err := logger.New(logger.DefaultConfig())
		assert.NoError(t, err)
		m, err := manager.New(&rest.Config{}, manager.Options{
			Scheme: fakeClient.Scheme(),
		})
		assert.NoError(t, err)

		lm, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{RequeAfter: true}}, fakeClient)
		tr := &testReconciler{
			lifecycleManager: lm,
		}

		// Act
		err = lm.SetupWithManager(m, 0, "testReconciler", instance, "test", tr, log)

		// Assert
		assert.NoError(t, err)
	})

	t.Run("Should setup with manager not implementing interface", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.NotImplementingSpreadReconciles{}
		fakeClient := pmtesting.CreateFakeClient(t, instance)
		log, err := logger.New(logger.DefaultConfig())
		assert.NoError(t, err)
		m, err := manager.New(&rest.Config{}, manager.Options{
			Scheme: fakeClient.Scheme(),
		})
		assert.NoError(t, err)

		lm, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{RequeAfter: true}}, fakeClient)
		lm.WithSpreadingReconciles()
		tr := &testReconciler{
			lifecycleManager: lm,
		}

		// Act
		err = lm.SetupWithManager(m, 0, "testReconciler", instance, "test", tr, log)

		// Assert
		assert.Error(t, err)
	})

	t.Run("Lifecycle with spread reconciles and refresh label", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
					Labels:     map[string]string{spread.ReconcileRefreshLabel: "true"},
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 1,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		lm, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.ChangeStatusSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)
		lm.WithSpreadingReconciles()

		// Act
		_, err := lm.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Equal(t, int64(1), instance.Status.ObservedGeneration)

		serverObject := &pmtesting.ImplementingSpreadReconciles{}
		err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, serverObject)
		assert.NoError(t, err)
		assert.Equal(t, serverObject.Status.Some, "other string")
		_, ok := serverObject.Labels[spread.ReconcileRefreshLabel]
		assert.False(t, ok)
	})

	t.Run("Should handle a client error", func(t *testing.T) {
		// Arrange
		_, log := createLifecycleManager([]subroutine.Subroutine{}, nil)
		testErr := fmt.Errorf("test error")

		// Act
		result, err := lifecycle.HandleClientError("test", log.Logger, testErr, true, sentry.Tags{})

		// Assert
		assert.Error(t, err)
		assert.Equal(t, testErr, err)
		assert.Equal(t, controllerruntime.Result{}, result)
	})

	t.Run("Lifecycle with manage conditions reconciles w/o subroutines", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Len(t, instance.Status.Conditions, 1)
		assert.Equal(t, instance.Status.Conditions[0].Type, conditions.ConditionReady)
		assert.Equal(t, instance.Status.Conditions[0].Status, metav1.ConditionTrue)
		assert.Equal(t, instance.Status.Conditions[0].Message, "The resource is ready")
	})

	t.Run("Lifecycle with manage conditions reconciles with subroutine", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.ChangeStatusSubroutine{
			Client: fakeClient,
		}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		require.Len(t, instance.Status.Conditions, 2)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "changeStatus_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine is complete", instance.Status.Conditions[1].Message)
	})

	t.Run("Lifecycle with manage conditions reconciles with subroutine that adds a condition", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.AddConditionSubroutine{Ready: metav1.ConditionTrue}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		require.Len(t, instance.Status.Conditions, 3)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "addCondition_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine is complete", instance.Status.Conditions[1].Message)
		assert.Equal(t, "test", instance.Status.Conditions[2].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[2].Status)
		assert.Equal(t, "test", instance.Status.Conditions[2].Message)

	})

	t.Run("Lifecycle with manage conditions reconciles with subroutine that adds a condition", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.AddConditionSubroutine{Ready: metav1.ConditionTrue}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		require.Len(t, instance.Status.Conditions, 3)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "addCondition_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine is complete", instance.Status.Conditions[1].Message)
		assert.Equal(t, "test", instance.Status.Conditions[2].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[2].Status)
		assert.Equal(t, "test", instance.Status.Conditions[2].Message)

	})

	t.Run("Lifecycle with manage conditions reconciles with subroutine that adds a condition with preexisting conditions (update)", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "test",
							Status:  metav1.ConditionFalse,
							Reason:  "test",
							Message: "test",
						},
					},
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.AddConditionSubroutine{Ready: metav1.ConditionTrue}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		require.Len(t, instance.Status.Conditions, 3)
		assert.Equal(t, "test", instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "test", instance.Status.Conditions[0].Message)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The resource is ready", instance.Status.Conditions[1].Message)
		assert.Equal(t, "addCondition_Ready", instance.Status.Conditions[2].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[2].Status)
		assert.Equal(t, "The subroutine is complete", instance.Status.Conditions[2].Message)

	})

	t.Run("Lifecycle with manage conditions reconciles with subroutine that adds a condition with preexisting conditions", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Conditions: []metav1.Condition{
						{
							Type:    conditions.ConditionReady,
							Status:  metav1.ConditionTrue,
							Message: "The resource is ready!!",
							Reason:  conditions.ConditionReady,
						},
					},
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.AddConditionSubroutine{Ready: metav1.ConditionTrue}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		require.Len(t, instance.Status.Conditions, 3)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "addCondition_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine is complete", instance.Status.Conditions[1].Message)
		assert.Equal(t, "test", instance.Status.Conditions[2].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[2].Status)
		assert.Equal(t, "test", instance.Status.Conditions[2].Message)

	})

	t.Run("Lifecycle w/o manage conditions reconciles with subroutine that adds a condition", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.AddConditionSubroutine{Ready: metav1.ConditionTrue}}, fakeClient)

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		require.Len(t, instance.Status.Conditions, 1)
		assert.Equal(t, "test", instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "test", instance.Status.Conditions[0].Message)

	})

	t.Run("Lifecycle with manage conditions reconciles with subroutine failing Status update", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.ChangeStatusSubroutine{
				Client: fakeClient,
			}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Len(t, instance.Status.Conditions, 2)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "changeStatus_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine is complete", instance.Status.Conditions[1].Message)
	})

	t.Run("Lifecycle with manage conditions finalizes with multiple subroutines partially succeeding", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					Namespace:         namespace,
					Generation:        1,
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{pmtesting.FailureScenarioSubroutineFinalizer, pmtesting.ChangeStatusSubroutineFinalizer},
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FailureScenarioSubroutine{},
			pmtesting.ChangeStatusSubroutine{Client: fakeClient}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.Error(t, err)
		require.Len(t, instance.Status.Conditions, 3)
		assert.Equal(t, "changeStatus_Finalize", instance.Status.Conditions[0].Type, "")
		assert.Equal(t, metav1.ConditionTrue, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The subroutine finalization is complete", instance.Status.Conditions[0].Message)
		assert.Equal(t, "FailureScenarioSubroutine_Finalize", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionFalse, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine finalization has an error: FailureScenarioSubroutine", instance.Status.Conditions[1].Message)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[2].Type)
		assert.Equal(t, metav1.ConditionFalse, instance.Status.Conditions[2].Status)
		assert.Equal(t, "The resource is not ready", instance.Status.Conditions[2].Message)
	})

	t.Run("Lifecycle with manage conditions reconciles with ReqeueAfter subroutine", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FailureScenarioSubroutine{RequeAfter: true}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Len(t, instance.Status.Conditions, 2)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionFalse, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is not ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "FailureScenarioSubroutine_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionUnknown, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine is processing", instance.Status.Conditions[1].Message)
	})

	t.Run("Lifecycle with manage conditions reconciles with Error subroutine (no-retry)", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FailureScenarioSubroutine{RequeAfter: false}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Len(t, instance.Status.Conditions, 2)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionFalse, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is not ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "FailureScenarioSubroutine_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionFalse, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine has an error: FailureScenarioSubroutine", instance.Status.Conditions[1].Message)
	})

	t.Run("Lifecycle with manage conditions reconciles with Error subroutine (retry)", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.FailureScenarioSubroutine{Retry: true, RequeAfter: false}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.Error(t, err)
		assert.Len(t, instance.Status.Conditions, 2)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, metav1.ConditionFalse, instance.Status.Conditions[0].Status)
		assert.Equal(t, "The resource is not ready", instance.Status.Conditions[0].Message)
		assert.Equal(t, "FailureScenarioSubroutine_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, metav1.ConditionFalse, instance.Status.Conditions[1].Status)
		assert.Equal(t, "The subroutine has an error: FailureScenarioSubroutine", instance.Status.Conditions[1].Message)
	})

	t.Run("Lifecycle with manage conditions not implementing the interface", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.NotImplementingSpreadReconciles{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  namespace,
				Generation: 1,
			},
			Status: pmtesting.TestStatus{
				Some:               "string",
				ObservedGeneration: 0,
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{
			pmtesting.ChangeStatusSubroutine{
				Client: fakeClient,
			},
		}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		// So the validation is already happening in SetupWithManager. So we can panic in the reconcile.
		assert.Panics(t, func() {
			_, _ = mgr.Reconcile(ctx, request, instance)
		})
	})

	t.Run("Lifecycle with manage conditions failing finalize", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:              name,
					Namespace:         namespace,
					Generation:        1,
					Finalizers:        []string{pmtesting.FailureScenarioSubroutineFinalizer},
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{}}, fakeClient)
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.Error(t, err)
		assert.Equal(t, "FailureScenarioSubroutine", err.Error())
	})

	t.Run("Lifecycle with spread reconciles and manage conditions and processing fails (retry)", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditionsAndSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{Retry: true, RequeAfter: false}}, fakeClient)
		mgr.WithSpreadingReconciles()
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.Error(t, err)
		assert.Len(t, instance.Status.Conditions, 2)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, string(v1.ConditionFalse), string(instance.Status.Conditions[0].Status))
		assert.Equal(t, "FailureScenarioSubroutine_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, string(v1.ConditionFalse), string(instance.Status.Conditions[1].Status))
		assert.Equal(t, int64(0), instance.Status.ObservedGeneration)
	})

	t.Run("Lifecycle with spread reconciles and manage conditions and processing fails (no-retry)", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditionsAndSpreadReconciles{
			TestApiObject: pmtesting.TestApiObject{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  namespace,
					Generation: 1,
				},
				Status: pmtesting.TestStatus{
					Some:               "string",
					ObservedGeneration: 0,
				},
			},
		}

		fakeClient := pmtesting.CreateFakeClient(t, instance)

		mgr, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.FailureScenarioSubroutine{RequeAfter: false}}, fakeClient)
		mgr.WithSpreadingReconciles()
		mgr.WithConditionManagement()

		// Act
		_, err := mgr.Reconcile(ctx, request, instance)

		assert.NoError(t, err)
		assert.Len(t, instance.Status.Conditions, 2)
		assert.Equal(t, conditions.ConditionReady, instance.Status.Conditions[0].Type)
		assert.Equal(t, string(v1.ConditionFalse), string(instance.Status.Conditions[0].Status))
		assert.Equal(t, "FailureScenarioSubroutine_Ready", instance.Status.Conditions[1].Type)
		assert.Equal(t, string(v1.ConditionFalse), string(instance.Status.Conditions[1].Status))
		assert.Equal(t, int64(1), instance.Status.ObservedGeneration)
	})

	t.Run("Test Lifecycle setupWithManager /w conditions and expecting no error", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementConditions{}
		fakeClient := pmtesting.CreateFakeClient(t, instance)

		m, err := manager.New(&rest.Config{}, manager.Options{Scheme: fakeClient.Scheme()})
		assert.NoError(t, err)

		lm, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)
		lm = lm.WithConditionManagement()
		tr := &testReconciler{lifecycleManager: lm}

		// Act
		err = lm.SetupWithManager(m, 0, "testReconciler1", instance, "test", tr, log.Logger)

		// Assert
		assert.NoError(t, err)
	})

	t.Run("Test Lifecycle setupWithManager /w conditions and expecting error", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.NotImplementingSpreadReconciles{}
		fakeClient := pmtesting.CreateFakeClient(t, instance)

		m, err := manager.New(&rest.Config{}, manager.Options{Scheme: fakeClient.Scheme()})
		assert.NoError(t, err)

		lm, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)
		lm = lm.WithConditionManagement()
		tr := &testReconciler{lifecycleManager: lm}

		// Act
		err = lm.SetupWithManager(m, 0, "testReconciler2", instance, "test", tr, log.Logger)

		// Assert
		assert.Error(t, err)
	})

	t.Run("Test Lifecycle setupWithManager /w spread and expecting no error", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.ImplementingSpreadReconciles{}
		fakeClient := pmtesting.CreateFakeClient(t, instance)

		m, err := manager.New(&rest.Config{}, manager.Options{Scheme: fakeClient.Scheme()})
		assert.NoError(t, err)

		lm, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)
		lm = lm.WithSpreadingReconciles()
		tr := &testReconciler{lifecycleManager: lm}

		// Act
		err = lm.SetupWithManager(m, 0, "testReconciler3", instance, "test", tr, log.Logger)

		// Assert
		assert.NoError(t, err)
	})

	t.Run("Test Lifecycle setupWithManager /w spread and expecting a error", func(t *testing.T) {
		// Arrange
		instance := &pmtesting.NotImplementingSpreadReconciles{}
		fakeClient := pmtesting.CreateFakeClient(t, instance)

		m, err := manager.New(&rest.Config{}, manager.Options{Scheme: fakeClient.Scheme()})
		assert.NoError(t, err)

		lm, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)
		lm = lm.WithSpreadingReconciles()
		tr := &testReconciler{lifecycleManager: lm}

		// Act
		err = lm.SetupWithManager(m, 0, "testReconciler", instance, "test", tr, log.Logger)

		// Assert
		assert.Error(t, err)
	})

	errorMessage := "oh nose"
	t.Run("handleOperatorError", func(t *testing.T) {
		t.Run("Should handle an operator error with retry and sentry", func(t *testing.T) {
			// Arrange
			instance := &pmtesting.ImplementConditions{}
			fakeClient := pmtesting.CreateFakeClient(t, instance)

			_, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)
			ctx = sentry.ContextWithSentryTags(ctx, map[string]string{})

			// Act
			result, err := lifecycle.HandleOperatorError(ctx, operrors.NewOperatorError(goerrors.New(errorMessage), true, true), "handle op error", true, log.Logger)

			// Assert
			assert.Error(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, errorMessage, err.Error())

			errorMessages, err := log.GetErrorMessages()
			assert.NoError(t, err)
			assert.Equal(t, errorMessage, *errorMessages[0].Error)
		})

		t.Run("Should handle an operator error without retry", func(t *testing.T) {
			// Arrange
			instance := &pmtesting.ImplementConditions{}
			fakeClient := pmtesting.CreateFakeClient(t, instance)

			_, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)

			// Act
			result, err := lifecycle.HandleOperatorError(ctx, operrors.NewOperatorError(goerrors.New(errorMessage), false, false), "handle op error", true, log.Logger)

			// Assert
			assert.Nil(t, err)
			assert.NotNil(t, result)

			errorMessages, err := log.GetErrorMessages()
			assert.NoError(t, err)
			assert.Equal(t, errorMessage, *errorMessages[0].Error)
		})
	})

	t.Run("Prepare Context", func(t *testing.T) {
		t.Run("Sets a context that can be used in the subroutine", func(t *testing.T) {
			// Arrange
			ctx := context.Background()

			fakeClient := pmtesting.CreateFakeClient(t, testApiObject)

			lm, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.ContextValueSubroutine{}}, fakeClient)
			lm = lm.WithPrepareContextFunc(func(ctx context.Context, instance runtimeobject.RuntimeObject) (context.Context, operrors.OperatorError) {
				return context.WithValue(ctx, pmtesting.ContextValueKey, "valueFromContext"), nil
			})
			tr := &testReconciler{lifecycleManager: lm}
			result, err := tr.Reconcile(ctx, controllerruntime.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}})

			// Then
			assert.NotNil(t, ctx)
			assert.NotNil(t, result)
			assert.NoError(t, err)

			err = fakeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, testApiObject)
			assert.NoError(t, err)
			assert.Equal(t, "valueFromContext", testApiObject.Status.Some)
		})

		t.Run("Handles the errors correctly", func(t *testing.T) {
			// Arrange
			ctx := context.Background()

			fakeClient := pmtesting.CreateFakeClient(t, testApiObject)

			lm, _ := createLifecycleManager([]subroutine.Subroutine{pmtesting.ContextValueSubroutine{}}, fakeClient)
			lm = lm.WithPrepareContextFunc(func(ctx context.Context, instance runtimeobject.RuntimeObject) (context.Context, operrors.OperatorError) {
				return nil, operrors.NewOperatorError(goerrors.New(errorMessage), true, false)
			})
			tr := &testReconciler{lifecycleManager: lm}
			result, err := tr.Reconcile(ctx, controllerruntime.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}})

			// Then
			assert.NotNil(t, ctx)
			assert.NotNil(t, result)
			assert.Error(t, err)
		})
	})
}

// Test LifecycleManager.WithConditionManagement
func TestLifecycleManager_WithConditionManagement(t *testing.T) {
	// Given
	fakeClient := pmtesting.CreateFakeClient(t, &pmtesting.TestApiObject{})
	_, log := createLifecycleManager([]subroutine.Subroutine{}, fakeClient)

	// When
	l := NewLifecycleManager(log.Logger, "test-operator", "test-controller", fakeClient, []subroutine.Subroutine{}).WithConditionManagement()

	// Then
	assert.True(t, true, l.ConditionsManager() != nil)
}

type testReconciler struct {
	lifecycleManager *LifecycleManager
}

func (r *testReconciler) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	return r.lifecycleManager.Reconcile(ctx, req, &pmtesting.TestApiObject{})
}

func createLifecycleManager(subroutines []subroutine.Subroutine, c client.Client) (*LifecycleManager, *testlogger.TestLogger) {
	log := testlogger.New()
	mgr := NewLifecycleManager(log.Logger, "test-operator", "test-controller", c, subroutines)
	return mgr, log
}
