package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// An automatic retry must not restart an assignment run on an issue the user
// closed while the run was failing; an open issue still gets its retry.
func TestAutoRetrySkipsClosedIssue(t *testing.T) {
	pool := newResolveOriginatorPool(t)
	ctx := context.Background()
	q := db.New(pool)
	_, _, agentID, issueID := seedAttributionFixture(t, pool)
	svc := &TaskService{Queries: q, TxStarter: pool, Bus: events.New()}

	var runtimeID string
	if err := pool.QueryRow(ctx, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID); err != nil {
		t.Fatalf("read agent runtime: %v", err)
	}

	setIssueStatus := func(t *testing.T, status string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE issue SET status = $2 WHERE id = $1`, issueID, status); err != nil {
			t.Fatalf("set issue status: %v", err)
		}
	}
	insertParent := func(t *testing.T, status string) pgtype.UUID {
		t.Helper()
		var parentID pgtype.UUID
		if err := pool.QueryRow(ctx, `
			INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority, attempt, max_attempts, failure_reason)
			VALUES ($1, $2, $3, $4, 0, 1, 2, CASE WHEN $4 = 'failed' THEN 'agent_error.provider_network' END)
			RETURNING id
		`, agentID, runtimeID, issueID, status).Scan(&parentID); err != nil {
			t.Fatalf("insert parent task: %v", err)
		}
		t.Cleanup(func() {
			pool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE parent_task_id = $1 OR id = $1`, parentID)
		})
		return parentID
	}
	childCount := func(t *testing.T, parentID pgtype.UUID) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE parent_task_id = $1`, parentID).Scan(&n); err != nil {
			t.Fatalf("count children: %v", err)
		}
		return n
	}

	for _, tc := range []struct {
		status    string
		wantChild bool
	}{
		{"todo", true},
		{"done", false},
		{"cancelled", false},
	} {
		t.Run("FailTask/"+tc.status, func(t *testing.T) {
			setIssueStatus(t, tc.status)
			parentID := insertParent(t, "running")
			if _, err := svc.FailTask(ctx, parentID, "API Error: Connection closed mid-response.", "", "", "", "agent_error.provider_network", false, "", ""); err != nil {
				t.Fatalf("FailTask: %v", err)
			}
			if got := childCount(t, parentID) == 1; got != tc.wantChild {
				t.Fatalf("retry child created = %v, want %v", got, tc.wantChild)
			}
		})

		t.Run("MaybeRetryFailedTask/"+tc.status, func(t *testing.T) {
			setIssueStatus(t, tc.status)
			parentID := insertParent(t, "failed")
			parent, err := q.GetAgentTask(ctx, parentID)
			if err != nil {
				t.Fatalf("load parent: %v", err)
			}
			child, err := svc.MaybeRetryFailedTask(ctx, parent)
			if err != nil {
				t.Fatalf("MaybeRetryFailedTask: %v", err)
			}
			if got := child != nil; got != tc.wantChild {
				t.Fatalf("retry child created = %v, want %v", got, tc.wantChild)
			}
		})
	}
}
