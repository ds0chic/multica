package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// createChatRunOverrideRuntime seeds an online cloud runtime for a provider
// with a fixed thinking enum, so per-message effort validation has a real
// provider to check against (the shared handler test runtime uses a
// placeholder provider that accepts no thinking levels).
func createChatRunOverrideRuntime(t *testing.T, provider string) string {
	t.Helper()
	var runtimeID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status, device_info, metadata, owner_id, last_seen_at
		)
		VALUES ($1, NULL, $2, 'cloud', $3, 'online', 'chat run override test', '{}'::jsonb, $4, now())
		RETURNING id
	`, testWorkspaceID, "Chat run override "+provider, provider, testUserID).Scan(&runtimeID); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE runtime_id = $1`, runtimeID)
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id = $1`, runtimeID)
	})
	return runtimeID
}

func createChatRunOverrideAgent(t *testing.T, name, runtimeID string) string {
	t.Helper()
	agentID := dbfx.Agent(t, name, runtimeID, testutil.Cols{
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
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, agentID)
	})
	return agentID
}

func sendChatMessageWithOverrides(t *testing.T, sessionID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	req := newRequest("POST", "/api/chat-sessions/"+sessionID+"/messages", body)
	req = withURLParam(req, "sessionId", sessionID)
	req = withChatTestWorkspaceCtx(t, req)
	w := httptest.NewRecorder()
	testHandler.SendChatMessage(w, req)
	return w
}

func TestSendChatMessage_RunOverridesReachClaim(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	runtimeID := createChatRunOverrideRuntime(t, "claude")
	agentID := createChatRunOverrideAgent(t, "ChatRunOverrideAgent", runtimeID)
	sessionID := createHandlerTestChatSession(t, agentID)

	w := sendChatMessageWithOverrides(t, sessionID, map[string]any{
		"content":        "use the bigger model for this one",
		"model":          "claude-opus-override",
		"thinking_level": "high",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("SendChatMessage: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sent SendChatMessageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &sent); err != nil {
		t.Fatalf("decode send response: %v", err)
	}

	var model, thinking *string
	if err := testPool.QueryRow(context.Background(),
		`SELECT model_override, thinking_level_override FROM agent_task_queue WHERE id = $1`,
		sent.TaskID,
	).Scan(&model, &thinking); err != nil {
		t.Fatalf("load task overrides: %v", err)
	}
	if model == nil || *model != "claude-opus-override" || thinking == nil || *thinking != "high" {
		t.Fatalf("task overrides = (%v, %v), want (claude-opus-override, high)", model, thinking)
	}

	claimW := httptest.NewRecorder()
	claimReq := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/claim", nil, testWorkspaceID, "chat-run-override-test")
	claimReq = withURLParam(claimReq, "runtimeId", runtimeID)
	testHandler.ClaimTaskByRuntime(claimW, claimReq)
	if claimW.Code != http.StatusOK {
		t.Fatalf("ClaimTaskByRuntime: expected 200, got %d: %s", claimW.Code, claimW.Body.String())
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
	if err := json.NewDecoder(claimW.Body).Decode(&claimed); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if claimed.Task == nil || claimed.Task.ID != sent.TaskID || claimed.Task.Agent == nil {
		t.Fatalf("claimed task = %+v, want %s with agent", claimed.Task, sent.TaskID)
	}
	if claimed.Task.Agent.Model != "claude-opus-override" || claimed.Task.Agent.ThinkingLevel != "high" {
		t.Fatalf("claimed agent model/thinking = (%q, %q), want overrides", claimed.Task.Agent.Model, claimed.Task.Agent.ThinkingLevel)
	}

	// The agent's own defaults are untouched by a per-message pick.
	var agentModel, agentThinking string
	if err := testPool.QueryRow(context.Background(),
		`SELECT model, thinking_level FROM agent WHERE id = $1`, agentID,
	).Scan(&agentModel, &agentThinking); err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if agentModel != "agent-default-model" || agentThinking != "low" {
		t.Fatalf("agent defaults changed to (%q, %q)", agentModel, agentThinking)
	}
}

func TestSendChatMessage_WithoutOverridesUsesAgentDefaults(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	runtimeID := createChatRunOverrideRuntime(t, "claude")
	agentID := createChatRunOverrideAgent(t, "ChatRunNoOverrideAgent", runtimeID)
	sessionID := createHandlerTestChatSession(t, agentID)

	w := sendChatMessageWithOverrides(t, sessionID, map[string]any{"content": "plain message"})
	if w.Code != http.StatusCreated {
		t.Fatalf("SendChatMessage: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	claimW := httptest.NewRecorder()
	claimReq := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/claim", nil, testWorkspaceID, "chat-run-override-test")
	claimReq = withURLParam(claimReq, "runtimeId", runtimeID)
	testHandler.ClaimTaskByRuntime(claimW, claimReq)
	if claimW.Code != http.StatusOK {
		t.Fatalf("ClaimTaskByRuntime: expected 200, got %d: %s", claimW.Code, claimW.Body.String())
	}
	var claimed struct {
		Task *struct {
			Agent *struct {
				Model         string `json:"model"`
				ThinkingLevel string `json:"thinking_level"`
			} `json:"agent"`
		} `json:"task"`
	}
	if err := json.NewDecoder(claimW.Body).Decode(&claimed); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if claimed.Task == nil || claimed.Task.Agent == nil {
		t.Fatalf("expected a claimed task with agent, got %+v", claimed.Task)
	}
	if claimed.Task.Agent.Model != "agent-default-model" || claimed.Task.Agent.ThinkingLevel != "low" {
		t.Fatalf("claimed agent model/thinking = (%q, %q), want agent defaults", claimed.Task.Agent.Model, claimed.Task.Agent.ThinkingLevel)
	}
}

func TestSendChatMessage_RejectsUnknownThinkingLevelOverride(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	runtimeID := createChatRunOverrideRuntime(t, "claude")
	agentID := createChatRunOverrideAgent(t, "ChatRunBadThinkingAgent", runtimeID)
	sessionID := createHandlerTestChatSession(t, agentID)

	w := sendChatMessageWithOverrides(t, sessionID, map[string]any{
		"content":        "should not land",
		"thinking_level": "turbo",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("SendChatMessage: expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var count int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM chat_message WHERE chat_session_id = $1`, sessionID,
	).Scan(&count); err != nil {
		t.Fatalf("count chat messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no chat_message rows after rejected send, got %d", count)
	}
}
