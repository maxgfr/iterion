package main

import "context"

// ThreadRef identifies a chat thread across the adapter. For Mattermost
// it is (channel id, root post id); a Slack driver would map it to
// (channel, thread_ts). It is what a callback token encodes.
type ThreadRef struct {
	ChannelID string
	RootID    string
}

// InboundPost is a normalised "a message was posted" event, emitted by
// a ChannelDriver's Listen loop. Platform-specific event envelopes are
// flattened to this shape so the Coordinator stays platform-agnostic.
type InboundPost struct {
	Thread         ThreadRef
	UserID         string
	Text           string
	CreateAtMillis int64
	// MentionsBot is true when this post @-mentions the bot — the
	// signal that activates the bot for the thread.
	MentionsBot bool
	// FromBot is true when the post was authored by the bot account
	// itself; the Coordinator ignores these to avoid self-loops.
	FromBot bool
}

// ConsentAction is a normalised consent button click.
type ConsentAction struct {
	Thread  ThreadRef
	UserID  string
	Granted bool
}

// ChannelDriver is the platform-specific surface. A Mattermost and a
// Slack implementation differ only here; the Coordinator and the
// privacy cores (Anonymizer, ConsentStore, callback token) are shared.
//
// All methods must be safe for concurrent use.
type ChannelDriver interface {
	// Listen streams inbound posts until ctx is cancelled. Consent
	// button clicks are delivered out-of-band to the Coordinator's HTTP
	// handler (they arrive as platform webhooks), not through this
	// channel.
	Listen(ctx context.Context) (<-chan InboundPost, error)

	// RequestConsent posts an interactive consent prompt (Accept /
	// Decline) into the thread, addressed to userID.
	RequestConsent(ctx context.Context, thread ThreadRef, userID string) error

	// PostReply posts text as a reply in the thread.
	PostReply(ctx context.Context, thread ThreadRef, text string) error
}
