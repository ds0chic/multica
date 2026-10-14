package handler

import (
	"net/http"
	"strings"

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
