package search

import (
	"context"
	"errors"
	"testing"
)

type fakeResolver struct {
	index SearchIndexRef
	err   error
}

func (f fakeResolver) ResolveIndex(ctx context.Context, org string) (SearchIndexRef, error) {
	return f.index, f.err
}

type fakeSearcher struct {
	pages []OpenSearchPage
	calls int
}

func (f *fakeSearcher) Search(ctx context.Context, indexName, query string, size int, searchAfter []interface{}) (OpenSearchPage, error) {
	if f.calls >= len(f.pages) {
		return OpenSearchPage{}, nil
	}
	page := f.pages[f.calls]
	f.calls++
	return page, nil
}

type fakeAuthorizer struct {
	results []AuthorizationResult
	calls   int
}

func (f *fakeAuthorizer) FilterAuthorized(ctx context.Context, req AuthorizationRequest) (AuthorizationResult, error) {
	if f.calls >= len(f.results) {
		return AuthorizationResult{Allowed: make([]bool, len(req.Hits))}, nil
	}
	res := f.results[f.calls]
	f.calls++
	return res, nil
}

func TestSearchFillsAuthorizedPageAcrossBatches(t *testing.T) {
	searcher := &fakeSearcher{pages: []OpenSearchPage{
		{Hits: []OpenSearchHit{
			{ID: "1", Score: 1, Sort: []interface{}{1.0, "1"}, Source: map[string]interface{}{"id": "1"}},
			{ID: "2", Score: 1, Sort: []interface{}{0.9, "2"}, Source: map[string]interface{}{"id": "2"}},
		}},
		{Hits: []OpenSearchHit{
			{ID: "3", Score: 1, Sort: []interface{}{0.8, "3"}, Source: map[string]interface{}{"id": "3"}},
			{ID: "4", Score: 1, Sort: []interface{}{0.7, "4"}, Source: map[string]interface{}{"id": "4"}},
		}},
	}}
	authorizer := &fakeAuthorizer{results: []AuthorizationResult{
		{Allowed: []bool{false, true}, Denied: 1, Calls: 1},
		{Allowed: []bool{true, false}, Denied: 1, Calls: 1},
	}}

	svc := NewService(
		fakeResolver{index: SearchIndexRef{IndexName: "idx-acme"}},
		searcher,
		authorizer,
		nil,
		ServiceConfig{DefaultLimit: 20, MaxLimit: 100, FetchBatchSize: 2, MaxScannedHits: 1000},
	)

	resp, err := svc.Search(context.Background(), SearchRequest{Organization: "acme", User: "alice@example.com", Query: "foo", Limit: 2})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.NextCursor == nil {
		t.Fatalf("expected non-nil next cursor")
	}
}

func TestSearchInvalidCursor(t *testing.T) {
	svc := NewService(
		fakeResolver{index: SearchIndexRef{IndexName: "idx"}},
		&fakeSearcher{},
		&fakeAuthorizer{},
		nil,
		ServiceConfig{},
	)

	_, err := svc.Search(context.Background(), SearchRequest{
		Organization: "acme",
		User:         "alice@example.com",
		Query:        "foo",
		Limit:        20,
		Cursor:       "not-a-cursor",
	})
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("expected ErrInvalidCursor, got %v", err)
	}
}

func TestSearchRejectsMissingQuery(t *testing.T) {
	svc := NewService(fakeResolver{}, &fakeSearcher{}, &fakeAuthorizer{}, nil, ServiceConfig{})
	_, err := svc.Search(context.Background(), SearchRequest{Organization: "acme", User: "alice@example.com", Query: "  "})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestSearchClampsLimitToConfiguredMax(t *testing.T) {
	searcher := &fakeSearcher{pages: []OpenSearchPage{
		{Hits: []OpenSearchHit{
			{ID: "1", Score: 1, Sort: []interface{}{1.0, "1"}, Source: map[string]interface{}{"id": "1"}},
		}},
		{Hits: []OpenSearchHit{
			{ID: "2", Score: 1, Sort: []interface{}{0.9, "2"}, Source: map[string]interface{}{"id": "2"}},
		}},
	}}
	authorizer := &fakeAuthorizer{results: []AuthorizationResult{
		{Allowed: []bool{true}, Calls: 1},
		{Allowed: []bool{true}, Calls: 1},
	}}

	svc := NewService(
		fakeResolver{index: SearchIndexRef{IndexName: "idx-acme"}},
		searcher,
		authorizer,
		nil,
		ServiceConfig{DefaultLimit: 20, MaxLimit: 100, FetchBatchSize: 1, MaxScannedHits: 1},
	)

	resp, err := svc.Search(context.Background(), SearchRequest{
		Organization: "acme",
		User:         "alice@example.com",
		Query:        "foo",
		Limit:        500,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if resp.NextCursor == nil {
		t.Fatalf("expected next cursor when scan cap is reached")
	}

	decoded, err := DecodeCursor(*resp.NextCursor)
	if err != nil {
		t.Fatalf("decode next cursor: %v", err)
	}
	if decoded.Limit != 100 {
		t.Fatalf("expected clamped limit 100, got %d", decoded.Limit)
	}
}
