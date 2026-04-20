package search

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/platform-mesh/golang-commons/logger"

	"github.com/platform-mesh/search/internal/observability"
)

type ServiceConfig struct {
	DefaultLimit   int
	MaxLimit       int
	FetchBatchSize int
	MaxScannedHits int
}

type Service struct {
	resolver   SearchIndexResolver
	searcher   OpenSearchSearcher
	authorizer FGAAuthorizer
	metrics    *observability.Metrics
	cfg        ServiceConfig
}

func NewService(
	resolver SearchIndexResolver,
	searcher OpenSearchSearcher,
	authorizer FGAAuthorizer,
	metrics *observability.Metrics,
	cfg ServiceConfig,
) *Service {
	if cfg.DefaultLimit <= 0 {
		cfg.DefaultLimit = 20
	}
	if cfg.MaxLimit <= 0 {
		cfg.MaxLimit = 100
	}
	if cfg.FetchBatchSize <= 0 {
		cfg.FetchBatchSize = 100
	}
	if cfg.MaxScannedHits <= 0 {
		cfg.MaxScannedHits = 1000
	}
	if metrics == nil {
		metrics = observability.NewMetrics()
	}

	return &Service{
		resolver:   resolver,
		searcher:   searcher,
		authorizer: authorizer,
		metrics:    metrics,
		cfg:        cfg,
	}
}

func (s *Service) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	start := time.Now()
	s.metrics.IncSearchRequests()
	defer func() {
		s.metrics.ObserveSearchDuration(time.Since(start))
	}()

	log := logger.LoadLoggerFromContext(ctx)

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return SearchResponse{}, fmt.Errorf("%w: q is required", ErrInvalidRequest)
	}
	org := strings.TrimSpace(req.Organization)
	if org == "" {
		return SearchResponse{}, fmt.Errorf("%w: organization is required", ErrInvalidRequest)
	}
	user := strings.TrimSpace(req.User)
	if user == "" {
		return SearchResponse{}, fmt.Errorf("%w: user is required", ErrInvalidRequest)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = s.cfg.DefaultLimit
	}
	if limit > s.cfg.MaxLimit {
		limit = s.cfg.MaxLimit
	}

	qHash := queryHash(query)
	var searchAfter []interface{}
	if req.Cursor != "" {
		decoded, err := DecodeCursor(req.Cursor)
		if err != nil {
			return SearchResponse{}, err
		}
		if err := ValidateCursor(decoded, org, qHash, limit); err != nil {
			return SearchResponse{}, err
		}
		searchAfter = decoded.SearchAfter
	}

	indexRef, err := s.resolver.ResolveIndex(ctx, org)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("%w: resolve search index: %v", ErrBackend, err)
	}

	log.Debug().
		Str("organization", org).
		Str("queryHash", qHash).
		Int("limit", limit).
		Str("index", indexRef.IndexName).
		Msg("starting search")

	results := make([]SearchHit, 0, limit)
	var nextSearchAfter []interface{}
	var totalScanned int
	var exhausted bool

outer:
	for len(results) < limit {
		page, err := s.searcher.Search(ctx, indexRef.IndexName, query, s.cfg.FetchBatchSize, searchAfter)
		s.metrics.AddOpenSearchCalls(1)
		if err != nil {
			return SearchResponse{}, fmt.Errorf("%w: query OpenSearch: %v", ErrBackend, err)
		}
		if len(page.Hits) == 0 {
			exhausted = true
			break
		}

		authz, err := s.authorizer.FilterAuthorized(ctx, AuthorizationRequest{
			Organization: org,
			User:         user,
			Relation:     "get",
			Hits:         page.Hits,
		})
		if err != nil {
			return SearchResponse{}, fmt.Errorf("%w: filter authorization: %v", ErrBackend, err)
		}
		s.metrics.AddOpenFGACalls(authz.Calls)
		s.metrics.AddDroppedMissingContext(authz.DroppedMissingContext)
		s.metrics.AddAuthDenied(authz.Denied)

		for i, hit := range page.Hits {
			totalScanned++
			nextSearchAfter = hit.Sort

			if totalScanned > s.cfg.MaxScannedHits {
				break outer
			}

			if i >= len(authz.Allowed) || !authz.Allowed[i] {
				continue
			}

			results = append(results, mapHit(hit))
			if len(results) == limit {
				break outer
			}
		}

		if len(page.Hits) < s.cfg.FetchBatchSize {
			exhausted = true
			break
		}
		searchAfter = page.Hits[len(page.Hits)-1].Sort
	}

	var nextCursor *string
	if !exhausted && len(nextSearchAfter) > 0 {
		cursor, err := EncodeCursor(CursorState{
			Version:     cursorVersion,
			Org:         org,
			QueryHash:   qHash,
			Limit:       limit,
			SearchAfter: nextSearchAfter,
		})
		if err != nil {
			return SearchResponse{}, err
		}
		nextCursor = &cursor
	}

	return SearchResponse{Results: results, NextCursor: nextCursor}, nil
}

func mapHit(hit OpenSearchHit) SearchHit {
	src := hit.Source
	if src == nil {
		src = map[string]interface{}{}
	}
	return SearchHit{
		ID:               firstString(hit.ID, stringFromMap(src, "id")),
		Score:            hit.Score,
		Kind:             stringFromMap(src, "kind"),
		Name:             stringFromMap(src, "name"),
		Namespace:        stringFromMap(src, "namespace"),
		APIGroup:         stringFromMap(src, "api_group"),
		APIVersion:       stringFromMap(src, "api_version"),
		WorkspacePath:    stringFromMap(src, "workspace_path"),
		ClusterName:      stringFromMap(src, "cluster_name"),
		OrganizationID:   stringFromMap(src, "organization_id"),
		OrganizationName: stringFromMap(src, "organization_name"),
		AccountID:        stringFromMap(src, "account_id"),
		AccountName:      stringFromMap(src, "account_name"),
		Source:           src,
	}
}

func stringFromMap(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
