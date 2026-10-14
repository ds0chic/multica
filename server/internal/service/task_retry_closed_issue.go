package service

import (
	"context"

	"github.com/multica-ai/multica/server/internal/issuestatus"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// retryTargetsClosedIssue reports whether an automatic retry would restart an
// assignment run on an issue that is now done or cancelled, typically because
// the user closed it while the run was failing. A run someone asked for by
// name (a comment or @mention) is left alone: it was requested on the issue
// as it was, closed or not. An unreadable issue is not treated as closed, so
// the retry keeps its old behavior.
func retryTargetsClosedIssue(ctx context.Context, q *db.Queries, t db.AgentTaskQueue) bool {
	if !t.IssueID.Valid || t.TriggerCommentID.Valid {
		return false
	}
	issue, err := q.GetIssue(ctx, t.IssueID)
	if err != nil {
		return false
	}
	switch issuestatus.Effective(ctx, q, issue.WorkspaceID, issue.Status) {
	case "done", "cancelled":
		return true
	}
	return false
}
