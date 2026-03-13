package kcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/platform-mesh/golang-commons/logger"
	"k8s.io/client-go/rest"

	"github.com/platform-mesh/search/internal/config"
	"github.com/platform-mesh/search/internal/service/search"
)

type SearchIndexResolver struct {
	http    *http.Client
	baseURL *url.URL
	cfg     config.SearchIndexConfig
	log     *logger.Logger
}

type workspaceResource struct {
	Spec struct {
		Cluster string `json:"cluster"`
	} `json:"spec"`
}

type searchIndexResource struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		OrganizationClusterID string `json:"organizationClusterID"`
	} `json:"spec"`
	Status struct {
		IndexName string `json:"indexName"`
	} `json:"status"`
}

func NewSearchIndexResolver(restCfg *rest.Config, cfg config.SearchIndexConfig, log *logger.Logger) (*SearchIndexResolver, error) {
	httpClient, err := rest.HTTPClientFor(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create KCP HTTP client: %w", err)
	}

	baseURL, err := url.Parse(restCfg.Host)
	if err != nil {
		return nil, fmt.Errorf("parse KCP host URL: %w", err)
	}
	baseURL.Path = ""

	return &SearchIndexResolver{
		http:    httpClient,
		baseURL: baseURL,
		cfg:     cfg,
		log:     log,
	}, nil
}

func (r *SearchIndexResolver) ResolveIndex(ctx context.Context, org string) (search.SearchIndexRef, error) {
	org = strings.TrimSpace(org)
	if org == "" {
		return search.SearchIndexRef{}, fmt.Errorf("organization is required")
	}

	orgClusterID, err := r.resolveOrganizationClusterID(ctx, org)
	if err != nil {
		return search.SearchIndexRef{}, err
	}

	resourceURL := fmt.Sprintf("%s://%s/clusters/%s/apis/%s/%s/%s",
		r.baseURL.Scheme,
		r.baseURL.Host,
		r.cfg.WorkspacePath,
		r.cfg.Group,
		r.cfg.Version,
		r.cfg.Resource,
	)
	var list struct {
		Items []searchIndexResource `json:"items"`
	}
	if err := r.getJSON(ctx, resourceURL, &list); err != nil {
		return search.SearchIndexRef{}, fmt.Errorf("list SearchIndex resources: %w", err)
	}
	if len(list.Items) == 0 {
		return search.SearchIndexRef{}, fmt.Errorf(
			"no SearchIndex resources found at %q; ensure SearchIndex objects exist in workspace %q",
			resourceURL,
			r.cfg.WorkspacePath,
		)
	}

	selected, err := selectSearchIndex(org, orgClusterID, list.Items)
	if err != nil {
		return search.SearchIndexRef{}, err
	}

	indexName := strings.TrimSpace(selected.Status.IndexName)
	if indexName == "" {
		indexName = strings.TrimSpace(selected.Spec.OrganizationClusterID)
	}
	if indexName == "" {
		return search.SearchIndexRef{}, fmt.Errorf("searchindex %q has neither status.indexName nor spec.organizationClusterID", selected.Metadata.Name)
	}

	return search.SearchIndexRef{
		IndexName:             indexName,
		OrganizationClusterID: firstNonEmpty(strings.TrimSpace(selected.Spec.OrganizationClusterID), orgClusterID),
		Group:                 r.cfg.Group,
		Version:               r.cfg.Version,
	}, nil
}

func (r *SearchIndexResolver) resolveOrganizationClusterID(ctx context.Context, org string) (string, error) {
	workspaceURL := fmt.Sprintf("%s://%s/clusters/%s/apis/tenancy.kcp.io/v1alpha1/workspaces/%s",
		r.baseURL.Scheme,
		r.baseURL.Host,
		r.cfg.WorkspacePath,
		org,
	)

	var payload workspaceResource
	if err := r.getJSON(ctx, workspaceURL, &payload); err != nil {
		return "", fmt.Errorf("resolve workspace cluster ID for org %q: %w", org, err)
	}

	clusterID := strings.TrimSpace(payload.Spec.Cluster)
	if clusterID == "" {
		return "", fmt.Errorf("workspace %q does not expose spec.cluster", org)
	}
	return clusterID, nil
}

func (r *SearchIndexResolver) getJSON(ctx context.Context, requestURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := r.http.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func selectSearchIndex(org, orgClusterID string, items []searchIndexResource) (searchIndexResource, error) {
	clusterMatches := make([]searchIndexResource, 0, 1)
	for _, item := range items {
		if strings.TrimSpace(item.Spec.OrganizationClusterID) == orgClusterID {
			clusterMatches = append(clusterMatches, item)
		}
	}
	if len(clusterMatches) == 1 {
		return clusterMatches[0], nil
	}
	if len(clusterMatches) > 1 {
		for _, item := range clusterMatches {
			if strings.TrimSpace(item.Metadata.Name) == org {
				return item, nil
			}
		}
		return searchIndexResource{}, fmt.Errorf("multiple SearchIndex resources match org cluster %q", orgClusterID)
	}

	for _, item := range items {
		if strings.TrimSpace(item.Metadata.Name) == org {
			return item, nil
		}
	}

	return searchIndexResource{}, fmt.Errorf("no SearchIndex matched org %q and org cluster %q", org, orgClusterID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
