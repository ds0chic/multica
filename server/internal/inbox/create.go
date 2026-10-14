package inbox

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// CreateIfEligible returns inserted=false without an error when the recipient
// does not belong to the workspace, or is a member who muted the type's
// preference group (MuteGroup). Other database errors remain failures.
// Eligibility is checked by the insert's snapshot, without additional locks.
func CreateIfEligible(ctx context.Context, queries *db.Queries, params db.CreateInboxItemParams) (item db.InboxItem, inserted bool, err error) {
	if group := MuteGroup(params.Type); group != "" {
		params.MuteGroup = pgtype.Text{String: group, Valid: true}
	}
	item, err = queries.CreateInboxItem(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.InboxItem{}, false, nil
	}
	if err != nil {
		return db.InboxItem{}, false, err
	}
	return item, true, nil
}
