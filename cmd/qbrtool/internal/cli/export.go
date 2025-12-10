package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/platform-mesh/qbrtool/internal/exporter"
	"github.com/platform-mesh/qbrtool/internal/filter"
	"github.com/platform-mesh/qbrtool/internal/github"
	"github.com/platform-mesh/qbrtool/internal/models"
	"github.com/spf13/cobra"
)

var (
	org             string
	projectNumber   int
	quarter         string
	itemTypes       []string
	includeArchived bool
	outputFile      string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export project board items to JSON",
	Long: `Export items from a GitHub Project Board to JSON format.

Supports filtering by quarter (e.g., Q3-2024) and item types (issue, pr, draft, epic).
Can include archived items using the --include-archived flag.

Examples:
  # Export all items from Q3-2024
  qbrtool export --quarter Q3-2024 -f q3-2024.json

  # Export only issues including archived
  qbrtool export --type issue --include-archived -f issues.json

  # Export to stdout
  qbrtool export --quarter Q4-2024`,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().StringVarP(&org, "org", "o", "platform-mesh", "GitHub organization name")
	exportCmd.Flags().IntVarP(&projectNumber, "project", "p", 1, "Project number")
	exportCmd.Flags().StringVarP(&quarter, "quarter", "q", "", "Quarter filter (e.g., Q3-2024, Q1-2025)")
	exportCmd.Flags().StringSliceVarP(&itemTypes, "type", "t", nil, "Item types to include (issue, pr, draft, epic)")
	exportCmd.Flags().BoolVar(&includeArchived, "include-archived", false, "Include archived items")
	exportCmd.Flags().StringVarP(&outputFile, "output-file", "f", "", "Output file path (default: stdout)")
}

func runExport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Validate token
	ghToken := GetToken()
	if ghToken == "" {
		return fmt.Errorf("GitHub token required: set GITHUB_TOKEN env var or use --token flag")
	}

	// Create GitHub client
	client := github.NewClient(ghToken)

	Log("Fetching project ID for %s/projects/%d", org, projectNumber)

	// Get project info
	projectID, err := client.GetProjectID(ctx, org, projectNumber)
	if err != nil {
		return fmt.Errorf("failed to get project ID: %w", err)
	}
	Log("Project ID: %s", projectID)

	// Fetch current (non-archived) items
	Log("Fetching current project items...")
	items, err := client.GetProjectItems(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to get project items: %w", err)
	}
	Log("Fetched %d current items", len(items))

	// Fetch archived items if requested
	if includeArchived {
		Log("Fetching archived items via search...")
		var q *models.Quarter
		if quarter != "" {
			parsed, err := models.ParseQuarter(quarter)
			if err != nil {
				return fmt.Errorf("invalid quarter format: %w", err)
			}
			q = &parsed
		}
		archivedItems, err := client.SearchArchivedItems(ctx, org, projectNumber, q, items, verbose)
		if err != nil {
			return fmt.Errorf("failed to search archived items: %w", err)
		}
		Log("Found %d archived items", len(archivedItems))
		items = mergeItems(items, archivedItems)
		Log("Total items after merge: %d", len(items))
	}

	// Apply filters
	var filters []filter.Filter

	// Quarter filter - by default, show only items closed in the quarter
	if quarter != "" {
		q, err := models.ParseQuarter(quarter)
		if err != nil {
			return fmt.Errorf("invalid quarter format: %w", err)
		}
		filters = append(filters, filter.NewClosedInQuarterFilter(q))
		Log("Applying quarter filter (closed in %s)", quarter)
	}

	// Type filter
	if len(itemTypes) > 0 {
		types := make([]models.ItemType, 0, len(itemTypes))
		for _, t := range itemTypes {
			switch strings.ToLower(t) {
			case "issue":
				types = append(types, models.ItemTypeIssue)
			case "pr", "pullrequest", "pull_request":
				types = append(types, models.ItemTypePullRequest)
			case "draft", "draftissue", "draft_issue":
				types = append(types, models.ItemTypeDraftIssue)
			case "epic":
				// Epic is handled separately via IsEpic field
				filters = append(filters, filter.NewEpicFilter())
			default:
				return fmt.Errorf("unknown item type: %s", t)
			}
		}
		if len(types) > 0 {
			filters = append(filters, filter.NewTypeFilter(types))
		}
		Log("Applying type filter: %v", itemTypes)
	}

	// Apply all filters
	if len(filters) > 0 {
		chain := filter.NewChain(filters, filter.ModeAND)
		items = chain.Apply(items)
		Log("Items after filtering: %d", len(items))
	}

	// Export to JSON
	result := exporter.ExportResult{
		Metadata: exporter.Metadata{
			Organization:  org,
			ProjectNumber: projectNumber,
			Quarter:       quarter,
			ItemTypes:     itemTypes,
			IncludeArchived: includeArchived,
			TotalItems:    len(items),
		},
		Items: items,
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write output
	if outputFile != "" {
		if err := os.WriteFile(outputFile, output, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Exported %d items to %s\n", len(items), outputFile)
	} else {
		fmt.Println(string(output))
	}

	return nil
}

// mergeItems merges current and archived items, deduplicating by ID
func mergeItems(current, archived []*models.ProjectItem) []*models.ProjectItem {
	seen := make(map[string]bool)
	result := make([]*models.ProjectItem, 0, len(current)+len(archived))

	// Current items take precedence
	for _, item := range current {
		if !seen[item.ID] {
			seen[item.ID] = true
			result = append(result, item)
		}
	}

	// Add archived items not already present
	for _, item := range archived {
		if !seen[item.ID] {
			seen[item.ID] = true
			result = append(result, item)
		}
	}

	return result
}
