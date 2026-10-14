package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// When a run ends without success and nothing else is working the issue, the
// issue must not stay in_progress: the daemon-reported failure path and a
// user's stop now reset it to todo, like the sweeper already did.
func TestIssueLeavesInProgressWhenRunEnds(t *testing.T) {
	pool := newResolveOriginatorPool(t)
	ctx := context.Background()
	q := db.New(pool)
	_, userID, agentID, issueID := seedAttributionFixture(t, pool)
	svc := &TaskService{Queries: q, TxStarter: pool, Bus: events.New()}

	var runtimeID string
	if err := pool.QueryRow(ctx, `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID); err != nil {
		t.Fatalf("read agent runtime: %v", err)
	}

	setStatus := func(t *testing.T, status string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE issue SET status = $2 WHERE id = $1`, issueID, status); err != nil {
			t.Fatalf("set issue status: %v", err)
		}
	}
	issueStatus := func(t *testing.T) string {
		t.Helper()
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM issue WHERE id = $1`, issueID).Scan(&status); err != nil {
			t.Fatalf("read issue status: %v", err)
		}
		return status
	}
	insertTask := func(t *testing.T, status string) pgtype.UUID {
		t.Helper()
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, `
			INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority, attempt, max_attempts)
			VALUES ($1, $2, $3, $4, 0, 1, 2)
			RETURNING id
		`, agentID, runtimeID, issueID, status).Scan(&id); err != nil {
			t.Fatalf("insert task: %v", err)
		}
		t.Cleanup(func() {
			pool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE parent_task_id = $1 OR id = $1`, id)
		})
		return id
	}
	fail := func(t *testing.T, id pgtype.UUID, reason string) {
		t.Helper()
		if _, err := svc.FailTask(ctx, id, "agent exited with an error", "", "", "", reason, false, "", ""); err != nil {
			t.Fatalf("FailTask: %v", err)
		}
	}

	t.Run("agent error resets to todo", func(t *testing.T) {
		setStatus(t, "in_progress")
		fail(t, insertTask(t, "running"), "agent_error")
		if got := issueStatus(t); got != "todo" {
			t.Fatalf("issue status = %q, want todo", got)
		}
	})

	t.Run("pending retry keeps in_progress", func(t *testing.T) {
		setStatus(t, "in_progress")
		fail(t, insertTask(t, "running"), "agent_error.provider_network")
		if got := issueStatus(t); got != "in_progress" {
			t.Fatalf("issue status = %q, want in_progress", got)
		}
	})

	t.Run("another active task keeps in_progress", func(t *testing.T) {
		setStatus(t, "in_progress")
		insertTask(t, "queued")
		fail(t, insertTask(t, "running"), "agent_error")
		if got := issueStatus(t); got != "in_progress" {
			t.Fatalf("issue status = %q, want in_progress", got)
		}
		pool.Exec(ctx, `UPDATE agent_task_queue SET status = 'cancelled' WHERE issue_id = $1 AND status = 'queued'`, issueID)
	})

	t.Run("in_review set by the agent is left alone", func(t *testing.T) {
		setStatus(t, "in_review")
		fail(t, insertTask(t, "running"), "agent_error")
		if got := issueStatus(t); got != "in_review" {
			t.Fatalf("issue status = %q, want in_review", got)
		}
	})

	t.Run("user stop resets to todo", func(t *testing.T) {
		setStatus(t, "in_progress")
		id := insertTask(t, "running")
		if _, err := svc.CancelTaskByUser(ctx, id, TaskCancellationActor{
			Type: "member",
			ID:   util.MustParseUUID(userID),
			Name: "Attr User",
		}); err != nil {
			t.Fatalf("CancelTaskByUser: %v", err)
		}
		if got := issueStatus(t); got != "todo" {
			t.Fatalf("issue status = %q, want todo", got)
		}
	})
}
