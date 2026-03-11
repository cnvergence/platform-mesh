package search

import "context"

type SearchRequest struct {
	Organization string
	User         string
	Query        string
	Limit        int
	Cursor       string
}

type SearchResponse struct {
	Results    []SearchHit `json:"results"`
	NextCursor *string     `json:"nextCursor"`
}

type SearchHit struct {
	ID               string                 `json:"id"`
	Score            float64                `json:"score"`
	Kind             string                 `json:"kind,omitempty"`
	Name             string                 `json:"name,omitempty"`
	Namespace        string                 `json:"namespace,omitempty"`
	APIGroup         string                 `json:"apiGroup,omitempty"`
	APIVersion       string                 `json:"apiVersion,omitempty"`
	WorkspacePath    string                 `json:"workspacePath,omitempty"`
	ClusterName      string                 `json:"clusterName,omitempty"`
	OrganizationID   string                 `json:"organizationId,omitempty"`
	OrganizationName string                 `json:"organizationName,omitempty"`
	AccountID        string                 `json:"accountId,omitempty"`
	AccountName      string                 `json:"accountName,omitempty"`
	Source           map[string]interface{} `json:"source"`
}

type SearchIndexRef struct {
	IndexName             string
	OrganizationClusterID string
	Group                 string
	Version               string
}

type OpenSearchHit struct {
	ID     string
	Score  float64
	Sort   []interface{}
	Source map[string]interface{}
}

type OpenSearchPage struct {
	Hits []OpenSearchHit
}

type AuthorizationRequest struct {
	Organization string
	User         string
	Relation     string
	Hits         []OpenSearchHit
}

type AuthorizationResult struct {
	Allowed               []bool
	DroppedMissingContext int
	Denied                int
	Calls                 int
}

type SearchIndexResolver interface {
	ResolveIndex(ctx context.Context, org string) (SearchIndexRef, error)
}

type OpenSearchSearcher interface {
	Search(ctx context.Context, indexName, query string, size int, searchAfter []interface{}) (OpenSearchPage, error)
}

type FGAAuthorizer interface {
	FilterAuthorized(ctx context.Context, req AuthorizationRequest) (AuthorizationResult, error)
}

type OrgAccessValidator interface {
	ValidateTokenForOrg(ctx context.Context, authHeader, org string) (bool, error)
}
