# qbrtool - Quarterly Board Report Tool

A Go CLI tool for exporting and analyzing GitHub Project Board items. Designed for generating quarterly reports with support for archived items, CVE tracking, OSS contributions, and more.

## Features

- **Export to JSON** - Export project board items with filtering options
- **Quarter filtering** - Filter items closed within a calendar quarter (Q1-Q4)
- **Item type filtering** - Filter by issue, PR, draft, or epic
- **Archived items support** - Retrieve archived items via search API workaround
- **Analysis capabilities**:
  - CVE detection (pattern: `CVE-YYYY-NNNNN`)
  - OSS contributions to specific organizations
  - Monitoring/observability related items
  - Lifecycle management items
  - Security related items

## Installation

### Prerequisites

- Go 1.21 or later
- GitHub CLI (`gh`) for authentication (recommended)

### Build from source

```bash
git clone https://github.com/platform-mesh/qbrtool.git
cd qbrtool
make build
```

The binary will be available at `./bin/qbrtool`.

### Install to GOPATH

```bash
make install
```

## Authentication

The tool requires a GitHub personal access token with the following scopes:
- `read:project` - Read access to projects
- `repo` - Read access to repositories (for searching issues/PRs)

### Option 1: Using GitHub CLI (Recommended)

If you have the GitHub CLI (`gh`) installed and authenticated:

```bash
# Use gh to get a token and pass it to qbrtool
export GITHUB_TOKEN=$(gh auth token)

# Or run directly with the token
GITHUB_TOKEN=$(gh auth token) ./bin/qbrtool export --quarter Q3-2024
```

You can add this to your shell profile (`.bashrc`, `.zshrc`, etc.):

```bash
# Add to ~/.zshrc or ~/.bashrc
export GITHUB_TOKEN=$(gh auth token)
```

### Option 2: Personal Access Token

1. Go to GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens
2. Create a new token with:
   - Repository access: All repositories (or select specific ones)
   - Permissions:
     - Repository permissions: Read access to issues and pull requests
     - Organization permissions: Read access to projects
3. Export the token:

```bash
export GITHUB_TOKEN=github_pat_xxxxxxxxxxxx
```

### Option 3: Pass token directly

```bash
./bin/qbrtool export --token github_pat_xxxxxxxxxxxx --quarter Q3-2024
```

## Usage

### Export Command

Export project board items to JSON format.

```bash
# Export items closed in Q3-2024 (July-September)
./bin/qbrtool export --quarter Q3-2024 -f q3-2024.json

# Include archived items (uses search API workaround)
./bin/qbrtool export --quarter Q3-2024 --include-archived -f q3-2024.json

# Export only issues
./bin/qbrtool export --quarter Q3-2024 --type issue -f issues.json

# Export only PRs
./bin/qbrtool export --quarter Q3-2024 --type pr -f prs.json

# Export epics only
./bin/qbrtool export --quarter Q3-2024 --type epic -f epics.json

# Export from a different project
./bin/qbrtool export --org my-org --project 5 --quarter Q3-2024

# Export to stdout (for piping)
./bin/qbrtool export --quarter Q3-2024 --include-archived
```

#### Export Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--org` | `-o` | GitHub organization name | `platform-mesh` |
| `--project` | `-p` | Project number | `1` |
| `--quarter` | `-q` | Show items closed in quarter (e.g., Q3-2024) | - |
| `--type` | `-t` | Item types: issue, pr, draft, epic | all |
| `--include-archived` | - | Include archived items | `false` |
| `--output-file` | `-f` | Output file path | stdout |

### Analyze Command

Analyze exported items for specific categories.

```bash
# Run all analyzers
./bin/qbrtool analyze -i q3-2024.json --analysis all

# Find CVEs
./bin/qbrtool analyze -i q3-2024.json --analysis cve

# Find OSS contributions
./bin/qbrtool analyze -i q3-2024.json --analysis oss

# Find monitoring-related items
./bin/qbrtool analyze -i q3-2024.json --analysis monitoring

# Find lifecycle management items
./bin/qbrtool analyze -i q3-2024.json --analysis lifecycle

# Find security-related items
./bin/qbrtool analyze -i q3-2024.json --analysis security

# Custom OSS organizations
./bin/qbrtool analyze -i q3-2024.json --analysis oss --oss-orgs kubernetes,istio,envoyproxy

# Save analysis to file
./bin/qbrtool analyze -i q3-2024.json --analysis all -f analysis.json
```

#### Analyze Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--input` | `-i` | Input JSON file | stdin |
| `--analysis` | `-a` | Analysis type(s) | `all` |
| `--oss-orgs` | - | OSS orgs to detect | `kcp-dev,kube-bind,multicluster-runtime` |
| `--output-file` | `-f` | Output file path | stdout |

### Pipeline Usage

Export and analyze in one command:

```bash
# Export items closed in Q3-2024 and run all analyzers
./bin/qbrtool export --quarter Q3-2024 --include-archived | ./bin/qbrtool analyze --analysis all

# Export, analyze, and save
./bin/qbrtool export --quarter Q3-2024 --include-archived | ./bin/qbrtool analyze --analysis all -f report.json
```

### Verbose Mode

Enable verbose logging to see what the tool is doing:

```bash
./bin/qbrtool export --quarter Q3-2024 --include-archived -v
```

## Analysis Types

### CVE Analysis
Detects CVE references using the pattern `CVE-YYYY-NNNNN` in:
- Issue/PR titles
- Issue/PR bodies
- Labels

### OSS Contribution Analysis
Identifies contributions to specified OSS organizations by checking:
- Repository owner
- URLs in content
- Mentions in title/body

Default organizations: `kcp-dev`, `kube-bind`, `multicluster-runtime`

### Monitoring Analysis
Keywords: monitoring, observability, metrics, prometheus, grafana, alerting, logging, tracing, opentelemetry, dashboard, SLO, SLI, etc.

### Lifecycle Analysis
Keywords: lifecycle, upgrade, migration, deprecation, EOL, maintenance, release, version, rollout, rollback, canary, blue-green, etc.

### Security Analysis
Keywords: security, vulnerability, CVE, RBAC, authentication, authorization, TLS, certificate, encryption, audit, penetration, hardening, etc.

## Output Format

### Export Output

When using `--quarter`, only items closed within that quarter are returned.

```json
{
  "metadata": {
    "organization": "platform-mesh",
    "project_number": 1,
    "quarter": "Q3-2024",
    "include_archived": true,
    "total_items": 42,
    "exported_at": "2024-10-15T10:30:00Z"
  },
  "items": [
    {
      "id": "I_xxxxx",
      "type": "ISSUE",
      "is_archived": false,
      "number": 123,
      "title": "Fix CVE-2024-1234",
      "body": "...",
      "state": "CLOSED",
      "url": "https://github.com/...",
      "created_at": "2024-07-15T...",
      "closed_at": "2024-08-20T...",
      "repository": {
        "owner": "platform-mesh",
        "name": "my-repo"
      },
      "labels": ["security", "priority/high"],
      "field_values": {
        "Status": "Done",
        "Type": "Bug"
      },
      "is_epic": false
    }
  ]
}
```

### Analysis Output

```json
{
  "source_metadata": { ... },
  "results": {
    "cve": {
      "type": "cve",
      "items": [...],
      "summary": {
        "cve_ids": ["CVE-2024-1234", "CVE-2024-5678"],
        "count": 2
      },
      "timestamp": "2024-10-15T..."
    },
    "oss": {
      "type": "oss",
      "items": [...],
      "summary": {
        "by_org": {
          "kcp-dev": 5,
          "kube-bind": 3
        },
        "total": 8
      }
    }
  }
}
```

## Development

```bash
# Build
make build

# Run tests
make test

# Run tests with coverage
make test-coverage

# Lint (requires golangci-lint)
make lint

# Clean build artifacts
make clean

# Show all make targets
make help
```

## Known Limitations

1. **GitHub Search API limit**: Maximum 1000 results per query. The tool splits queries by month to mitigate this.

2. **Archived items**: GitHub's ProjectV2 API doesn't return archived items directly. The tool uses a search-based workaround that queries issues/PRs and checks their `projectItems` connection.

3. **Draft Issues**: Draft issues are not searchable via GitHub's search API, so they can only be retrieved from the project items query (if not archived).

4. **Rate limits**: GitHub's GraphQL API has rate limits. For large exports, you may need to wait between requests.

## License

MIT
