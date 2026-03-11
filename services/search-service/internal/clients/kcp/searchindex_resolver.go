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
	resourceURL := fmt.Sprintf("%s://%s/clusters/%s/apis/%s/%s/%s/%s",
		r.baseURL.Scheme,
		r.baseURL.Host,
		r.cfg.WorkspacePath,
		r.cfg.Group,
		r.cfg.Version,
		r.cfg.Resource,
		org,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL, http.NoBody)
	if err != nil {
		return search.SearchIndexRef{}, fmt.Errorf("create SearchIndex request: %w", err)
	}

	resp, err := r.http.Do(req)
	if err != nil {
		return search.SearchIndexRef{}, fmt.Errorf("execute SearchIndex request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return search.SearchIndexRef{}, fmt.Errorf("read SearchIndex failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var payload struct {
		Spec struct {
			OrganizationClusterID string `json:"organizationClusterID"`
		} `json:"spec"`
		Status struct {
			IndexName string `json:"indexName"`
		} `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return search.SearchIndexRef{}, fmt.Errorf("decode SearchIndex response: %w", err)
	}

	indexName := strings.TrimSpace(payload.Status.IndexName)
	if indexName == "" {
		indexName = strings.TrimSpace(payload.Spec.OrganizationClusterID)
	}
	if indexName == "" {
		return search.SearchIndexRef{}, fmt.Errorf("searchindex has neither status.indexName nor spec.organizationClusterID")
	}

	return search.SearchIndexRef{
		IndexName:             indexName,
		OrganizationClusterID: payload.Spec.OrganizationClusterID,
		Group:                 r.cfg.Group,
		Version:               r.cfg.Version,
	}, nil
}
