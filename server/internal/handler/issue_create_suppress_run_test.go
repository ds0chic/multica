package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Creating an issue already assigned to an agent starts a run immediately.
// suppress_run on create keeps the assignment but holds the run, the same
// control updates already have ("Don't start yet").
func TestCreateIssueSuppressRunHoldsAssignedRun(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "CreateSuppressRun", []byte("[]"))

	create := func(title string, suppress bool) string {
		body := map[string]any{
			"title":         title,
			"status":        "todo",
			"assignee_type": "agent",
			"assignee_id":   agentID,
		}
		if suppress {
			body["suppress_run"] = true
		}
		var out IssueResponse
		testutil.Call(t, testHandler.CreateIssue,
			newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, body),
		).Want(http.StatusCreated).JSON(&out)
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id = $1`, out.ID)
			testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, out.ID)
		})
		return out.ID
	}
	taskCount := func(issueID string) int {
		var n int
		if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE issue_id = $1`, issueID).Scan(&n); err != nil {
			t.Fatalf("count tasks: %v", err)
		}
		return n
	}

	started := create("create and start", false)
	if got := taskCount(started); got != 1 {
		t.Fatalf("plain create: want 1 queued task, got %d", got)
	}

	held := create("create but hold", true)
	if got := taskCount(held); got != 0 {
		t.Fatalf("suppress_run create: want no task, got %d", got)
	}
	var assigneeID string
	if err := testPool.QueryRow(ctx, `SELECT assignee_id::text FROM issue WHERE id = $1`, held).Scan(&assigneeID); err != nil {
		t.Fatalf("read assignee: %v", err)
	}
	if assigneeID != agentID {
		t.Fatalf("suppress_run must keep the assignment: want %s, got %s", agentID, assigneeID)
	}
}
