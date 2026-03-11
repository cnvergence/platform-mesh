package opensearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/platform-mesh/search/internal/service/search"
)

type Config struct {
	URL      string
	Username string
	Password string
	Insecure bool
	Timeout  time.Duration
}

type Client struct {
	baseURL  *url.URL
	http     *http.Client
	username string
	password string
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("OpenSearch URL is required")
	}
	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse OpenSearch URL: %w", err)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if parsed.Scheme == "https" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.Insecure}
	}

	return &Client{
		baseURL: parsed,
		http: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
		username: cfg.Username,
		password: cfg.Password,
	}, nil
}

func BuildQueryBody(query string, size int, searchAfter []interface{}) ([]byte, error) {
	body := map[string]interface{}{
		"size": size,
		"query": map[string]interface{}{
			"simple_query_string": map[string]interface{}{
				"query":            query,
				"fields":           []string{"*"},
				"default_operator": "and",
			},
		},
		"sort": []map[string]string{
			{"_score": "desc"},
			{"_id": "asc"},
		},
	}

	if len(searchAfter) > 0 {
		body["search_after"] = searchAfter
	}

	return json.Marshal(body)
}

func (c *Client) Search(ctx context.Context, indexName, query string, size int, searchAfter []interface{}) (search.OpenSearchPage, error) {
	body, err := BuildQueryBody(query, size, searchAfter)
	if err != nil {
		return search.OpenSearchPage{}, fmt.Errorf("build OpenSearch query body: %w", err)
	}

	requestURL := c.baseURL.ResolveReference(&url.URL{Path: fmt.Sprintf("/%s/_search", indexName)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return search.OpenSearchPage{}, fmt.Errorf("create OpenSearch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return search.OpenSearchPage{}, fmt.Errorf("execute OpenSearch request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return search.OpenSearchPage{}, fmt.Errorf("OpenSearch request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var payload struct {
		Hits struct {
			Hits []struct {
				ID     string                 `json:"_id"`
				Score  float64                `json:"_score"`
				Sort   []interface{}          `json:"sort"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return search.OpenSearchPage{}, fmt.Errorf("decode OpenSearch response: %w", err)
	}

	hits := make([]search.OpenSearchHit, 0, len(payload.Hits.Hits))
	for _, hit := range payload.Hits.Hits {
		hits = append(hits, search.OpenSearchHit{
			ID:     hit.ID,
			Score:  hit.Score,
			Sort:   hit.Sort,
			Source: hit.Source,
		})
	}

	return search.OpenSearchPage{Hits: hits}, nil
}
