package gitlab

import (
	"context"
	"strconv"
	"strings"
)

// ReplierAuth is the input to the conversational authorization decision.
type ReplierAuth struct {
	AuthorID       int64
	AuthorUsername string
	ProjectID      int64
	Allowlist      []string // usernames (with/without @) or numeric ids
	MinRole        string   // role name; "" → developer
}

// AuthorizeReplier decides whether a note's author may trigger the bot:
// **(in the explicit allowlist) OR (a project member at >= MinRole)**. An
// allowlist hit short-circuits the role check (no API call). The role check
// queries the forge with the bot's token.
func AuthorizeReplier(ctx context.Context, api API, in ReplierAuth) (ok bool, reason string, err error) {
	if InAllowlist(in.Allowlist, in.AuthorUsername, in.AuthorID) {
		return true, "allowlist", nil
	}
	level, member, err := api.MemberAccessLevel(ctx, in.ProjectID, in.AuthorID)
	if err != nil {
		return false, "", err
	}
	if member && level >= RoleLevel(in.MinRole) {
		return true, "role", nil
	}
	return false, "", nil
}

// InAllowlist matches an author by username (case-insensitive, optional @)
// or numeric id.
func InAllowlist(allow []string, username string, id int64) bool {
	idStr := strconv.FormatInt(id, 10)
	for _, a := range allow {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if a == idStr || strings.EqualFold(strings.TrimPrefix(a, "@"), username) {
			return true
		}
	}
	return false
}
