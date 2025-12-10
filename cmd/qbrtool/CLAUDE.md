# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build          # Build binary to ./bin/qbrtool
make test           # Run tests with race detection
make test-coverage  # Run tests with coverage report (outputs coverage.html)
make lint           # Run golangci-lint
make vet            # Run go vet
make install        # Install to $GOPATH/bin
```

## Running the Tool

Requires `GITHUB_TOKEN` environment variable (or `--token` flag):
```bash
export GITHUB_TOKEN=$(gh auth token)  # If using GitHub CLI

# Export project items
./bin/qbrtool export --quarter Q3-2024 --include-archived -f output.json

# Analyze exported items
./bin/qbrtool analyze -i output.json --analysis all

# Pipeline: export and analyze
./bin/qbrtool export --quarter Q3-2024 | ./bin/qbrtool analyze --analysis cve
```

## Architecture

This is a Go CLI tool built with Cobra that exports GitHub Project Board items and analyzes them for quarterly reports.

### Package Structure

- `cmd/qbrtool/` - Entry point, calls `cli.Execute()`
- `internal/cli/` - Cobra commands: `root.go` (global flags, token handling), `export.go`, `analyze.go`
- `internal/github/` - GraphQL client wrapping `shurcooL/graphql`:
  - `client.go` - Base client with OAuth2
  - `project.go` - Project queries (GetProjectID, GetProjectItems)
  - `search.go` - Search API for archived items workaround
- `internal/models/` - Domain types:
  - `item.go` - `ProjectItem`, `MatchedItem`, `AnalysisResult`
  - `quarter.go` - Quarter parsing and date range logic
- `internal/filter/` - Filter interface and implementations (quarter, type, epic)
- `internal/analyzer/` - Analyzer interface with implementations: CVE, OSS, monitoring, lifecycle, security
- `internal/exporter/` - JSON export formatting

### Key Patterns

**Filter Chain**: Filters implement `Filter` interface with `Matches(item)` method. Combined via `filter.Chain` with AND/OR modes.

**Analyzer Registry**: Analyzers implement `Analyzer` interface with `Name()` and `Analyze(items)` methods. Can be composed via `NewDefaultRegistry()`.

**Archived Items Workaround**: GitHub's ProjectV2 API doesn't return archived items. The tool uses search API queries split by month (to avoid 1000-result limit) and checks `projectItems` connection on found issues/PRs.

### Data Flow

1. `export` command fetches project items via GraphQL
2. Optionally searches for archived items via Search API
3. Applies filter chain (quarter, type)
4. Outputs JSON with metadata + items
5. `analyze` command reads JSON, runs selected analyzers, outputs analysis results
