package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/pkg/agent"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// maxChatRunModelLength bounds a per-message model id. Model ids are
// runtime-native free text (custom ids are allowed, as on the agent), so this
// only rejects obviously malformed input.
const maxChatRunModelLength = 256

// resolveChatRunOverrides validates a per-message model / thinking-level
// selection against the chat agent's runtime and returns it in service form.
// It applies the same rules as saving the values on the agent, with the
// effective model being the override when set and the agent's model otherwise.
// On failure it has already written a 4xx and returns ok=false.
func (h *Handler) resolveChatRunOverrides(w http.ResponseWriter, r *http.Request, ag db.Agent, model, thinkingLevel string) (service.ChatRunOverrides, bool) {
	model = strings.TrimSpace(model)
	thinkingLevel = strings.TrimSpace(thinkingLevel)
	if model == "" && thinkingLevel == "" {
		return service.ChatRunOverrides{}, true
	}
	if len(model) > maxChatRunModelLength {
		writeError(w, http.StatusBadRequest, "model is too long")
		return service.ChatRunOverrides{}, false
	}
	if thinkingLevel != "" {
		provider, ok := h.resolveAgentProvider(r, ag.WorkspaceID, ag.RuntimeID)
		if !ok {
			writeError(w, http.StatusConflict, "chat agent has no runtime")
			return service.ChatRunOverrides{}, false
		}
		if !agent.IsKnownThinkingValue(provider, thinkingLevel) {
			writeError(w, http.StatusBadRequest, thinkingLevelRejection(provider, thinkingLevel))
			return service.ChatRunOverrides{}, false
		}
		switch h.acpThinkingDecision(r.Context(), provider, ag.RuntimeID) {
		case acpEffortAbsent:
			writeError(w, http.StatusBadRequest, thinkingCapabilityRejection(provider))
			return service.ChatRunOverrides{}, false
		case acpEffortUnknown:
			writeError(w, http.StatusBadRequest, thinkingCapabilityUnknownRejection(provider))
			return service.ChatRunOverrides{}, false
		}
		effectiveModel := model
		if effectiveModel == "" {
			effectiveModel = strings.TrimSpace(ag.Model.String)
		}
		if agent.ThinkingLevelRejectedWithoutModel(provider) && effectiveModel == "" {
			writeError(w, http.StatusBadRequest, thinkingNeedsExplicitModelRejection(provider))
			return service.ChatRunOverrides{}, false
		}
	}
	return service.ChatRunOverrides{Model: model, ThinkingLevel: thinkingLevel}, true
}

// attachChatRunOverrides fills each message's model / thinking-level pick from
// its task, so the transcript shows what a turn ran with. resp is index-aligned
// with messages. Best-effort: a failed lookup leaves the fields empty.
func (h *Handler) attachChatRunOverrides(ctx context.Context, messages []db.ChatMessage, resp []ChatMessageResponse) {
	taskIDs := make([]pgtype.UUID, 0, len(messages))
	seen := make(map[string]bool, len(messages))
	for _, m := range messages {
		if !m.TaskID.Valid || seen[uuidToString(m.TaskID)] {
			continue
		}
		seen[uuidToString(m.TaskID)] = true
		taskIDs = append(taskIDs, m.TaskID)
	}
	if len(taskIDs) == 0 {
		return
	}
	rows, err := h.Queries.ListTaskRunOverrides(ctx, taskIDs)
	if err != nil {
		slog.Warn("list chat run overrides failed", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	byTask := make(map[string]db.ListTaskRunOverridesRow, len(rows))
	for _, row := range rows {
		byTask[uuidToString(row.ID)] = row
	}
	for i, m := range messages {
		if row, ok := byTask[uuidToString(m.TaskID)]; ok && m.TaskID.Valid {
			resp[i].ModelOverride = row.ModelOverride.String
			resp[i].ThinkingLevelOverride = row.ThinkingLevelOverride.String
		}
	}
}
