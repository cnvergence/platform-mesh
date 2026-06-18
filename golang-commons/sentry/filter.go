package sentry

import (
	"errors"

	v1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
)

func ShouldBeProcessed(err error) bool {
	if sentryErr, ok := AsSentryError(err); ok {
		err = sentryErr.GetReason()
	}

	return !isTerminatingNSError(err)
}

func isTerminatingNSError(err error) bool {
	status := apiErrors.APIStatus(nil)

	if errors.As(err, &status) && status.Status().Details != nil {
		for _, cause := range status.Status().Details.Causes {
			if cause.Type == v1.NamespaceTerminatingCause {
				return true
			}
		}
	}

	return false
}
