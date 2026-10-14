package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// rerunOverrideFixture seeds a claude agent (fixed thinking enum) assigned to
// an issue, plus one failed run on it to retry.
func rerunOverrideFixture(t *testing.T) (issueID, agentID, failedTaskID string) {
	t.Helper()
	runtimeID := createChatRunOverrideRuntime(t, "claude")
	agentID = createChatRunOverrideAgent(t, "RerunOverrideAgent", runtimeID)
	issueID = dbfx.Issue(t, "Rerun override issue", testutil.Cols{
		"assignee_type": "agent",
		"assignee_id":   agentID,
	})
	failedTaskID = dbfx.Task(t, agentID, testutil.Cols{
		"runtime_id": runtimeID,
		"issue_id":   issueID,
		"status":     "failed",
	})
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id = $1`, issueID)
	})
	return issueID, agentID, failedTaskID
}

func rerunWithBody(t *testing.T, issueID string, body map[string]any) *testutil.Response {
	t.Helper()
	req := withURLParam(newRequest(http.MethodPost, "/api/issues/"+issueID+"/rerun", body), "id", issueID)
	return testutil.Call(t, testHandler.RerunIssue, req)
}

func taskRunOverrides(t *testing.T, taskID string) (model, thinking *string) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(),
		`SELECT model_override, thinking_level_override FROM agent_task_queue WHERE id = $1`, taskID,
	).Scan(&model, &thinking); err != nil {
		t.Fatalf("load task overrides: %v", err)
	}
	return model, thinking
}

func TestRerunIssue_RetryWithModelAndEffort(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, agentID, failedTaskID := rerunOverrideFixture(t)

	var reran struct {
		ID      string `json:"id"`
		AgentID string `json:"agent_id"`
	}
	rerunWithBody(t, issueID, map[string]any{
		"task_id":        failedTaskID,
		"model":          "claude-opus-retry",
		"thinking_level": "high",
	}).Want(http.StatusAccepted).JSON(&reran)
	if reran.AgentID != agentID {
		t.Fatalf("rerun agent = %q, want %q", reran.AgentID, agentID)
	}
	model, thinking := taskRunOverrides(t, reran.ID)
	if model == nil || *model != "claude-opus-retry" || thinking == nil || *thinking != "high" {
		t.Fatalf("rerun overrides = (%v, %v), want (claude-opus-retry, high)", model, thinking)
	}

	// The agent's saved settings are untouched.
	var agentModel, agentThinking string
	if err := testPool.QueryRow(context.Background(),
		`SELECT model, thinking_level FROM agent WHERE id = $1`, agentID,
	).Scan(&agentModel, &agentThinking); err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if agentModel != "agent-default-model" || agentThinking != "low" {
		t.Fatalf("agent settings changed to (%q, %q)", agentModel, agentThinking)
	}

	// A plain retry of that run repeats its pick instead of falling back to
	// the agent default.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE agent_task_queue SET status = 'failed' WHERE id = $1`, reran.ID); err != nil {
		t.Fatalf("fail rerun: %v", err)
	}
	var again struct {
		ID string `json:"id"`
	}
	rerunWithBody(t, issueID, map[string]any{"task_id": reran.ID}).Want(http.StatusAccepted).JSON(&again)
	model, thinking = taskRunOverrides(t, again.ID)
	if model == nil || *model != "claude-opus-retry" || thinking == nil || *thinking != "high" {
		t.Fatalf("plain retry overrides = (%v, %v), want inherited (claude-opus-retry, high)", model, thinking)
	}
}

func TestRerunIssue_WithoutPickRunsAgentDefaults(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, _, failedTaskID := rerunOverrideFixture(t)

	var reran struct {
		ID string `json:"id"`
	}
	rerunWithBody(t, issueID, map[string]any{"task_id": failedTaskID}).Want(http.StatusAccepted).JSON(&reran)
	if model, thinking := taskRunOverrides(t, reran.ID); model != nil || thinking != nil {
		t.Fatalf("rerun overrides = (%v, %v), want none", model, thinking)
	}
}

func TestRerunIssue_RejectsUnknownEffortWithoutEnqueueing(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, _, failedTaskID := rerunOverrideFixture(t)

	rerunWithBody(t, issueID, map[string]any{
		"task_id":        failedTaskID,
		"thinking_level": "turbo",
	}).Want(http.StatusBadRequest)

	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_task_queue WHERE issue_id = $1`, issueID).Scan(&n); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if n != 1 {
		t.Fatalf("task count = %d, want 1 (nothing enqueued)", n)
	}
}
