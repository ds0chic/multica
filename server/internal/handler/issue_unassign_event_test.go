package handler

import (
	"net/http"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Unassigning sends explicit nulls for both assignee fields. The issue:updated
// event must still report assignee_changed, or the activity row, the previous
// assignee's inbox item and other clients' assignee views never hear about it.

func captureIssueUpdated(t *testing.T, issueID string) func() []map[string]any {
	t.Helper()
	var mu sync.Mutex
	var payloads []map[string]any
	testHandler.Bus.Subscribe(protocol.EventIssueUpdated, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		if resp, ok := payload["issue"].(IssueResponse); ok && resp.ID == issueID {
			mu.Lock()
			payloads = append(payloads, payload)
			mu.Unlock()
		}
	})
	return func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), payloads...)
	}
}

func assignedIssue(t *testing.T, title string) string {
	t.Helper()
	return dbfx.Issue(t, title, testutil.Cols{
		"status":        "todo",
		"assignee_type": "member",
		"assignee_id":   testutil.Raw("'" + testUserID + "'::uuid"),
	})
}

func TestUpdateIssueUnassignReportsAssigneeChanged(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := assignedIssue(t, "unassign-single")
	events := captureIssueUpdated(t, issueID)

	var resp IssueResponse
	testutil.Call(t, testHandler.UpdateIssue, updateIssueRequest(issueID, map[string]any{
		"assignee_type": nil,
		"assignee_id":   nil,
	})).Want(http.StatusOK).JSON(&resp)
	if resp.AssigneeID != nil {
		t.Fatalf("assignee_id = %v, want nil", *resp.AssigneeID)
	}

	got := events()
	if len(got) != 1 {
		t.Fatalf("got %d issue:updated events, want 1", len(got))
	}
	if got[0]["assignee_changed"] != true {
		t.Errorf("assignee_changed = %v, want true", got[0]["assignee_changed"])
	}
}

func TestUpdateIssueUntouchedAssigneeIsNotChanged(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := assignedIssue(t, "unassign-untouched")
	events := captureIssueUpdated(t, issueID)

	testutil.Call(t, testHandler.UpdateIssue, updateIssueRequest(issueID, map[string]any{
		"priority": "high",
	})).Want(http.StatusOK)

	got := events()
	if len(got) != 1 {
		t.Fatalf("got %d issue:updated events, want 1", len(got))
	}
	if got[0]["assignee_changed"] != false {
		t.Errorf("assignee_changed = %v, want false", got[0]["assignee_changed"])
	}
}

func TestBatchUpdateUnassignReportsAssigneeChanged(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := assignedIssue(t, "unassign-batch")
	events := captureIssueUpdated(t, issueID)

	testutil.Call(t, testHandler.BatchUpdateIssues, newRequest("POST", "/api/issues/batch-update", map[string]any{
		"issue_ids": []string{issueID},
		"updates":   map[string]any{"assignee_type": nil, "assignee_id": nil},
	})).Want(http.StatusOK)

	got := events()
	if len(got) != 1 {
		t.Fatalf("got %d issue:updated events, want 1", len(got))
	}
	if got[0]["assignee_changed"] != true {
		t.Errorf("assignee_changed = %v, want true", got[0]["assignee_changed"])
	}
}
