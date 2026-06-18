package errors

import (
	"time"

	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func IsRetriable(err error) (bool, ctrl.Result) {
	// This covers ServerTimeout, Timeout and TooManyRequests
	delay, ok := k8sErrors.SuggestsClientDelay(err)
	if ok {
		return true, ctrl.Result{RequeueAfter: time.Duration(delay) * time.Second}
	}

	if k8sErrors.IsInternalError(err) || k8sErrors.IsServiceUnavailable(err) || k8sErrors.IsConflict(err) {
		return true, ctrl.Result{}
	}

	return false, ctrl.Result{}
}
