package errors

type operatorError struct {
	err    error
	retry  bool
	sentry bool
}

func (e *operatorError) Err() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *operatorError) Retry() bool {
	return e != nil && e.err != nil && e.retry
}

func (e *operatorError) Sentry() bool {
	return e != nil && e.err != nil && e.sentry
}

type OperatorError interface {
	Err() error
	Retry() bool
	Sentry() bool
}

func NewOperatorError(err error, retry bool, sentry bool) OperatorError {
	return &operatorError{err: err, retry: retry, sentry: sentry}
}
