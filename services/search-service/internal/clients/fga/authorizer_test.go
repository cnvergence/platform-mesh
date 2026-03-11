package fga

import (
	"reflect"
	"testing"

	"github.com/platform-mesh/search/internal/service/search"
)

func TestBuildBatchCheckItemResourceObjectFormat(t *testing.T) {
	hit := search.OpenSearchHit{Source: map[string]interface{}{
		"kind":            "Component",
		"name":            "my-component",
		"namespace":       "dev",
		"api_group":       "core.platform-mesh.io",
		"organization_id": "orgcluster1",
		"account_id":      "acccluster1",
		"account_name":    "account-a",
	}}

	item, missing := buildBatchCheckItem("alice@example.com", "get", 0, hit)
	if missing {
		t.Fatalf("expected context to be valid")
	}
	if item.TupleKey.Relation != "get" {
		t.Fatalf("unexpected relation: %s", item.TupleKey.Relation)
	}
	expected := "core_platform-mesh_io_component:acccluster1/dev/my-component"
	if item.TupleKey.Object != expected {
		t.Fatalf("unexpected object: %s", item.TupleKey.Object)
	}
	if len(item.ContextualTuples.TupleKeys) == 0 {
		t.Fatalf("expected contextual tuples")
	}
}

func TestBuildBatchCheckItemDropsMissingAuthContext(t *testing.T) {
	hit := search.OpenSearchHit{Source: map[string]interface{}{
		"kind":            "Component",
		"name":            "my-component",
		"namespace":       "dev",
		"api_group":       "core.platform-mesh.io",
		"organization_id": "orgcluster1",
		// account_name intentionally missing for namespaced resources
	}}

	_, missing := buildBatchCheckItem("alice@example.com", "get", 0, hit)
	if !missing {
		t.Fatalf("expected missing auth context")
	}
}

func TestChunkRanges(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		chunkSize int
		want      [][2]int
	}{
		{
			name:      "empty",
			total:     0,
			chunkSize: 100,
			want:      nil,
		},
		{
			name:      "single chunk exact",
			total:     100,
			chunkSize: 100,
			want:      [][2]int{{0, 100}},
		},
		{
			name:      "single chunk partial",
			total:     50,
			chunkSize: 100,
			want:      [][2]int{{0, 50}},
		},
		{
			name:      "multiple chunks",
			total:     250,
			chunkSize: 100,
			want:      [][2]int{{0, 100}, {100, 200}, {200, 250}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkRanges(tt.total, tt.chunkSize)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("chunkRanges(%d, %d) = %#v, want %#v", tt.total, tt.chunkSize, got, tt.want)
			}
		})
	}
}
