package builder

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/controllerruntime"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/multicluster"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/ratelimiter"
	"github.com/platform-mesh/golang-commons/controller/lifecycle/subroutine"
	"github.com/platform-mesh/golang-commons/logger"
)

type Builder struct {
	operatorName            string
	controllerName          string
	withConditionManagement bool
	withSpreadingReconciles bool
	withReadOnly            bool
	rateLimiterOptions      *[]ratelimiter.Option
	subroutines             []subroutine.Subroutine
	log                     *logger.Logger
}

func NewBuilder(operatorName, controllerName string, subroutines []subroutine.Subroutine, log *logger.Logger) *Builder {
	return &Builder{
		operatorName:            operatorName,
		controllerName:          controllerName,
		log:                     log,
		withConditionManagement: false,
		subroutines:             subroutines,
	}
}

func (b *Builder) WithConditionManagement() *Builder {
	b.withConditionManagement = true
	return b
}

func (b *Builder) WithSpreadingReconciles() *Builder {
	b.withSpreadingReconciles = true
	return b
}

func (b *Builder) WithReadOnly() *Builder {
	b.withReadOnly = true
	return b
}

func (b *Builder) WithStaticThenExponentialRateLimiter(opts ...ratelimiter.Option) *Builder {
	b.rateLimiterOptions = &opts
	return b
}

func (b *Builder) BuildControllerRuntime(cl client.Client) *controllerruntime.LifecycleManager {
	lm := controllerruntime.NewLifecycleManager(b.subroutines, b.operatorName, b.controllerName, cl, b.log)
	if b.withConditionManagement {
		lm.WithConditionManagement()
	}
	if b.withSpreadingReconciles {
		lm.WithSpreadingReconciles()
	}
	if b.withReadOnly {
		lm.WithReadOnly()
	}
	if b.rateLimiterOptions != nil {
		lm.WithStaticThenExponentialRateLimiter((*b.rateLimiterOptions)...)
	}
	return lm
}

func (b *Builder) BuildMultiCluster(mgr mcmanager.Manager) *multicluster.LifecycleManager {
	lm := multicluster.NewLifecycleManager(b.subroutines, b.operatorName, b.controllerName, mgr, b.log)
	if b.withConditionManagement {
		lm.WithConditionManagement()
	}
	if b.withSpreadingReconciles {
		lm.WithSpreadingReconciles()
	}
	if b.withReadOnly {
		lm.WithReadOnly()
	}
	if b.rateLimiterOptions != nil {
		lm.WithStaticThenExponentialRateLimiter((*b.rateLimiterOptions)...)
	}
	return lm
}
