package github

import (
	"context"
	"fmt"
	"time"

	"github.com/platform-mesh/qbrtool/internal/models"
	"github.com/shurcooL/graphql"
)

// GetProjectID returns the project ID for the given organization and project number
func (c *Client) GetProjectID(ctx context.Context, org string, projectNumber int) (string, error) {
	var query struct {
		Organization struct {
			ProjectV2 struct {
				ID graphql.ID
			} `graphql:"projectV2(number: $projectNumber)"`
		} `graphql:"organization(login: $org)"`
	}

	variables := map[string]interface{}{
		"org":           graphql.String(org),
		"projectNumber": graphql.Int(projectNumber),
	}

	if err := c.gql.Query(ctx, &query, variables); err != nil {
		return "", fmt.Errorf("failed to query project: %w", err)
	}

	return fmt.Sprintf("%v", query.Organization.ProjectV2.ID), nil
}

// GetProjectItems fetches all items from a project
func (c *Client) GetProjectItems(ctx context.Context, projectID string) ([]*models.ProjectItem, error) {
	var allItems []*models.ProjectItem
	var cursor *graphql.String

	for {
		var query struct {
			Node struct {
				ProjectV2 struct {
					Items struct {
						TotalCount graphql.Int
						PageInfo   struct {
							HasNextPage graphql.Boolean
							EndCursor   graphql.String
						}
						Nodes []struct {
							ID         graphql.ID
							IsArchived graphql.Boolean
							Type       graphql.String
							CreatedAt  graphql.String
							UpdatedAt  graphql.String
							Content    struct {
								Typename graphql.String `graphql:"__typename"`
								Issue    struct {
									ID        graphql.ID
									Number    graphql.Int
									Title     graphql.String
									Body      graphql.String
									State     graphql.String
									URL       graphql.String `graphql:"url"`
									CreatedAt graphql.String
									ClosedAt  graphql.String
									Labels    struct {
										Nodes []struct {
											Name graphql.String
										}
									} `graphql:"labels(first: 20)"`
									Repository struct {
										Owner struct {
											Login graphql.String
										}
										Name graphql.String
									}
								} `graphql:"... on Issue"`
								PullRequest struct {
									ID        graphql.ID
									Number    graphql.Int
									Title     graphql.String
									Body      graphql.String
									State     graphql.String
									URL       graphql.String `graphql:"url"`
									CreatedAt graphql.String
									ClosedAt  graphql.String
									MergedAt  graphql.String
									Labels    struct {
										Nodes []struct {
											Name graphql.String
										}
									} `graphql:"labels(first: 20)"`
									Repository struct {
										Owner struct {
											Login graphql.String
										}
										Name graphql.String
									}
								} `graphql:"... on PullRequest"`
								DraftIssue struct {
									Title     graphql.String
									Body      graphql.String
									CreatedAt graphql.String
								} `graphql:"... on DraftIssue"`
							}
						}
					} `graphql:"items(first: 100, after: $cursor)"`
				} `graphql:"... on ProjectV2"`
			} `graphql:"node(id: $projectId)"`
		}

		variables := map[string]interface{}{
			"projectId": graphql.ID(projectID),
			"cursor":    cursor,
		}

		if err := c.gql.Query(ctx, &query, variables); err != nil {
			return nil, fmt.Errorf("failed to query project items: %w", err)
		}

		// Convert nodes to ProjectItem models
		for _, node := range query.Node.ProjectV2.Items.Nodes {
			item := &models.ProjectItem{
				ProjectItemID: fmt.Sprintf("%v", node.ID),
				IsArchived:    bool(node.IsArchived),
				FieldValues:   make(map[string]string),
			}

			// Parse content based on type
			switch string(node.Content.Typename) {
			case "Issue":
				issue := node.Content.Issue
				item.ID = fmt.Sprintf("%v", issue.ID)
				item.Type = models.ItemTypeIssue
				item.Number = int(issue.Number)
				item.Title = string(issue.Title)
				item.Body = string(issue.Body)
				item.State = string(issue.State)
				item.URL = string(issue.URL)
				item.CreatedAt = parseTime(string(issue.CreatedAt))
				item.ClosedAt = parseTimePtr(string(issue.ClosedAt))
				item.Repository = models.Repository{
					Owner: string(issue.Repository.Owner.Login),
					Name:  string(issue.Repository.Name),
				}
				for _, label := range issue.Labels.Nodes {
					item.Labels = append(item.Labels, string(label.Name))
				}

			case "PullRequest":
				pr := node.Content.PullRequest
				item.ID = fmt.Sprintf("%v", pr.ID)
				item.Type = models.ItemTypePullRequest
				item.Number = int(pr.Number)
				item.Title = string(pr.Title)
				item.Body = string(pr.Body)
				item.State = string(pr.State)
				item.URL = string(pr.URL)
				item.CreatedAt = parseTime(string(pr.CreatedAt))
				item.ClosedAt = parseTimePtr(string(pr.ClosedAt))
				item.MergedAt = parseTimePtr(string(pr.MergedAt))
				item.Repository = models.Repository{
					Owner: string(pr.Repository.Owner.Login),
					Name:  string(pr.Repository.Name),
				}
				for _, label := range pr.Labels.Nodes {
					item.Labels = append(item.Labels, string(label.Name))
				}

			case "DraftIssue":
				draft := node.Content.DraftIssue
				item.ID = item.ProjectItemID
				item.Type = models.ItemTypeDraftIssue
				item.Title = string(draft.Title)
				item.Body = string(draft.Body)
				item.CreatedAt = parseTime(string(draft.CreatedAt))

			default:
				continue
			}

			if string(node.UpdatedAt) != "" {
				item.UpdatedAt = parseTime(string(node.UpdatedAt))
			}

			allItems = append(allItems, item)
		}

		// Check for more pages
		if !bool(query.Node.ProjectV2.Items.PageInfo.HasNextPage) {
			break
		}
		cursor = &query.Node.ProjectV2.Items.PageInfo.EndCursor
	}

	return allItems, nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
