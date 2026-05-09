package ir

import (
	"fmt"
	"strings"
)

// ParseRefs extracts all {{...}} template references from a string.
// Returns the parsed Ref values. Returns an error if a template
// expression is malformed.
func ParseRefs(s string) ([]*Ref, error) {
	var refs []*Ref
	rest := s
	for {
		start := strings.Index(rest, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(rest[start:], "}}")
		if end == -1 {
			return nil, fmt.Errorf("unterminated template expression at %q", rest[start:])
		}
		end += start // adjust to absolute position
		expr := strings.TrimSpace(rest[start+2 : end])
		raw := rest[start : end+2]
		ref, err := parseRef(expr, raw)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
		rest = rest[end+2:]
	}
	return refs, nil
}

// parseRef parses a single template expression like "outputs.node.field".
//
// A leading `!` flags the reference as "unquoted" (raw substitution) for
// tool command rendering. `{{!input.cmd}}` substitutes input.cmd verbatim
// into the resolved shell command instead of running through shellEscape.
// The bang prefix is stripped before namespace parsing so the rest of the
// expression behaves exactly as without it. The flag has no effect on
// non-shell template contexts (prompts, edge data mappings) — they always
// render values via formatValue.
func parseRef(expr, raw string) (*Ref, error) {
	unquoted := false
	if strings.HasPrefix(expr, "!") {
		unquoted = true
		expr = strings.TrimSpace(expr[1:])
	}
	parts := strings.Split(expr, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid reference %q: expected namespace.path", raw)
	}
	namespace := parts[0]
	path := parts[1:]

	var kind RefKind
	switch namespace {
	case "vars":
		kind = RefVars
	case "input":
		kind = RefInput
	case "outputs":
		kind = RefOutputs
	case "artifacts":
		kind = RefArtifacts
	case "attachments":
		kind = RefAttachments
	case "loop":
		kind = RefLoop
	case "run":
		kind = RefRun
	default:
		return nil, fmt.Errorf("unknown reference namespace %q in %q", namespace, raw)
	}

	return &Ref{
		Kind:     kind,
		Path:     path,
		Raw:      raw,
		Unquoted: unquoted,
	}, nil
}
