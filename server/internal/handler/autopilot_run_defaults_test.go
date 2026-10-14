package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// autopilotRunDefaultsAgent seeds an online claude runtime (fixed thinking
// enum) and a workspace-invocable agent on it with saved model/effort values.
func autopilotRunDefaultsAgent(t *testing.T, name string) (agentID, runtimeID string) {
	t.Helper()
	runtimeID = dbfx.Runtime(t, name+" runtime", testutil.Cols{"provider": "claude"})
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE runtime_id = $1`, runtimeID)
	})
	agentID = dbfx.Agent(t, name, runtimeID, testutil.Cols{
		"visibility":      "workspace",
		"permission_mode": "public_to",
		"instructions":    "",
		"custom_env":      testutil.Raw("'{}'::jsonb"),
		"custom_args":     testutil.Raw("'[]'::jsonb"),
		"mcp_config":      []byte("[]"),
		"model":           "agent-default-model",
		"thinking_level":  "low",
	})
	dbfx.Exec(t, `
		INSERT INTO agent_invocation_target (agent_id, target_type, target_id)
		VALUES ($1, 'workspace', $2)
		ON CONFLICT (agent_id, target_type, target_id) DO NOTHING
	`, agentID, testWorkspaceID)
	return agentID, runtimeID
}

func cleanupAutopilot(t *testing.T, id string) {
	t.Cleanup(func() {
		ctx := context.Background()
		testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE autopilot_run_id IN (SELECT id FROM autopilot_run WHERE autopilot_id = $1)`, id)
		testPool.Exec(ctx, `DELETE FROM autopilot_run WHERE autopilot_id = $1`, id)
		testPool.Exec(ctx, `DELETE FROM autopilot_rule_version WHERE autopilot_id = $1`, id)
		testPool.Exec(ctx, `DELETE FROM autopilot WHERE id = $1`, id)
	})
}

func createAutopilotWithBody(t *testing.T, body map[string]any) *testutil.Response {
	t.Helper()
	resp := testutil.Call(t, testHandler.CreateAutopilot,
		newRequest(http.MethodPost, "/api/autopilots?workspace_id="+testWorkspaceID, body))
	if resp.Code == http.StatusCreated {
		var ap AutopilotResponse
		if err := json.Unmarshal(resp.Body.Bytes(), &ap); err == nil {
			cleanupAutopilot(t, ap.ID)
		}
	}
	return resp
}

func updateAutopilotWithBody(t *testing.T, id string, body map[string]any) *testutil.Response {
	t.Helper()
	req := withURLParam(newRequest(http.MethodPatch, "/api/autopilots/"+id+"?workspace_id="+testWorkspaceID, body), "id", id)
	return testutil.Call(t, testHandler.UpdateAutopilot, req)
}

func strOrNil(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestAutopilotRunDefaults_SaveAndRetarget(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	agentID, _ := autopilotRunDefaultsAgent(t, "AutopilotDefaultsAgent")

	var ap AutopilotResponse
	createAutopilotWithBody(t, map[string]any{
		"title":          "nightly triage",
		"assignee_id":    agentID,
		"execution_mode": "run_only",
		"model":          "claude-haiku-cheap",
		"thinking_level": "high",
	}).Want(http.StatusCreated).JSON(&ap)
	if strOrNil(ap.Model) != "claude-haiku-cheap" || strOrNil(ap.ThinkingLevel) != "high" {
		t.Fatalf("created defaults = (%s, %s)", strOrNil(ap.Model), strOrNil(ap.ThinkingLevel))
	}

	// An edit that does not mention them keeps them.
	updateAutopilotWithBody(t, ap.ID, map[string]any{"title": "nightly triage v2"}).
		Want(http.StatusOK).JSON(&ap)
	if strOrNil(ap.Model) != "claude-haiku-cheap" || strOrNil(ap.ThinkingLevel) != "high" {
		t.Fatalf("defaults after title edit = (%s, %s)", strOrNil(ap.Model), strOrNil(ap.ThinkingLevel))
	}

	// An unknown effort is rejected and nothing changes.
	updateAutopilotWithBody(t, ap.ID, map[string]any{"thinking_level": "turbo"}).Want(http.StatusBadRequest)

	// Clearing one value keeps the other.
	updateAutopilotWithBody(t, ap.ID, map[string]any{"thinking_level": ""}).
		Want(http.StatusOK).JSON(&ap)
	if strOrNil(ap.Model) != "claude-haiku-cheap" || ap.ThinkingLevel != nil {
		t.Fatalf("defaults after clearing effort = (%s, %s)", strOrNil(ap.Model), strOrNil(ap.ThinkingLevel))
	}

	// Retargeting to another agent without restating them clears them: model
	// ids belong to the old agent's runtime.
	otherAgentID, _ := autopilotRunDefaultsAgent(t, "AutopilotDefaultsOther")
	updateAutopilotWithBody(t, ap.ID, map[string]any{"assignee_type": "agent", "assignee_id": otherAgentID}).
		Want(http.StatusOK).JSON(&ap)
	if ap.Model != nil || ap.ThinkingLevel != nil {
		t.Fatalf("defaults after retarget = (%s, %s), want cleared", strOrNil(ap.Model), strOrNil(ap.ThinkingLevel))
	}
}

func TestAutopilotRunDefaults_RejectsUnknownEffortOnCreate(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	agentID, _ := autopilotRunDefaultsAgent(t, "AutopilotDefaultsReject")
	createAutopilotWithBody(t, map[string]any{
		"title":          "bad effort",
		"assignee_id":    agentID,
		"execution_mode": "run_only",
		"thinking_level": "turbo",
	}).Want(http.StatusBadRequest)
}

func claimForTest(t *testing.T, runtimeID string) (taskID, model, thinking string) {
	t.Helper()
	w := httptest.NewRecorder()
	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/claim", nil, testWorkspaceID, "autopilot-defaults-test")
	req = withURLParam(req, "runtimeId", runtimeID)
	testHandler.ClaimTaskByRuntime(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ClaimTaskByRuntime: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var claimed struct {
		Task *struct {
			ID    string `json:"id"`
			Agent *struct {
				Model         string `json:"model"`
				ThinkingLevel string `json:"thinking_level"`
			} `json:"agent"`
		} `json:"task"`
	}
	if err := json.NewDecoder(w.Body).Decode(&claimed); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if claimed.Task == nil || claimed.Task.Agent == nil {
		t.Fatalf("claim returned no task")
	}
	return claimed.Task.ID, claimed.Task.Agent.Model, claimed.Task.Agent.ThinkingLevel
}

func TestAutopilotRunDefaults_ReachTheClaim(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	agentID, runtimeID := autopilotRunDefaultsAgent(t, "AutopilotDefaultsClaim")

	var ap AutopilotResponse
	createAutopilotWithBody(t, map[string]any{
		"title":          "claim defaults",
		"assignee_id":    agentID,
		"execution_mode": "run_only",
		"model":          "claude-haiku-cheap",
		"thinking_level": "high",
	}).Want(http.StatusCreated).JSON(&ap)

	// run_only: the task is tied to the autopilot through its run.
	runID := dbfx.Insert(t, "autopilot_run", testutil.Cols{
		"autopilot_id": ap.ID,
		"source":       "manual",
		"status":       "running",
	})
	runTaskID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": runtimeID, "autopilot_run_id": runID})
	gotID, model, thinking := claimForTest(t, runtimeID)
	if gotID != runTaskID || model != "claude-haiku-cheap" || thinking != "high" {
		t.Fatalf("run_only claim = (%s, %s, %s), want (%s, claude-haiku-cheap, high)", gotID, model, thinking, runTaskID)
	}
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'completed' WHERE id = $1`, runTaskID)

	// create_issue: a later run of the assignee on the autopilot's issue
	// still uses the autopilot's pick.
	issueID := dbfx.Issue(t, "Autopilot issue", testutil.Cols{
		"assignee_type": "agent",
		"assignee_id":   agentID,
		"origin_type":   "autopilot",
		"origin_id":     ap.ID,
	})
	issueTaskID := dbfx.Task(t, agentID, testutil.Cols{"runtime_id": runtimeID, "issue_id": issueID})
	gotID, model, thinking = claimForTest(t, runtimeID)
	if gotID != issueTaskID || model != "claude-haiku-cheap" || thinking != "high" {
		t.Fatalf("issue claim = (%s, %s, %s), want (%s, claude-haiku-cheap, high)", gotID, model, thinking, issueTaskID)
	}
	dbfx.Exec(t, `UPDATE agent_task_queue SET status = 'completed' WHERE id = $1`, issueTaskID)

	// Another agent on that issue keeps its own settings.
	otherAgentID, otherRuntimeID := autopilotRunDefaultsAgent(t, "AutopilotDefaultsBystander")
	otherTaskID := dbfx.Task(t, otherAgentID, testutil.Cols{"runtime_id": otherRuntimeID, "issue_id": issueID})
	gotID, model, thinking = claimForTest(t, otherRuntimeID)
	if gotID != otherTaskID || model != "agent-default-model" || thinking != "low" {
		t.Fatalf("bystander claim = (%s, %s, %s), want agent defaults", gotID, model, thinking)
	}
}
