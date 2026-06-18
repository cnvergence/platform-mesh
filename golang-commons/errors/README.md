# Platform Mesh Errors

The `errors` package is a drop-in replacement for the stdlib `errors` package but enhances
all errors with a stacktrace. This is especially useful in combination with Sentry as it implements
the `StackFrames()` function that the Sentry SDK uses to collect data to show a call history. 

### Usage

This package can be used like the default stdlib `errors` package. Just switch the import from
`errors` to `github.com/platform-mesh/golang-commons/errors`.

Instead of `fmt.Errorf()` to wrap errors you can use the `errors.Errorf()` function from this package
or the helper functions `errors.Wrap()` or `errors.Wrapf()`.
All of the above return an error with attached stack trace.

To add the current stacktrace to an existing error use the `errors.WithStack()` util function.
