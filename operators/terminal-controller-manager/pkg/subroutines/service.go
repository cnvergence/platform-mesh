/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package subroutines

import (
	"context"
	"fmt"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/ratelimiter"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/errors"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/terminal-controller-manager/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/workqueue"
)

const (
	ServiceSubroutineName      = "ServiceSubroutine"
	ServiceSubroutineFinalizer = "terminal.platform-mesh.io/service-finalizer"
	TerminalServicePort        = 8080
)

// ServiceSubroutine manages terminal services on the runtime cluster
type ServiceSubroutine struct {
	runtimeClient client.Client
	limiter       workqueue.TypedRateLimiter[*v1alpha1.Terminal]
	namespace     string
}

func NewServiceSubroutine(runtimeClient client.Client, namespace string) *ServiceSubroutine {
	rl, _ := ratelimiter.NewStaticThenExponentialRateLimiter[*v1alpha1.Terminal](ratelimiter.NewConfig()) //nolint:errcheck
	return &ServiceSubroutine{
		runtimeClient: runtimeClient,
		limiter:       rl,
		namespace:     namespace,
	}
}

func (r *ServiceSubroutine) GetName() string {
	return ServiceSubroutineName
}

func (r *ServiceSubroutine) Finalizers(_ runtimeobject.RuntimeObject) []string { // coverage-ignore
	return []string{ServiceSubroutineFinalizer}
}

func (r *ServiceSubroutine) Finalize(ctx context.Context, ro runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
	instance := ro.(*v1alpha1.Terminal)
	log := logger.LoadLoggerFromContext(ctx)

	serviceName := fmt.Sprintf("terminal-%s", instance.Name)
	service := &corev1.Service{}
	serviceKey := client.ObjectKey{Namespace: r.namespace, Name: serviceName}

	if err := r.runtimeClient.Get(ctx, serviceKey, service); err != nil {
		if kerrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			r.limiter.Forget(instance)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.NewOperatorError(err, true, true)
	}

	if service.GetDeletionTimestamp() != nil {
		log.Debug().Str("serviceName", service.Name).Msg("service is already being deleted, waiting")
		return ctrl.Result{RequeueAfter: r.limiter.When(instance)}, nil
	}

	log.Info().Str("serviceName", service.Name).Msg("deleting terminal service")
	if err := r.runtimeClient.Delete(ctx, service); err != nil {
		if kerrors.IsNotFound(err) {
			r.limiter.Forget(instance)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.NewOperatorError(err, true, true)
	}

	return ctrl.Result{RequeueAfter: r.limiter.When(instance)}, nil
}

func (r *ServiceSubroutine) Process(ctx context.Context, ro runtimeobject.RuntimeObject) (ctrl.Result, errors.OperatorError) {
	instance := ro.(*v1alpha1.Terminal)
	log := logger.LoadLoggerFromContext(ctx)

	serviceName := fmt.Sprintf("terminal-%s", instance.Name)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: r.namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.runtimeClient, service, func() error {
		r.mutateService(service, instance)
		return nil
	})
	if err != nil {
		return ctrl.Result{}, errors.NewOperatorError(err, true, true)
	}

	log.Debug().Str("serviceName", serviceName).Str("result", string(result)).Msg("service reconciled")
	r.limiter.Forget(instance)
	return ctrl.Result{}, nil
}

func (r *ServiceSubroutine) mutateService(service *corev1.Service, terminal *v1alpha1.Terminal) {
	service.Labels = map[string]string{
		"app.kubernetes.io/name":                  "terminal",
		"app.kubernetes.io/instance":              terminal.Name,
		"app.kubernetes.io/managed-by":            "terminal-controller-manager",
		"terminal.platform-mesh.io/terminal-name": terminal.Name,
	}
	service.Spec.Type = corev1.ServiceTypeClusterIP
	service.Spec.Selector = map[string]string{
		"terminal.platform-mesh.io/terminal-name": terminal.Name,
	}
	service.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "http",
			Port:       TerminalServicePort,
			TargetPort: intstr.FromInt32(TerminalServicePort),
			Protocol:   corev1.ProtocolTCP,
		},
	}
}
