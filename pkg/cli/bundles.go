package cli

import (
	"github.com/SocialGouv/iterion/pkg/bundle"
)

// openBundleOrFile applies the standard bundle dispatch — detect →
// open archive or directory — that the CLI's run / resume / sandbox-
// doctor paths all share. When path is neither a `.botz` archive nor
// a bundle directory it returns (nil, path, no-op cleanup, nil) so
// callers can fall through to a plain `.bot` compile without
// branching on Detect's Kind themselves.
//
// Errors are returned bare (Detect / Open / OpenDir all surface
// their own context); the caller wraps with its own subcommand
// prefix so resume / run / doctor each keep their bespoke messages.
//
// The returned cleanup MUST be deferred by the caller. It is a no-op
// for bundle directories and for the non-bundle pass-through; only
// the `.botz` archive path carries a real temp-dir cleanup today,
// but callers should still defer unconditionally so future per-run
// extraction modes stay safe. On error the cleanup is the no-op (the
// underlying bundle openers release their own resources before
// returning an error), so callers can `defer cleanup()` before
// checking err without leaking.
//
// The returned Kind lets callers differentiate the archive vs.
// directory branches when their error wrapping must mention the
// distinction (e.g. resume's "original archive may have moved" hint).
func openBundleOrFile(path string) (b *bundle.Bundle, iterPath string, kind bundle.Kind, cleanup func() error, err error) {
	cleanup = func() error { return nil }
	kind, detectErr := bundle.Detect(path)
	if detectErr != nil {
		return nil, path, kind, cleanup, detectErr
	}
	switch kind {
	case bundle.KindBundle:
		opened, c, openErr := bundle.Open(path, "")
		if openErr != nil {
			return nil, path, kind, cleanup, openErr
		}
		return opened, opened.IterPath, kind, c, nil
	case bundle.KindBundleDir:
		opened, openErr := bundle.OpenDir(path)
		if openErr != nil {
			return nil, path, kind, cleanup, openErr
		}
		return opened, opened.IterPath, kind, cleanup, nil
	}
	return nil, path, kind, cleanup, nil
}
