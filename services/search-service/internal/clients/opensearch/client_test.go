package opensearch

import (
	"encoding/json"
	"testing"
)

func TestBuildQueryBodyWithoutSearchAfter(t *testing.T) {
	body, err := BuildQueryBody("hello", 20, nil)
	if err != nil {
		t.Fatalf("BuildQueryBody returned error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if payload["size"].(float64) != 20 {
		t.Fatalf("unexpected size: %v", payload["size"])
	}
	if _, ok := payload["search_after"]; ok {
		t.Fatalf("search_after should not be set")
	}

	sort := payload["sort"].([]interface{})
	if len(sort) != 2 {
		t.Fatalf("expected 2 sort fields")
	}
}

func TestBuildQueryBodyWithSearchAfter(t *testing.T) {
	body, err := BuildQueryBody("hello", 10, []interface{}{1.0, "id-1"})
	if err != nil {
		t.Fatalf("BuildQueryBody returned error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	searchAfter := payload["search_after"].([]interface{})
	if len(searchAfter) != 2 {
		t.Fatalf("expected 2 search_after values")
	}
}
