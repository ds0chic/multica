package main

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// An agent moving its own issue todo -> in_progress is routine progress; the
// person who filed the issue should not get an inbox row for it. Handoff
// statuses (in_review, done, ...) still reach them, and so does a status change
// made by a person.
func TestStatusChanged_AgentRoutineProgressSkipsDirectSubscribers(t *testing.T) {
	ctx := context.Background()
	queries := db.New(testPool)
	bus := events.New()

	agentID, _ := firstFixtureAgent(t)
	issueID := createTestIssue(t, testWorkspaceID, testUserID)
	t.Cleanup(func() { cleanupTestIssue(t, issueID) })
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM inbox_item WHERE issue_id = $1`, issueID)
	})

	if _, err := queries.AddIssueSubscriber(ctx, db.AddIssueSubscriberParams{
		IssueID:  util.MustParseUUID(issueID),
		UserType: "member",
		UserID:   util.MustParseUUID(testUserID),
		Reason:   "creator",
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	countInbox := func() int {
		t.Helper()
		var n int
		if err := testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM inbox_item WHERE recipient_type='member' AND recipient_id=$1 AND issue_id=$2`,
			testUserID, issueID,
		).Scan(&n); err != nil {
			t.Fatalf("count inbox: %v", err)
		}
		return n
	}
	agentEvent := events.Event{
		Type:        protocol.EventIssueUpdated,
		WorkspaceID: testWorkspaceID,
		ActorType:   "agent",
		ActorID:     agentID,
	}

	notifySubscribers(ctx, queries, bus, issueID, "in_progress", testWorkspaceID,
		agentEvent, nil, "status_changed", "info", "issue", "", emptyDetails)
	if n := countInbox(); n != 0 {
		t.Fatalf("agent's routine in_progress reached the creator %d time(s), want 0", n)
	}

	notifySubscribers(ctx, queries, bus, issueID, "in_review", testWorkspaceID,
		agentEvent, nil, "status_changed", "info", "issue", "", emptyDetails)
	if n := countInbox(); n != 1 {
		t.Fatalf("agent's hand-off to in_review: got %d inbox row(s), want 1", n)
	}
}
