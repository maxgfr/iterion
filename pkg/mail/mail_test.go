package mail

import (
	"strings"
	"testing"
)

func TestBuildMIMEHeaderInjection(t *testing.T) {
	m := Message{
		To:       "victim@example.org",
		Subject:  "hello\r\nBcc: attacker@evil.example",
		TextBody: "body",
		HTMLBody: "<p>body</p>",
	}
	raw := string(buildMIME("iterion <no-reply@example.org>", m))
	// The literal "Bcc:" may survive INSIDE the subject value (it's
	// inert there) — the property is that no header LINE starts with
	// it, i.e. the CRLF that would have split the header is gone.
	for _, line := range strings.Split(raw, "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("CRLF injection produced a Bcc header line:\n%s", raw)
		}
	}
	if strings.Contains(m.Subject, "\r\n") && !strings.Contains(raw, "Subject: hello  Bcc:") {
		t.Fatalf("subject sanitisation changed unexpectedly:\n%s", raw)
	}
	for _, want := range []string{"From: iterion <no-reply@example.org>", "To: victim@example.org", "multipart/alternative", "text/plain", "text/html"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("MIME missing %q:\n%s", want, raw)
		}
	}
}

func TestRenderTemplatesEscape(t *testing.T) {
	reset := RenderPasswordReset("u@example.org", ResetData{ResetURL: "https://x/auth/reset?token=iar_abc", ExpiresMinutes: 60})
	if !strings.Contains(reset.TextBody, "iar_abc") || !strings.Contains(reset.HTMLBody, "iar_abc") {
		t.Fatal("reset token link missing from bodies")
	}
	inv := RenderInvitation("u@example.org", InviteData{
		TeamName:  `<script>alert(1)</script>`,
		Role:      "admin",
		AcceptURL: "https://x/invitations/accept?token=t",
		InvitedBy: "boss@example.org",
	})
	if strings.Contains(inv.HTMLBody, "<script>") {
		t.Fatalf("HTML injection in invitation body:\n%s", inv.HTMLBody)
	}
	if !strings.Contains(inv.TextBody, "admin") {
		t.Fatal("role missing from text body")
	}
}

func TestNewSMTPValidation(t *testing.T) {
	if _, err := NewSMTP(Config{}); err == nil {
		t.Fatal("empty config accepted")
	}
	if _, err := NewSMTP(Config{Host: "smtp.example.org", From: "not-an-address"}); err == nil {
		t.Fatal("invalid from accepted")
	}
	m, err := NewSMTP(Config{Host: "smtp.example.org", From: "iterion <no-reply@example.org>"})
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if !m.Enabled() {
		t.Fatal("SMTP mailer must report enabled")
	}
	if m.cfg.Port != 587 || m.cfg.Timeout == 0 {
		t.Fatalf("defaults not applied: %+v", m.cfg)
	}
}

func TestLogMailerDisabled(t *testing.T) {
	l := &LogMailer{}
	if l.Enabled() {
		t.Fatal("log mailer must report disabled (drives email_enabled=false)")
	}
	if err := l.Send(nil, Message{To: "x@example.org"}); err != nil { //nolint:staticcheck // nil ctx fine for the no-op
		t.Fatalf("log send: %v", err)
	}
}
