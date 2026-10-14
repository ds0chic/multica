package handler

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/pkg/agent"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// maxAutopilotModelLength bounds a per-autopilot model id. Model ids are
// runtime-native free text (custom ids are allowed, as on the agent), so this
// only rejects obviously malformed input.
const maxAutopilotModelLength = 256

// resolveAutopilotRunDefaults validates a per-autopilot model / thinking-level
// pair against the assignee agent's runtime, with the same rules as saving
// them on the agent (the effective model being the autopilot's when set and
// the agent's otherwise). It returns them as nullable columns. On failure it
// has already written a 4xx and returns ok=false.
func (h *Handler) resolveAutopilotRunDefaults(
	w http.ResponseWriter,
	r *http.Request,
	q *db.Queries,
	assigneeType string,
	assigneeID pgtype.UUID,
	workspaceID pgtype.UUID,
	model, thinkingLevel string,
) (pgtype.Text, pgtype.Text, bool) {
	model = strings.TrimSpace(model)
	thinkingLevel = strings.TrimSpace(thinkingLevel)
	modelText := pgtype.Text{String: model, Valid: model != ""}
	thinkingText := pgtype.Text{String: thinkingLevel, Valid: thinkingLevel != ""}
	if model == "" && thinkingLevel == "" {
		return modelText, thinkingText, true
	}
	if assigneeType != "agent" {
		writeError(w, http.StatusBadRequest, "model and thinking_level can only be set on an autopilot assigned to an agent")
		return pgtype.Text{}, pgtype.Text{}, false
	}
	if len(model) > maxAutopilotModelLength {
		writeError(w, http.StatusBadRequest, "model is too long")
		return pgtype.Text{}, pgtype.Text{}, false
	}
	if thinkingLevel == "" {
		return modelText, thinkingText, true
	}
	ag, err := q.GetAgent(r.Context(), assigneeID)
	if err != nil || uuidToString(ag.WorkspaceID) != uuidToString(workspaceID) {
		writeError(w, http.StatusBadRequest, "assignee agent not found")
		return pgtype.Text{}, pgtype.Text{}, false
	}
	provider, ok := h.resolveAgentProvider(r, ag.WorkspaceID, ag.RuntimeID)
	if !ok {
		writeError(w, http.StatusConflict, "assignee agent has no runtime")
		return pgtype.Text{}, pgtype.Text{}, false
	}
	if !agent.IsKnownThinkingValue(provider, thinkingLevel) {
		writeError(w, http.StatusBadRequest, thinkingLevelRejection(provider, thinkingLevel))
		return pgtype.Text{}, pgtype.Text{}, false
	}
	switch h.acpThinkingDecision(r.Context(), provider, ag.RuntimeID) {
	case acpEffortAbsent:
		writeError(w, http.StatusBadRequest, thinkingCapabilityRejection(provider))
		return pgtype.Text{}, pgtype.Text{}, false
	case acpEffortUnknown:
		writeError(w, http.StatusBadRequest, thinkingCapabilityUnknownRejection(provider))
		return pgtype.Text{}, pgtype.Text{}, false
	}
	effectiveModel := model
	if effectiveModel == "" {
		effectiveModel = strings.TrimSpace(ag.Model.String)
	}
	if agent.ThinkingLevelRejectedWithoutModel(provider) && effectiveModel == "" {
		writeError(w, http.StatusBadRequest, thinkingNeedsExplicitModelRejection(provider))
		return pgtype.Text{}, pgtype.Text{}, false
	}
	return modelText, thinkingText, true
}

// optionalString reads an optional JSON string, treating null like "".
func optionalString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
