package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// GitLab numeric access levels.
const (
	AccessGuest      = 10
	AccessReporter   = 20
	AccessDeveloper  = 30
	AccessMaintainer = 40
	AccessOwner      = 50
)

// RoleLevel maps a role name to its GitLab access level. Unknown / empty
// defaults to Developer — the sensible "can talk to the bot" floor.
func RoleLevel(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "guest":
		return AccessGuest
	case "reporter":
		return AccessReporter
	case "maintainer":
		return AccessMaintainer
	case "owner":
		return AccessOwner
	case "developer", "":
		return AccessDeveloper
	default:
		return AccessDeveloper
	}
}

// API is a minimal GitLab REST client for the conversational auth checks
// (loop-guard + role-gate). BaseURL is "https://<host>" (no /api/v4); auth
// is a PRIVATE-TOKEN header carrying the bot's forge_token.
type API struct {
	HTTP    *http.Client
	BaseURL string
	Token   string
}

func (a API) get(ctx context.Context, path string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.BaseURL, "/")+"/api/v4"+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("PRIVATE-TOKEN", a.Token)
	c := a.HTTP
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// CurrentUser returns the account the token authenticates as — used to skip
// the bot's own notes (loop-guard).
func (a API) CurrentUser(ctx context.Context) (User, error) {
	var u User
	code, err := a.get(ctx, "/user", &u)
	if err != nil {
		return User{}, err
	}
	if code != http.StatusOK {
		return User{}, fmt.Errorf("gitlab: GET /user: HTTP %d", code)
	}
	return u, nil
}

// MemberAccessLevel returns the user's effective access level on the project
// (inherited memberships included) and whether they are a member at all.
func (a API) MemberAccessLevel(ctx context.Context, projectID, userID int64) (level int, member bool, err error) {
	var m struct {
		AccessLevel int `json:"access_level"`
	}
	code, err := a.get(ctx, "/projects/"+strconv.FormatInt(projectID, 10)+"/members/all/"+strconv.FormatInt(userID, 10), &m)
	if err != nil {
		return 0, false, err
	}
	switch code {
	case http.StatusOK:
		return m.AccessLevel, true, nil
	case http.StatusNotFound:
		return 0, false, nil // not a member
	default:
		return 0, false, fmt.Errorf("gitlab: member check: HTTP %d", code)
	}
}

// DiscussionHasBotNote reports whether the MR discussion already contains a
// note authored by botUserID — i.e. it is a thread Revi started or replied
// in, so a plain reply in it is "talking to Revi". Drives the reply-in-thread
// trigger (a conversational answer without an explicit /revi command).
func (a API) DiscussionHasBotNote(ctx context.Context, projectID, mrIID int64, discussionID string, botUserID int64) (bool, error) {
	if discussionID == "" || botUserID == 0 {
		return false, nil
	}
	var d struct {
		Notes []struct {
			Author struct {
				ID int64 `json:"id"`
			} `json:"author"`
		} `json:"notes"`
	}
	code, err := a.get(ctx, fmt.Sprintf("/projects/%d/merge_requests/%d/discussions/%s", projectID, mrIID, discussionID), &d)
	if err != nil {
		return false, err
	}
	switch code {
	case http.StatusOK:
		for _, n := range d.Notes {
			if n.Author.ID == botUserID {
				return true, nil
			}
		}
		return false, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("gitlab: get discussion: HTTP %d", code)
	}
}
