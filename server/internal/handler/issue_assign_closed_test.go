package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAssignAgentToClosedIssueStartsNoRun covers handing a done or cancelled
// issue to an agent (e.g. dragging a closed card onto an agent's lane). Closed
// work stays closed: neither the preview nor the write starts a run, the same
// way moving an assigned issue from backlog to done starts none.
func TestAssignAgentToClosedIssueStartsNoRun(t *testing.T) {
	agentID := seededReadyAgentID(t)

	for _, status := range []string{"done", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			issue := createIssueForTest(t, map[string]any{
				"title":  "assign closed " + status,
				"status": status,
			})

			preview := previewIssueTrigger(t, map[string]any{
				"issue_ids":     []string{issue.ID},
				"assignee_type": "agent",
				"assignee_id":   agentID,
			})
			if preview.TotalCount != 0 {
				t.Fatalf("preview: expected 0 triggers for a %s issue, got %+v", status, preview)
			}

			w := httptest.NewRecorder()
			req := withURLParam(newRequest("PUT", "/api/issues/"+issue.ID, map[string]any{
				"assignee_type": "agent",
				"assignee_id":   agentID,
			}), "id", issue.ID)
			testHandler.UpdateIssue(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("UpdateIssue: expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if n := taskCountFor(t, issue.ID, agentID); n != 0 {
				t.Fatalf("assigning a %s issue enqueued %d task(s), want 0", status, n)
			}
		})
	}

	// Creating straight into a closed status with an agent assignee is the
	// same hand-off and starts nothing either.
	created := createIssueForTest(t, map[string]any{
		"title":         "create closed with agent",
		"status":        "done",
		"assignee_type": "agent",
		"assignee_id":   agentID,
	})
	if n := taskCountFor(t, created.ID, agentID); n != 0 {
		t.Fatalf("creating a done issue with an agent enqueued %d task(s), want 0", n)
	}
}
