package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Reassigning an issue from agent A to agent B must withdraw A's run that has
// not started yet; otherwise both agents end up working the same issue.
// Runs already in progress are left alone (they are stopped explicitly).
func TestReassignCancelsPreviousAgentsQueuedRun(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentA := createHandlerTestAgent(t, "ReassignQueuedA", []byte("[]"))
	agentB := createHandlerTestAgent(t, "ReassignQueuedB", []byte("[]"))

	var created IssueResponse
	testutil.Call(t, testHandler.CreateIssue,
		newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
			"title":         "reassign while queued",
			"status":        "todo",
			"assignee_type": "agent",
			"assignee_id":   agentA,
		}),
	).Want(http.StatusCreated).JSON(&created)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id = $1`, created.ID)
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, created.ID)
	})

	statusOf := func(agentID string) []string {
		rows, err := testPool.Query(ctx, `SELECT status FROM agent_task_queue WHERE issue_id = $1 AND agent_id = $2 ORDER BY created_at`, created.ID, agentID)
		if err != nil {
			t.Fatalf("query tasks: %v", err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, s)
		}
		return out
	}
	if got := statusOf(agentA); len(got) != 1 || got[0] != "queued" {
		t.Fatalf("A after create: want [queued], got %v", got)
	}

	req := newRequest("PUT", "/api/issues/"+created.ID, map[string]any{
		"assignee_type": "agent",
		"assignee_id":   agentB,
	})
	req = withURLParam(req, "id", created.ID)
	testutil.Call(t, testHandler.UpdateIssue, req).Want(http.StatusOK)

	if got := statusOf(agentA); len(got) != 1 || got[0] != "cancelled" {
		t.Fatalf("A after reassign: want [cancelled], got %v", got)
	}
	if got := statusOf(agentB); len(got) != 1 || got[0] != "queued" {
		t.Fatalf("B after reassign: want [queued], got %v", got)
	}
}
