package main

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/inbox"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// Inbox items written outside the event listeners (quick create, sub-issues
// done, a run that exhausted its retries) and reaction notices used to ignore
// the recipient's mute settings. Muting a group must silence every type that
// belongs to it, whoever writes the item.
func TestCreateIfEligibleHonorsMutedGroup(t *testing.T) {
	fx := workspaceFixture(t)
	member := fx.User(t, "Muter", "inbox-mute-all-types@example.test")
	fx.Member(t, testWorkspaceID, member, "member")
	issue := fx.Issue(t, "Mute every type")
	fx.Cleanup(t, `DELETE FROM inbox_item WHERE issue_id = $1`, issue)
	fx.Cleanup(t, `DELETE FROM notification_preference WHERE user_id = $1`, member)
	fx.Exec(t, `INSERT INTO notification_preference (workspace_id, user_id, preferences)
		VALUES ($1, $2, '{"agent_activity":"muted","status_changes":"muted","comments":"muted"}')`, testWorkspaceID, member)
	queries := db.New(testPool)

	for _, tc := range []struct {
		notifType string
		delivered bool
	}{
		{"quick_create_done", false},
		{"quick_create_failed", false},
		{"task_failed", false},
		{"children_done", false},
		{"reaction_added", false},
		{"mentioned", true},
		// Needs the owner to act; not mutable.
		{"autopilot_paused", true},
	} {
		t.Run(tc.notifType, func(t *testing.T) {
			_, inserted, err := inbox.CreateIfEligible(context.Background(), queries, db.CreateInboxItemParams{
				ID: dbid.NewV7(), WorkspaceID: util.MustParseUUID(testWorkspaceID),
				RecipientType: "member", RecipientID: util.MustParseUUID(member),
				IssueID: util.MustParseUUID(issue), Type: tc.notifType, Severity: "info",
				Title: "Mute every type", Details: emptyDetails,
			})
			if err != nil {
				t.Fatal(err)
			}
			if inserted != tc.delivered {
				t.Fatalf("%s: inserted = %v, want %v", tc.notifType, inserted, tc.delivered)
			}
		})
	}
}
