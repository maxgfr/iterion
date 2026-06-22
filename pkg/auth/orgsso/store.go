package orgsso

import (
	"context"
	"sort"
	"sync"
)

// Store persists per-tenant SSO provider rows. Get is keyed by id only — the
// HTTP layer asserts tenant ownership (row.TenantID == teamID) before mutating,
// matching the forge OAuthAppStore convention.
type Store interface {
	Create(ctx context.Context, p OrgSSOProvider) error
	Get(ctx context.Context, id string) (OrgSSOProvider, error)
	Update(ctx context.Context, p OrgSSOProvider) error
	Delete(ctx context.Context, id string) error

	// ListByTenant returns every provider for a tenant, oldest first.
	ListByTenant(ctx context.Context, tenantID string) ([]OrgSSOProvider, error)
	// ListByTenantKind narrows ListByTenant to one Kind (e.g. enabled OIDC
	// rows for the org-scoped login picker).
	ListByTenantKind(ctx context.Context, tenantID string, kind Kind) ([]OrgSSOProvider, error)

	// FindGitHubGrantingOrgs is the load-bearing reverse lookup for GitHub
	// team-gating: every ENABLED KindGitHub row whose GitHubTeamKeys intersect
	// keys. Backed by a multikey $in index — never a cross-tenant scan.
	FindGitHubGrantingOrgs(ctx context.Context, keys []string) ([]OrgSSOProvider, error)
}

// ---- in-memory store (tests / local) ----

// MemoryStore is an in-process Store for tests and local mode.
type MemoryStore struct {
	mu   sync.RWMutex
	rows map[string]OrgSSOProvider
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{rows: make(map[string]OrgSSOProvider)}
}

func (m *MemoryStore) Create(_ context.Context, p OrgSSOProvider) error {
	p.Normalize()
	m.mu.Lock()
	defer m.mu.Unlock()
	if p.Kind == KindGitHub {
		for _, ex := range m.rows {
			if ex.TenantID == p.TenantID && ex.Kind == KindGitHub {
				return ErrExists
			}
		}
	}
	m.rows[p.ID] = p
	return nil
}

func (m *MemoryStore) Get(_ context.Context, id string) (OrgSSOProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.rows[id]
	if !ok {
		return OrgSSOProvider{}, ErrNotFound
	}
	return p, nil
}

func (m *MemoryStore) Update(_ context.Context, p OrgSSOProvider) error {
	p.Normalize()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[p.ID]; !ok {
		return ErrNotFound
	}
	m.rows[p.ID] = p
	return nil
}

func (m *MemoryStore) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[id]; !ok {
		return ErrNotFound
	}
	delete(m.rows, id)
	return nil
}

func (m *MemoryStore) ListByTenant(_ context.Context, tenantID string) ([]OrgSSOProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []OrgSSOProvider
	for _, p := range m.rows {
		if p.TenantID == tenantID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemoryStore) ListByTenantKind(_ context.Context, tenantID string, kind Kind) ([]OrgSSOProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []OrgSSOProvider
	for _, p := range m.rows {
		if p.TenantID == tenantID && p.Kind == kind {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (m *MemoryStore) FindGitHubGrantingOrgs(_ context.Context, keys []string) ([]OrgSSOProvider, error) {
	want := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		want[k] = struct{}{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []OrgSSOProvider
	for _, p := range m.rows {
		if p.Kind != KindGitHub || !p.Enabled {
			continue
		}
		for _, k := range p.GitHubTeamKeys {
			if _, ok := want[k]; ok {
				out = append(out, p)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
