package server

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/SocialGouv/iterion/pkg/botregistry"
	"github.com/SocialGouv/iterion/pkg/bundle"
)

// BotsConfig configures the bot registry exposed at /api/v1/bots. It
// is an alias of botregistry.Config so the server and dispatcher
// share one type for "where to look for bots".
type BotsConfig = botregistry.Config

// effectivePaths returns the configured bot paths, falling back to the
// project-relative conventions when none are configured.
func (s *Server) effectivePaths() []string {
	if len(s.cfg.Bots.Paths) > 0 {
		return s.cfg.Bots.Paths
	}
	if s.cfg.WorkDir == "" {
		return nil
	}
	return botregistry.DefaultPaths(s.cfg.WorkDir)
}

// botListOptions builds the discovery options for the bot endpoints,
// passing WorkDir so each Entry.Enabled reflects the workspace overlay
// (.iterion/bot-overrides.yaml) composed over the manifest default.
func (s *Server) botListOptions() botregistry.ListOptions {
	return botregistry.ListOptions{Paths: s.effectivePaths(), Workdir: s.cfg.WorkDir}
}

// handleBotsList returns the discovered bots' metadata + vars schema.
// The list payload always includes schemas — the studio's bot picker
// renders the typed form inline on selection, so a separate "lite"
// endpoint would just double the request count. Disabled bots are
// included (Enabled=false) so the studio can show them to flip back on.
func (s *Server) handleBotsList(w http.ResponseWriter, r *http.Request) {
	if len(s.effectivePaths()) == 0 {
		s.writeJSONFor(w, r, map[string]any{"bots": []any{}})
		return
	}
	entries, err := botregistry.ListWithSchema(s.botListOptions())
	if err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "bots: %v", err)
		return
	}
	s.writeJSONFor(w, r, map[string]any{"bots": entries})
}

// handleBotsGet returns one bot with its full schema. Returns 404 when
// the bot name doesn't match any discovered entry.
func (s *Server) handleBotsGet(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.httpErrorFor(w, r, http.StatusBadRequest, "bots: missing name")
		return
	}
	entry, ok, err := s.findBot(name)
	if err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "bots: %v", err)
		return
	}
	if !ok {
		s.httpErrorFor(w, r, http.StatusNotFound, "bots: %q not found", name)
		return
	}
	s.writeJSONFor(w, r, entry)
}

// botUpdateRequest is the wire body for PUT /api/v1/bots/{name}. Pointer
// fields distinguish "omitted (no change)" from "set to empty string".
// The bot's technical name is the URL pathvar and is NOT renamable here
// (rename cascades through the catalog, dispatcher routing, and ticket
// history — out of scope).
type botUpdateRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Description *string `json:"description,omitempty"`
	Author      *string `json:"author,omitempty"`
	Version     *string `json:"version,omitempty"`
	WhenToUse   *string `json:"when_to_use,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

// handleBotsPut updates a bot's manifest.yaml in place (the studio Bot
// metadata panel). Bundle-only: a loose .bot file has no manifest to
// write, so it returns 409. On success it regenerates the orchestrator
// catalog so the edit reaches Nexie, then returns the refreshed entry.
func (s *Server) handleBotsPut(w http.ResponseWriter, r *http.Request) {
	if !s.requireSafeOrigin(w, r) {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.httpErrorFor(w, r, http.StatusBadRequest, "bots: missing name")
		return
	}
	entry, ok, err := s.findBot(name)
	if err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "bots: %v", err)
		return
	}
	if !ok {
		s.httpErrorFor(w, r, http.StatusNotFound, "bots: %q not found", name)
		return
	}
	if !entry.IsBundleDir {
		s.httpErrorFor(w, r, http.StatusConflict,
			"bots: %q is a loose .bot file; convert it to a bundle (manifest.yaml + main.bot) to edit metadata", name)
		return
	}
	var req botUpdateRequest
	if err := readJSON(r, &req); err != nil {
		s.httpErrorFor(w, r, http.StatusBadRequest, "invalid request: %v", err)
		return
	}

	manifestPath := filepath.Join(entry.Path, "manifest.yaml")
	patch := bundle.ManifestPatch{
		DisplayName: req.DisplayName,
		Description: req.Description,
		Author:      req.Author,
		Version:     req.Version,
		WhenToUse:   req.WhenToUse,
		Enabled:     req.Enabled,
	}
	if _, err := bundle.WriteManifest(manifestPath, patch); err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "bots: write manifest: %v", err)
		return
	}
	s.regenCatalog(name, "update")
	s.respondBot(w, r, name)
}

// botOverlayRequest is the wire body for PUT /api/v1/bots/{name}/overlay.
// A null/omitted enabled clears the workspace override (the manifest
// default stands again); true/false pins the bot on/off for this
// workspace without editing the (possibly git-tracked) manifest.
type botOverlayRequest struct {
	Enabled *bool `json:"enabled"`
}

// handleBotOverlay sets a bot's workspace-local catalog-visibility
// override (the studio Catalog manager quick-toggle), then regenerates
// the catalog and returns the refreshed entry.
func (s *Server) handleBotOverlay(w http.ResponseWriter, r *http.Request) {
	if !s.requireSafeOrigin(w, r) {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.httpErrorFor(w, r, http.StatusBadRequest, "bots: missing name")
		return
	}
	if s.cfg.WorkDir == "" {
		s.httpErrorFor(w, r, http.StatusBadRequest, "bots: no workspace configured for the catalog overlay")
		return
	}
	entry, ok, err := s.findBot(name)
	if err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "bots: %v", err)
		return
	}
	if !ok {
		s.httpErrorFor(w, r, http.StatusNotFound, "bots: %q not found", name)
		return
	}
	_ = entry
	var req botOverlayRequest
	if err := readJSON(r, &req); err != nil {
		s.httpErrorFor(w, r, http.StatusBadRequest, "invalid request: %v", err)
		return
	}
	if err := botregistry.SetOverlayEnabled(s.cfg.WorkDir, name, req.Enabled); err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "bots: overlay: %v", err)
		return
	}
	s.regenCatalog(name, "overlay")
	s.respondBot(w, r, name)
}

// findBot returns the schema-augmented entry for name (exact match).
func (s *Server) findBot(name string) (botregistry.EntryWithSchema, bool, error) {
	entries, err := botregistry.ListWithSchema(s.botListOptions())
	if err != nil {
		return botregistry.EntryWithSchema{}, false, err
	}
	for _, e := range entries {
		if e.Name == name {
			return e, true, nil
		}
	}
	return botregistry.EntryWithSchema{}, false, nil
}

// respondBot re-resolves name (post-mutation) and writes it as JSON.
func (s *Server) respondBot(w http.ResponseWriter, r *http.Request, name string) {
	entry, ok, err := s.findBot(name)
	if err != nil {
		s.httpErrorFor(w, r, http.StatusInternalServerError, "bots: %v", err)
		return
	}
	if !ok {
		s.httpErrorFor(w, r, http.StatusNotFound, "bots: %q not found", name)
		return
	}
	s.writeJSONFor(w, r, entry)
}

// regenCatalog refreshes the orchestrator-facing bot catalog after a
// metadata/overlay change. Best-effort: a failure must not fail the
// request (the runtime regenerates on Nexie's next run regardless).
func (s *Server) regenCatalog(name, action string) {
	if s.cfg.WorkDir == "" {
		return
	}
	if _, err := botregistry.RegenerateWhatsNextCatalog(s.cfg.WorkDir); err != nil && s.logger != nil {
		s.logger.Warn("bots: catalog regen after %q %s: %v", name, action, err)
	}
}
