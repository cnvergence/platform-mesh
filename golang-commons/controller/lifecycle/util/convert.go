package util

import (
	"fmt"
	"reflect"

	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/sentry"
)

func ToInterface[T any](instance any, log *logger.Logger) (T, error) {
	var zero T
	obj, ok := instance.(T)
	if ok {
		return obj, nil
	}
	err := fmt.Errorf("failed to cast instance of type %T to %v", instance, reflect.TypeOf(zero))
	log.Error().Err(err).Msg("Failed to cast instance to target interface")
	sentry.CaptureError(err, nil)
	return zero, err
}

func MustToInterface[T any](instance any, log *logger.Logger) T {
	obj, err := ToInterface[T](instance, log)
	if err == nil {
		return obj
	}
	log.Panic().Err(err).Msg("Failed to cast instance to target interface")
	panic(err)
}
