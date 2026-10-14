package inbox

// typeToGroup maps each inbox item type to the notification preference group
// that mutes it. Types not listed are always delivered: they ask the owner to
// act (a paused autopilot, an exhausted quota) and muting them would hide a
// problem nobody else will see. quick_create_failed and
// quick_create_unconfirmed belong here too: the row is the only place the
// requester's original prompt survives, and nothing re-sends it.
var typeToGroup = map[string]string{
	"issue_assigned":     "assignments",
	"unassigned":         "assignments",
	"assignee_changed":   "assignments",
	"issue_subscribed":   "assignments",
	"status_changed":     "status_changes",
	"children_done":      "status_changes",
	"new_comment":        "comments",
	"reaction_added":     "comments",
	"mentioned":          "mentions",
	"priority_changed":   "updates",
	"start_date_changed": "updates",
	"due_date_changed":   "updates",
	"task_completed":     "agent_activity",
	"task_failed":        "agent_activity",
	"agent_blocked":      "agent_activity",
	"agent_completed":    "agent_activity",
	"quick_create_done":  "agent_activity",
}

// MuteGroup returns the preference group that mutes notifType, or "" when the
// type cannot be muted.
func MuteGroup(notifType string) string {
	return typeToGroup[notifType]
}
