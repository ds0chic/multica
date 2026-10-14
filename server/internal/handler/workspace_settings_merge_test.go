package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// A settings write is merged into what is stored: a client that sends only the
// key it changed (or a stale copy missing a key) must not drop settings another
// writer added, such as the child-done wakeup default the server merges in.
func TestUpdateWorkspaceMergesSettings(t *testing.T) {
	if testHandler == nil {
		t.Skip("handler test fixture not initialized (no DB?)")
	}
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Settings merge", "slug": "settings-merge", "description": "", "issue_prefix": "SMG",
		"settings": testutil.Raw(`'{"system_wakeup_child_done": false, "system_wakeup_child_done_instruction": "wrap up", "pr_merge_status": "none", "pr_auto_complete_enabled": false}'::jsonb`),
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, wsID, testUserID)

	save := func(t *testing.T, settings any, want int) map[string]any {
		t.Helper()
		req := withURLParam(newRequest("PATCH", "/api/workspaces/"+wsID, map[string]any{"settings": settings}), "id", wsID)
		testutil.Call(t, testHandler.UpdateWorkspace, req).Want(want)
		ws, err := testHandler.Queries.GetWorkspace(context.Background(), parseUUID(wsID))
		if err != nil {
			t.Fatalf("GetWorkspace: %v", err)
		}
		var stored map[string]any
		if err := json.Unmarshal(ws.Settings, &stored); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		return stored
	}

	got := save(t, map[string]any{"github_pr_sidebar_enabled": false}, http.StatusOK)
	if got["github_pr_sidebar_enabled"] != false {
		t.Errorf("github_pr_sidebar_enabled = %v, want false", got["github_pr_sidebar_enabled"])
	}
	if got["system_wakeup_child_done"] != false || got["system_wakeup_child_done_instruction"] != "wrap up" {
		t.Errorf("child-done default was dropped: %v", got)
	}
	if got["pr_merge_status"] != "none" || got["pr_auto_complete_enabled"] != false {
		t.Errorf("PR merge choice changed by an unrelated write: %v", got)
	}

	got = save(t, map[string]any{"pr_merge_status": "in_review"}, http.StatusOK)
	if got["pr_merge_status"] != "in_review" {
		t.Errorf("pr_merge_status = %v, want in_review", got["pr_merge_status"])
	}
	if v, present := got["pr_auto_complete_enabled"]; present {
		t.Errorf("retired switch = %v, want removed once a target is chosen", v)
	}
	if got["system_wakeup_child_done"] != false || got["github_pr_sidebar_enabled"] != false {
		t.Errorf("other settings dropped: %v", got)
	}

	save(t, []any{"not", "an", "object"}, http.StatusBadRequest)
}
