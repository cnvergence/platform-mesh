package controller

import (
	"github.com/platform-mesh/golang-commons/logger"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
)

func SetupWithManager(mgr mcmanager.Manager, log *logger.Logger) error {
	// Controllers will be registered here in Phase 3
	_ = mgr
	_ = log
	return nil
}
