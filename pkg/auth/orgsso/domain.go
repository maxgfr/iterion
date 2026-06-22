package orgsso

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// VerifiedDomain records a tenant's claim over an email domain, proven by a DNS
// TXT challenge. It gates per-org SSO auto-link: an org's Keycloak may only
// auto-link a fresh OIDC identity onto an existing iterion user by email when
// the email's domain is verified for that org — so a malicious org IdP can't
// claim email_verified for an address outside the org's domain authority and
// take over an unrelated account (the H-16 account-takeover gate; JWKS alone is
// insufficient because the org controls its own IdP signing keys).
type VerifiedDomain struct {
	ID         string     `bson:"_id" json:"id"`
	TenantID   string     `bson:"tenant_id" json:"tenant_id"`
	Domain     string     `bson:"domain" json:"domain"` // lowercased, no leading "@"
	Token      string     `bson:"token" json:"token"`   // the TXT challenge value
	VerifiedAt *time.Time `bson:"verified_at,omitempty" json:"verified_at,omitempty"`
	CreatedBy  string     `bson:"created_by,omitempty" json:"created_by,omitempty"`
	CreatedAt  time.Time  `bson:"created_at" json:"created_at"`
}

// Verified reports whether the domain claim has been DNS-verified.
func (d VerifiedDomain) Verified() bool { return d.VerifiedAt != nil }

// ChallengeHost is the DNS name the admin must create a TXT record at.
func (d VerifiedDomain) ChallengeHost() string { return domainChallengePrefix + d.Domain }

// ChallengeValue is the exact TXT record value to publish.
func (d VerifiedDomain) ChallengeValue() string { return domainChallengePrefix2 + d.Token }

const (
	domainChallengePrefix  = "_iterion-challenge."
	domainChallengePrefix2 = "iterion-site-verification="
)

var (
	ErrDomainNotFound = errors.New("orgsso: domain not found")
	ErrDomainExists   = errors.New("orgsso: domain already claimed for this org")
	ErrDomainInvalid  = errors.New("orgsso: invalid domain")
)

// NormalizeDomain lowercases + trims a domain, stripping a leading "@" or
// "*." and any scheme/path an admin might paste.
func NormalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "@")
	s = strings.TrimPrefix(s, "*.")
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/:"); i >= 0 {
		s = s[:i]
	}
	return s
}

// EmailDomain extracts the lowercased domain from an email address.
func EmailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

// NewDomainToken returns a random challenge token (~32 bytes base64url).
func NewDomainToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// TXTLookupFunc resolves the TXT records for a name. Injectable for tests; the
// production default wraps net.Resolver.LookupTXT.
type TXTLookupFunc func(ctx context.Context, name string) ([]string, error)

// VerifyDomainTXT reports whether the domain's challenge TXT record is present.
func VerifyDomainTXT(ctx context.Context, lookup TXTLookupFunc, d VerifiedDomain) (bool, error) {
	records, err := lookup(ctx, d.ChallengeHost())
	if err != nil {
		return false, err
	}
	want := d.ChallengeValue()
	for _, r := range records {
		if strings.TrimSpace(r) == want {
			return true, nil
		}
	}
	return false, nil
}

// DomainStore persists per-tenant verified-domain claims. Get is keyed by id
// only — the HTTP layer asserts tenant ownership before mutating.
type DomainStore interface {
	Create(ctx context.Context, d VerifiedDomain) error
	Get(ctx context.Context, id string) (VerifiedDomain, error)
	Update(ctx context.Context, d VerifiedDomain) error
	Delete(ctx context.Context, id string) error
	ListByTenant(ctx context.Context, tenantID string) ([]VerifiedDomain, error)
	// IsVerifiedForTenant reports whether domain is a VERIFIED claim of tenantID
	// — the auto-link gate's lookup.
	IsVerifiedForTenant(ctx context.Context, tenantID, domain string) (bool, error)
}

// ---- in-memory store ----

type MemoryDomainStore struct {
	mu   sync.RWMutex
	rows map[string]VerifiedDomain
}

func NewMemoryDomainStore() *MemoryDomainStore {
	return &MemoryDomainStore{rows: make(map[string]VerifiedDomain)}
}

func (m *MemoryDomainStore) Create(_ context.Context, d VerifiedDomain) error {
	d.Domain = NormalizeDomain(d.Domain)
	if d.Domain == "" {
		return ErrDomainInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ex := range m.rows {
		if ex.TenantID == d.TenantID && ex.Domain == d.Domain {
			return ErrDomainExists
		}
	}
	m.rows[d.ID] = d
	return nil
}

func (m *MemoryDomainStore) Get(_ context.Context, id string) (VerifiedDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.rows[id]
	if !ok {
		return VerifiedDomain{}, ErrDomainNotFound
	}
	return d, nil
}

func (m *MemoryDomainStore) Update(_ context.Context, d VerifiedDomain) error {
	d.Domain = NormalizeDomain(d.Domain)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[d.ID]; !ok {
		return ErrDomainNotFound
	}
	m.rows[d.ID] = d
	return nil
}

func (m *MemoryDomainStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[id]; !ok {
		return ErrDomainNotFound
	}
	delete(m.rows, id)
	return nil
}

func (m *MemoryDomainStore) ListByTenant(_ context.Context, tenantID string) ([]VerifiedDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []VerifiedDomain
	for _, d := range m.rows {
		if d.TenantID == tenantID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemoryDomainStore) IsVerifiedForTenant(_ context.Context, tenantID, domain string) (bool, error) {
	domain = NormalizeDomain(domain)
	if domain == "" {
		return false, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, d := range m.rows {
		if d.TenantID == tenantID && d.Domain == domain && d.Verified() {
			return true, nil
		}
	}
	return false, nil
}

// defaultTXTLookup is the production TXT resolver.
func defaultTXTLookup(ctx context.Context, name string) ([]string, error) {
	r := &net.Resolver{}
	return r.LookupTXT(ctx, name)
}

// DefaultTXTLookup returns the production net-backed TXT lookup.
func DefaultTXTLookup() TXTLookupFunc { return defaultTXTLookup }
