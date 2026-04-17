## Repository Description
- `subroutines` provides the lifecycle engine used by Platform Mesh controllers and operators.
- The main exported areas are subroutine orchestration, typed reconciliation `Result` values, condition management, spread scheduling, finalizer handling, and metrics.
- This is a Go library repo built around [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) and [multicluster-runtime](https://github.com/platform-mesh/multicluster-runtime).
- Read the org-wide [AGENTS.md](https://github.com/platform-mesh/.github/blob/main/AGENTS.md) for general conventions.

## Core Principles
- Keep changes small and local. Behavior changes here can ripple into multiple downstream operators.
- Prefer extending existing lifecycle patterns over introducing parallel orchestration abstractions.
- Verify behavior before finishing. Start with focused package tests, then broader validation if needed.
- Keep this file focused on agent execution and repository-specific constraints.

## Project Structure
- `subroutines`: core interfaces and typed `Result` helpers.
- `lifecycle`: orchestration engine that runs subroutines and coordinates reconciliation flow.
- `conditions`: condition management helpers for subroutine and aggregate readiness.
- `spread`: helpers for spreading reconciliation work over time.
- `metrics`: Prometheus metrics for subroutine execution.

## Architecture
This is a dependency repo, not a runnable service. Small semantic changes can affect many controllers.

### Lifecycle model
- The core abstraction is the ordered execution of subroutines that return typed `Result` values such as `OK`, `Pending`, and `Stop`.
- `lifecycle` owns reconciliation flow semantics, finalizer handling, and status/condition integration.
- `conditions` is the supported path for condition aggregation; avoid inventing alternative condition wiring in this repo.

### Downstream impact
- Several Platform Mesh operators depend on this library for reconcile ordering and readiness semantics.
- A change that looks local, such as altering `Result` handling or condition updates, can change runtime behavior across multiple repositories.

## Commands
- `task lint` — run formatting and golangci-lint.
- `task test` — run the standard local unit test flow.
- `task cover` — run tests with coverage output.
- `task test-coverage` — enforce coverage thresholds from `.testcoverage.yml`.
- `task validate` — run the standard validation flow.
- `go test ./<pkg>` — fast fallback for targeted package verification.

## Code Conventions
- Preserve public package APIs unless the change is intentionally coordinated with downstream consumers.
- Add or update `_test.go` files alongside behavior changes.
- Follow the existing package split instead of introducing new umbrella utility packages.
- Keep logging, metrics, and condition semantics explicit and predictable.

## Generated Artifacts
- Mocks and generated outputs should be regenerated through the repo workflows, not hand-edited.
- Review coverage-related changes carefully when touching core lifecycle behavior.

## Do Not
- Change `Result` semantics casually.
- Hand-edit generated outputs if a repo workflow exists to regenerate them.
- Update `.testcoverage.yml` unless the task explicitly requires it.

## Hard Boundaries
- Do not invent new local workflows when a `task` target already exists.
- Ask before making changes that intentionally break compatibility for downstream operators.

## Human-Facing Guidance
- Use `README.md` for local setup and rough architecture context.
- Use `CONTRIBUTING.md` for contribution process, DCO, and broader developer workflow expectations.
