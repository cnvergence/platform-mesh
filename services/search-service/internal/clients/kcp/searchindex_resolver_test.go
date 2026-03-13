package kcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/platform-mesh/search/internal/config"
	"k8s.io/client-go/rest"
)

func TestResolveIndexMatchesByOrganizationClusterID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/root:orgs/apis/tenancy.kcp.io/v1alpha1/workspaces/acme":
			writeJSON(t, w, workspacePayload("acme", "cluster-123"))
		case "/clusters/root:orgs/apis/core.platform-mesh.io/v1alpha1/searchindices":
			writeJSON(t, w, searchIndexListPayload(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"metadata": map[string]interface{}{"name": "acme"},
						"spec":     map[string]interface{}{"organizationClusterID": "cluster-other"},
						"status":   map[string]interface{}{"indexName": "pm-orgs-cluster-other"},
					},
					{
						"metadata": map[string]interface{}{"name": "searchindex-acme"},
						"spec":     map[string]interface{}{"organizationClusterID": "cluster-123"},
						"status":   map[string]interface{}{"indexName": "pm-orgs-cluster-123"},
					},
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := newTestResolver(t, server.URL)
	ref, err := resolver.ResolveIndex(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ResolveIndex returned error: %v", err)
	}
	if ref.IndexName != "pm-orgs-cluster-123" {
		t.Fatalf("unexpected index name: %s", ref.IndexName)
	}
	if ref.OrganizationClusterID != "cluster-123" {
		t.Fatalf("unexpected org cluster id: %s", ref.OrganizationClusterID)
	}
}

func TestResolveIndexFallsBackToResourceName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/root:orgs/apis/tenancy.kcp.io/v1alpha1/workspaces/acme":
			writeJSON(t, w, workspacePayload("acme", "cluster-123"))
		case "/clusters/root:orgs/apis/core.platform-mesh.io/v1alpha1/searchindices":
			writeJSON(t, w, searchIndexListPayload(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"metadata": map[string]interface{}{"name": "acme"},
						"spec":     map[string]interface{}{"organizationClusterID": ""},
						"status":   map[string]interface{}{"indexName": "pm-orgs-static"},
					},
					{
						"metadata": map[string]interface{}{"name": "other"},
						"spec":     map[string]interface{}{"organizationClusterID": "cluster-other"},
						"status":   map[string]interface{}{"indexName": "pm-orgs-cluster-other"},
					},
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := newTestResolver(t, server.URL)
	ref, err := resolver.ResolveIndex(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ResolveIndex returned error: %v", err)
	}
	if ref.IndexName != "pm-orgs-static" {
		t.Fatalf("unexpected index name: %s", ref.IndexName)
	}
	if ref.OrganizationClusterID != "cluster-123" {
		t.Fatalf("expected workspace cluster id fallback, got %s", ref.OrganizationClusterID)
	}
}

func TestResolveIndexFallsBackToSpecOrganizationClusterIDWhenStatusEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/root:orgs/apis/tenancy.kcp.io/v1alpha1/workspaces/acme":
			writeJSON(t, w, workspacePayload("acme", "cluster-123"))
		case "/clusters/root:orgs/apis/core.platform-mesh.io/v1alpha1/searchindices":
			writeJSON(t, w, searchIndexListPayload(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"metadata": map[string]interface{}{"name": "searchindex-acme"},
						"spec":     map[string]interface{}{"organizationClusterID": "cluster-123"},
						"status":   map[string]interface{}{"indexName": ""},
					},
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := newTestResolver(t, server.URL)
	ref, err := resolver.ResolveIndex(context.Background(), "acme")
	if err != nil {
		t.Fatalf("ResolveIndex returned error: %v", err)
	}
	if ref.IndexName != "cluster-123" {
		t.Fatalf("expected spec.organizationClusterID fallback index name, got %s", ref.IndexName)
	}
}

func TestResolveIndexReturnsErrorWhenClusterMatchIsAmbiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clusters/root:orgs/apis/tenancy.kcp.io/v1alpha1/workspaces/acme":
			writeJSON(t, w, workspacePayload("acme", "cluster-123"))
		case "/clusters/root:orgs/apis/core.platform-mesh.io/v1alpha1/searchindices":
			writeJSON(t, w, searchIndexListPayload(map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"metadata": map[string]interface{}{"name": "idx-1"},
						"spec":     map[string]interface{}{"organizationClusterID": "cluster-123"},
						"status":   map[string]interface{}{"indexName": "pm-orgs-cluster-123-a"},
					},
					{
						"metadata": map[string]interface{}{"name": "idx-2"},
						"spec":     map[string]interface{}{"organizationClusterID": "cluster-123"},
						"status":   map[string]interface{}{"indexName": "pm-orgs-cluster-123-b"},
					},
				},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := newTestResolver(t, server.URL)
	if _, err := resolver.ResolveIndex(context.Background(), "acme"); err == nil {
		t.Fatalf("expected error for ambiguous cluster matches")
	}
}

func newTestResolver(t *testing.T, base string) *SearchIndexResolver {
	t.Helper()

	resolver, err := NewSearchIndexResolver(&rest.Config{Host: base}, config.SearchIndexConfig{
		WorkspacePath: "root:orgs",
		Group:         "core.platform-mesh.io",
		Version:       "v1alpha1",
		Resource:      "searchindices",
	}, nil)
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}

	return resolver
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}

func workspacePayload(name, clusterID string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "tenancy.kcp.io/v1alpha1",
		"kind":       "Workspace",
		"metadata": map[string]interface{}{
			"name": name,
		},
		"spec": map[string]interface{}{
			"cluster": clusterID,
		},
	}
}

func searchIndexListPayload(payload map[string]interface{}) map[string]interface{} {
	payload["apiVersion"] = "core.platform-mesh.io/v1alpha1"
	payload["kind"] = "SearchIndexList"
	return payload
}
