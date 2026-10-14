package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/cli"
)

// --project accepts a project's title as well as a UUID or UUID prefix, the way
// --assignee accepts a name.
func TestResolveProjectIDByTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"projects": []map[string]any{
			{"id": "aaaaaaaa-0000-4000-8000-000000000001", "title": "Website", "status": "planned"},
			{"id": "bbbbbbbb-0000-4000-8000-000000000002", "title": "Mobile App", "status": "planned"},
			{"id": "cccccccc-0000-4000-8000-000000000003", "title": "Mobile app", "status": "planned"},
			{"id": "dddddddd-0000-4000-8000-000000000004", "title": "beef", "status": "planned"},
		}})
	}))
	defer srv.Close()
	client := cli.NewAPIClient(srv.URL, "ws-1", "test-token")
	ctx := context.Background()

	got, err := resolveProjectID(ctx, client, "website")
	if err != nil || got.ID != "aaaaaaaa-0000-4000-8000-000000000001" {
		t.Fatalf("resolve by title = %+v, %v; want the Website project", got, err)
	}

	// Two titles differing only in case: the exact-case one wins, a bare
	// case-insensitive match would be ambiguous.
	got, err = resolveProjectID(ctx, client, "Mobile App")
	if err != nil || got.ID != "bbbbbbbb-0000-4000-8000-000000000002" {
		t.Fatalf("resolve by exact title = %+v, %v; want the exact-case match", got, err)
	}

	_, err = resolveProjectID(ctx, client, "MOBILE APP")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("resolve by case-insensitive title error = %v, want ambiguous", err)
	}

	// An id prefix still resolves by id.
	got, err = resolveProjectID(ctx, client, "aaaaaaaa")
	if err != nil || got.ID != "aaaaaaaa-0000-4000-8000-000000000001" {
		t.Fatalf("resolve by prefix = %+v, %v", got, err)
	}

	// A title that is also valid hex resolves to the title, not a prefix.
	got, err = resolveProjectID(ctx, client, "beef")
	if err != nil || got.ID != "dddddddd-0000-4000-8000-000000000004" {
		t.Fatalf("resolve hex-looking title = %+v, %v", got, err)
	}

	_, err = resolveProjectID(ctx, client, "nonexistent project")
	if err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("unknown project error = %v, want a project-not-found error", err)
	}
}
