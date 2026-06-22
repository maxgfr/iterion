package prforge

import "testing"

const ghIssueCommentPR = `{
  "action": "created",
  "repository": {"id": 42, "full_name": "acme/widgets", "clone_url": "https://github.com/acme/widgets.git"},
  "issue": {"number": 7, "title": "Add X", "body": "desc", "state": "open",
    "html_url": "https://github.com/acme/widgets/issues/7",
    "pull_request": {"html_url": "https://github.com/acme/widgets/pull/7"}},
  "comment": {"id": 555, "body": "/featurly add export endpoint",
    "html_url": "https://github.com/acme/widgets/pull/7#issuecomment-555"},
  "sender": {"login": "alice"}
}`

func TestParseIssueComment_PRCommand(t *testing.T) {
	p, err := ParseIssueComment([]byte(ghIssueCommentPR))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsPullRequest || p.Surface() != "pr" {
		t.Errorf("should be a PR comment (surface pr): %+v", p)
	}
	if p.ProjectPath != "acme/widgets" || p.IssueNumber != 7 || p.AuthorLogin != "alice" {
		t.Errorf("fields: %+v", p)
	}
	if p.PRURL != "https://github.com/acme/widgets/pull/7" {
		t.Errorf("pr url: %q", p.PRURL)
	}
	cmd, args := p.Command()
	if cmd != "featurly" || args != "add export endpoint" {
		t.Errorf("command: cmd=%q args=%q", cmd, args)
	}
	if p.SubjectID() != "comment:555" {
		t.Errorf("subject id: %q", p.SubjectID())
	}
}

func TestParseIssueComment_PlainIssueNoCommand(t *testing.T) {
	body := `{"action":"created","repository":{"full_name":"a/b"},"issue":{"number":3,"state":"open"},"comment":{"id":1,"body":"thanks!"},"sender":{"login":"bob"}}`
	p, err := ParseIssueComment([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.IsPullRequest || p.Surface() != "issue" {
		t.Errorf("should be a plain issue (surface issue): %+v", p)
	}
	if cmd, _ := p.Command(); cmd != "" {
		t.Errorf("no command expected, got %q", cmd)
	}
}

func TestParseIssueComment_QuotedReplyTolerated(t *testing.T) {
	body := `{"action":"created","repository":{"full_name":"a/b"},"issue":{"number":3,"state":"open","pull_request":{"html_url":"x"}},"comment":{"id":1,"body":"> quoted context\n/seki"},"sender":{"login":"bob"}}`
	p, _ := ParseIssueComment([]byte(body))
	if cmd, _ := p.Command(); cmd != "seki" {
		t.Errorf("quote-reply should be skipped, want seki, got %q", cmd)
	}
}
