package gitlab

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Parsed is the normalized merge-request view the handler consumes.
type Parsed struct {
	ProjectID     int64
	ProjectPath   string // group/sub/repo
	ProjectWebURL string
	CloneURL      string
	MRIID         int64
	Action        string // open|reopen|update|...
	SourceBranch  string
	TargetBranch  string
	Title         string
	Description   string
	MRURL         string
	HeadSHA       string
	OldRev        string
	Labels        []string
}

// ParseMergeRequest decodes a GitLab merge_request webhook body.
func ParseMergeRequest(body []byte) (Parsed, error) {
	var e MergeRequestEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return Parsed{}, fmt.Errorf("gitlab: decode mr event: %w", err)
	}
	if e.ObjectKind != "merge_request" {
		return Parsed{}, fmt.Errorf("gitlab: not a merge_request event (object_kind=%q)", e.ObjectKind)
	}
	oa := e.ObjectAttributes
	labels := make([]string, 0, len(e.Labels))
	for _, l := range e.Labels {
		if l.Title != "" {
			labels = append(labels, l.Title)
		}
	}
	return Parsed{
		ProjectID:     e.Project.ID,
		ProjectPath:   e.Project.PathWithNamespace,
		ProjectWebURL: e.Project.WebURL,
		CloneURL:      e.Project.GitHTTPURL,
		MRIID:         oa.IID,
		Action:        oa.Action,
		SourceBranch:  oa.SourceBranch,
		TargetBranch:  oa.TargetBranch,
		Title:         oa.Title,
		Description:   oa.Description,
		MRURL:         oa.URL,
		HeadSHA:       oa.LastCommit.ID,
		OldRev:        oa.OldRev,
		Labels:        labels,
	}, nil
}

// IsReviewable reports whether the MR action should trigger a review.
// open/reopen always do; update only when it carries a new head (a push)
// — GitLab fires "update" on label/description edits too, which we skip.
func (p Parsed) IsReviewable() bool {
	switch p.Action {
	case "open", "reopen":
		return true
	case "update":
		return p.OldRev != "" && p.OldRev != p.HeadSHA
	default:
		return false
	}
}

// SubjectID is the stable per-MR identifier used in delivery records.
func (p Parsed) SubjectID() string {
	return "mr:" + strconv.FormatInt(p.MRIID, 10)
}
