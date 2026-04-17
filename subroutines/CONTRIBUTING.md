## Overview

# Contributing to Platform Mesh

We want to make contributing to this project as easy and transparent as possible.

## Our development process

We use GitHub to track issues and feature requests, as well as accept pull requests.

## Pull requests

You are welcome to contribute with your pull requests. These steps explain the contribution process:

1. Fork the repository and create your branch from `main`.
1. [Add tests](#testing) for your code.
1. If you've changed APIs, update the documentation.
1. Make sure the tests pass. Our GitHub Actions pipeline runs the tests for your PR and will indicate any issues.
1. Sign the Developer Certificate of Origin (DCO).

## Development setup

subroutines uses [Task](https://taskfile.dev) for build automation:

```bash
task lint       # Format and lint (golangci-lint)
task test       # Run unit tests
task cover      # Run tests with coverage
task validate   # Run lint + test
```

### Code style

- Follow the [gci import ordering](https://github.com/daixiang0/gci): standard, default, `k8s.io` prefix
- Avoid named return values
- Use `ptr.To()` and `ptr.Deref()` from `k8s.io/utils/ptr`
- Use `any` instead of `interface{}`

## Testing

> **NOTE:** You should always add tests if you are adding code to our repository.

Prefer table-driven tests. To run tests locally:

```bash
task test
```

## Generative AI

If you use generative AI tools while preparing a contribution, you remain responsible for the correctness, safety, licensing, and maintainability of the submitted change.

If you use Claude while contributing, do not assume it will automatically pick up this repository's `AGENTS.md`. Explicitly provide or reference `AGENTS.md` at the start of the session so Claude has the repository-specific instructions before it suggests or applies changes.

## Issues

We use GitHub issues to track bugs. Please ensure your description is
clear and includes sufficient instructions to reproduce the issue.

## License

By contributing to Platform Mesh, you agree that your contributions will be licensed
under its [Apache-2.0 license](LICENSE).
