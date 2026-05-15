package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SocialGouv/iterion/pkg/conductor/native"
	"github.com/SocialGouv/iterion/pkg/store"
)

// IssueCommonOptions are flags shared by every `iterion issue` subcommand.
type IssueCommonOptions struct {
	StoreDir string
}

// openNativeStore resolves <store-dir>/conductor and opens the native
// store there. The directory and board.json are created on first call.
func openNativeStore(opts IssueCommonOptions) (*native.Store, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	storeDir := store.ResolveStoreDir(cwd, opts.StoreDir)
	root := filepath.Join(storeDir, "conductor")
	s, err := native.NewStore(root)
	if err != nil {
		return nil, "", err
	}
	return s, root, nil
}

// IssueCreateOptions captures the flags for `iterion issue create`.
type IssueCreateOptions struct {
	IssueCommonOptions
	Title    string
	Body     string
	State    string
	Labels   []string
	Priority int
	Assignee string
	Blockers []string
	Fields   []string // key=value pairs
}

// RunIssueCreate creates a new issue in the native tracker.
func RunIssueCreate(p *Printer, opts IssueCreateOptions) error {
	if opts.Title == "" {
		return errors.New("issue create: --title is required")
	}
	s, _, err := openNativeStore(opts.IssueCommonOptions)
	if err != nil {
		return err
	}
	fields, err := parseFieldPairs(opts.Fields)
	if err != nil {
		return err
	}
	iss := native.Issue{
		Title:    opts.Title,
		Body:     opts.Body,
		State:    opts.State,
		Labels:   opts.Labels,
		Priority: opts.Priority,
		Assignee: opts.Assignee,
		Blockers: opts.Blockers,
		Fields:   fields,
	}
	got, err := s.Create(iss)
	if err != nil {
		return err
	}
	if p.Format == OutputJSON {
		p.JSON(got)
		return nil
	}
	p.Line("Created %s", shortID(got.ID))
	p.KV("State", got.State)
	if got.Title != "" {
		p.KV("Title", got.Title)
	}
	return nil
}

// IssueListOptions captures the flags for `iterion issue list`.
type IssueListOptions struct {
	IssueCommonOptions
	States    []string
	Labels    []string
	Assignee  string
	OnlyClaim bool
	OnlyFree  bool
}

// RunIssueList prints issues from the native tracker.
func RunIssueList(p *Printer, opts IssueListOptions) error {
	s, _, err := openNativeStore(opts.IssueCommonOptions)
	if err != nil {
		return err
	}
	f := native.ListFilter{
		States:   opts.States,
		Labels:   opts.Labels,
		Assignee: opts.Assignee,
	}
	switch {
	case opts.OnlyClaim:
		t := true
		f.Claimed = &t
	case opts.OnlyFree:
		t := false
		f.Claimed = &t
	}
	issues, err := s.List(f)
	if err != nil {
		return err
	}
	if p.Format == OutputJSON {
		p.JSON(issues)
		return nil
	}
	rows := make([][]string, 0, len(issues))
	for _, iss := range issues {
		rows = append(rows, []string{
			shortID(iss.ID),
			iss.State,
			strconv.Itoa(iss.Priority),
			truncate(iss.Title, 50),
			strings.Join(iss.Labels, ","),
			iss.Assignee,
		})
	}
	p.Table([]string{"ID", "STATE", "PRIO", "TITLE", "LABELS", "ASSIGNEE"}, rows)
	return nil
}

// IssueRefOptions identifies a single issue by ID or prefix.
type IssueRefOptions struct {
	IssueCommonOptions
	IDOrPrefix string
}

// RunIssueShow prints a single issue.
func RunIssueShow(p *Printer, opts IssueRefOptions) error {
	s, _, err := openNativeStore(opts.IssueCommonOptions)
	if err != nil {
		return err
	}
	id, err := s.Resolve(opts.IDOrPrefix)
	if err != nil {
		return err
	}
	iss, err := s.Get(id)
	if err != nil {
		return err
	}
	if p.Format == OutputJSON {
		p.JSON(iss)
		return nil
	}
	p.Header(iss.Title)
	p.KV("ID", iss.ID)
	p.KV("State", iss.State)
	p.KV("Priority", strconv.Itoa(iss.Priority))
	if iss.Assignee != "" {
		p.KV("Assignee", iss.Assignee)
	}
	if len(iss.Labels) > 0 {
		p.KV("Labels", strings.Join(iss.Labels, ", "))
	}
	if len(iss.Blockers) > 0 {
		p.KV("Blockers", strings.Join(iss.Blockers, ", "))
	}
	if iss.Claim != "" {
		p.KV("Claim", iss.Claim)
	}
	if len(iss.Fields) > 0 {
		data, _ := json.MarshalIndent(iss.Fields, "  ", "  ")
		p.Line("  Fields:")
		p.Line("    %s", string(data))
	}
	if iss.Body != "" {
		p.Blank()
		p.Line("%s", iss.Body)
	}
	return nil
}

// IssueMoveOptions moves an issue to a new state.
type IssueMoveOptions struct {
	IssueCommonOptions
	IDOrPrefix string
	To         string
}

// RunIssueMove transitions an issue.
func RunIssueMove(p *Printer, opts IssueMoveOptions) error {
	if opts.To == "" {
		return errors.New("issue move: --to is required")
	}
	s, _, err := openNativeStore(opts.IssueCommonOptions)
	if err != nil {
		return err
	}
	id, err := s.Resolve(opts.IDOrPrefix)
	if err != nil {
		return err
	}
	iss, err := s.SetState(id, opts.To)
	if err != nil {
		return err
	}
	if p.Format == OutputJSON {
		p.JSON(iss)
		return nil
	}
	p.Line("Moved %s → %s", shortID(iss.ID), iss.State)
	return nil
}

// IssueUpdateOptions captures partial-update fields. Nil pointer means "unchanged".
type IssueUpdateOptions struct {
	IssueCommonOptions
	IDOrPrefix string
	Title      *string
	Body       *string
	Labels     *[]string
	Priority   *int
	Assignee   *string
	Blockers   *[]string
	Fields     []string // key=value (set or replace)
	ClearField []string
}

// RunIssueUpdate applies the patch.
func RunIssueUpdate(p *Printer, opts IssueUpdateOptions) error {
	s, _, err := openNativeStore(opts.IssueCommonOptions)
	if err != nil {
		return err
	}
	id, err := s.Resolve(opts.IDOrPrefix)
	if err != nil {
		return err
	}
	fields, err := parseFieldPairs(opts.Fields)
	if err != nil {
		return err
	}
	for _, k := range opts.ClearField {
		if fields == nil {
			fields = map[string]any{}
		}
		fields[k] = nil
	}
	patch := native.Patch{
		Title:    opts.Title,
		Body:     opts.Body,
		Labels:   opts.Labels,
		Priority: opts.Priority,
		Assignee: opts.Assignee,
		Blockers: opts.Blockers,
		Fields:   fields,
	}
	iss, err := s.Update(id, patch)
	if err != nil {
		return err
	}
	if p.Format == OutputJSON {
		p.JSON(iss)
		return nil
	}
	p.Line("Updated %s", shortID(iss.ID))
	return nil
}

// RunIssueClose moves the issue to the first terminal state on the board.
func RunIssueClose(p *Printer, opts IssueRefOptions) error {
	s, _, err := openNativeStore(opts.IssueCommonOptions)
	if err != nil {
		return err
	}
	id, err := s.Resolve(opts.IDOrPrefix)
	if err != nil {
		return err
	}
	board := s.Board()
	terminal := ""
	for _, st := range board.States {
		if st.Terminal {
			terminal = st.Name
			break
		}
	}
	if terminal == "" {
		return errors.New("issue close: board has no terminal state — declare one or use `issue move`")
	}
	iss, err := s.SetState(id, terminal)
	if err != nil {
		return err
	}
	if p.Format == OutputJSON {
		p.JSON(iss)
		return nil
	}
	p.Line("Closed %s → %s", shortID(iss.ID), iss.State)
	return nil
}

// RunIssueBoardShow prints the current board.json.
func RunIssueBoardShow(p *Printer, opts IssueCommonOptions) error {
	s, root, err := openNativeStore(opts)
	if err != nil {
		return err
	}
	b := s.Board()
	if p.Format == OutputJSON {
		p.JSON(b)
		return nil
	}
	p.Header("Board")
	p.KV("Location", filepath.Join(root, "board.json"))
	p.Blank()
	p.Line("States:")
	for _, st := range b.States {
		tag := ""
		if st.Eligible {
			tag += " (eligible)"
		}
		if st.Terminal {
			tag += " (terminal)"
		}
		p.Line("  - %s%s", st.Name, tag)
	}
	if len(b.Fields) > 0 {
		p.Blank()
		p.Line("Fields:")
		for _, f := range b.Fields {
			req := ""
			if f.Required {
				req = " (required)"
			}
			p.Line("  - %s : %s%s", f.Name, f.Type, req)
			if f.Type == native.FieldEnum {
				p.Line("      values: %s", strings.Join(f.EnumValues, ", "))
			}
		}
	}
	return nil
}

// IssueBoardInitOptions configures `issue board init`.
type IssueBoardInitOptions struct {
	IssueCommonOptions
	From  string // optional path to a board.json
	Force bool
}

// RunIssueBoardInit replaces the board configuration.
func RunIssueBoardInit(p *Printer, opts IssueBoardInitOptions) error {
	s, root, err := openNativeStore(opts.IssueCommonOptions)
	if err != nil {
		return err
	}
	var b *native.Board
	if opts.From != "" {
		data, err := os.ReadFile(opts.From)
		if err != nil {
			return fmt.Errorf("read %s: %w", opts.From, err)
		}
		b = &native.Board{}
		if err := json.Unmarshal(data, b); err != nil {
			return fmt.Errorf("parse %s: %w", opts.From, err)
		}
	} else {
		b = native.DefaultBoard()
	}
	if err := s.SetBoard(b); err != nil {
		return err
	}
	if p.Format == OutputJSON {
		p.JSON(b)
		return nil
	}
	p.Line("Board initialized at %s", filepath.Join(root, "board.json"))
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// parseFieldPairs converts ["k=v", "k2=42"] into typed values. Numbers,
// bools, and bare strings are auto-detected. Quoted values are kept
// literal.
func parseFieldPairs(pairs []string) (map[string]any, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := map[string]any{}
	for _, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("--field expects key=value, got %q", p)
		}
		k := strings.TrimSpace(p[:eq])
		v := strings.TrimSpace(p[eq+1:])
		out[k] = inferTyped(v)
	}
	return out, nil
}

func inferTyped(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

func shortID(id string) string {
	s := strings.TrimPrefix(id, "native:")
	if len(s) > 8 {
		s = s[:8]
	}
	return s
}
