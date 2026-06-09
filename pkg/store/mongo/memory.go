package mongo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/SocialGouv/iterion/pkg/internal/mongoutil"
	"github.com/SocialGouv/iterion/pkg/knowledge"
)

// Cloud MemoryStore: shared-knowledge spaces persisted in Mongo, with
// document bodies stored INLINE (markdown is capped at 2 MiB, well under
// Mongo's 16 MB document limit). A future binary mode can move bodies to
// blob storage; for V1 inline keeps the adapter self-contained and the
// quota accounting transactional within Mongo.
//
// Quota is enforced at two levels before bytes land: the per-org
// aggregate ceiling (memory_tenant_usage) and the per-space sub-cap
// (memory_spaces.used_bytes). Both use conditional ($expr) updates so an
// over-quota write is denied atomically; a denied/failed write rolls the
// other counter back.

const (
	colMemorySpaces      = "memory_spaces"
	colMemoryDocs        = "memory_docs"
	colMemoryTenantUsage = "memory_tenant_usage"
)

type memSpaceDoc struct {
	ID         string    `bson:"_id"`
	TenantID   string    `bson:"tenant_id,omitempty"`
	Visibility string    `bson:"visibility"`
	ProjectID  string    `bson:"project_id,omitempty"`
	BotID      string    `bson:"bot_id,omitempty"`
	UserID     string    `bson:"user_id,omitempty"`
	Name       string    `bson:"name"`
	QuotaBytes int64     `bson:"quota_bytes"`
	UsedBytes  int64     `bson:"used_bytes"`
	Mode       string    `bson:"mode"`
	Version    int       `bson:"version"`
	CreatedAt  time.Time `bson:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
}

type memDocID struct {
	SpaceID string `bson:"space_id"`
	Path    string `bson:"path"`
}

type memDoc struct {
	ID          memDocID  `bson:"_id"`
	TenantID    string    `bson:"tenant_id,omitempty"`
	SpaceID     string    `bson:"space_id"`
	Path        string    `bson:"path"`
	Title       string    `bson:"title,omitempty"`
	Description string    `bson:"description,omitempty"`
	Tags        []string  `bson:"tags,omitempty"`
	Size        int64     `bson:"size"`
	Checksum    string    `bson:"checksum,omitempty"`
	Revision    int64     `bson:"revision"`
	Content     []byte    `bson:"content"`
	UpdatedBy   string    `bson:"updated_by,omitempty"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

// MongoMemoryStore implements knowledge.MemoryStore over Mongo.
type MongoMemoryStore struct {
	spaces *mongo.Collection
	docs   *mongo.Collection
	tenant *mongo.Collection
}

var _ knowledge.MemoryStore = (*MongoMemoryStore)(nil)

// NewMongoMemoryStore wires the store to a database (reuse Store.DB()).
func NewMongoMemoryStore(db *mongo.Database) *MongoMemoryStore {
	return &MongoMemoryStore{
		spaces: db.Collection(colMemorySpaces),
		docs:   db.Collection(colMemoryDocs),
		tenant: db.Collection(colMemoryTenantUsage),
	}
}

// EnsureSchema creates the indexes idempotently.
func (s *MongoMemoryStore) EnsureSchema(ctx context.Context) error {
	if _, err := s.spaces.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "visibility", Value: 1}, {Key: "updated_at", Value: -1}}, Options: options.Index().SetName("tenant_vis_recent")},
	}); err != nil && !mongoutil.IsIndexConflict(err) {
		return fmt.Errorf("memory: ensure spaces indexes: %w", err)
	}
	if _, err := s.docs.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "space_id", Value: 1}, {Key: "updated_at", Value: -1}}, Options: options.Index().SetName("space_recent")},
	}); err != nil && !mongoutil.IsIndexConflict(err) {
		return fmt.Errorf("memory: ensure docs indexes: %w", err)
	}
	return nil
}

func docFilter(spaceID, path string) bson.M {
	return bson.M{"_id": memDocID{SpaceID: spaceID, Path: path}}
}

// Root returns a mem:// display URI (no IO).
func (s *MongoMemoryStore) Root(ref knowledge.SpaceRef) (string, error) {
	if err := ref.Validate(); err != nil {
		return "", err
	}
	return "mem://" + string(ref.Visibility) + "/" + ref.ID(), nil
}

func (s *MongoMemoryStore) allDocs(ctx context.Context, spaceID string) ([]memDoc, error) {
	cur, err := s.docs.Find(ctx, bson.M{"space_id": spaceID}, options.Find().SetSort(bson.M{"path": 1}))
	if err != nil {
		return nil, fmt.Errorf("memory: list docs: %w", err)
	}
	defer cur.Close(ctx)
	var out []memDoc
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("memory: decode docs: %w", err)
	}
	return out, nil
}

func (s *MongoMemoryStore) BuildIndex(ctx context.Context, ref knowledge.SpaceRef) ([]knowledge.IndexEntry, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	docs, err := s.allDocs(ctx, ref.ID())
	if err != nil {
		return nil, err
	}
	var out []knowledge.IndexEntry
	for _, d := range docs {
		if filepath.Ext(d.Path) != ".md" {
			continue
		}
		out = append(out, knowledge.IndexEntry{Path: d.Path, Title: d.Title, Description: d.Description, Tags: d.Tags})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *MongoMemoryStore) Autoload(ctx context.Context, ref knowledge.SpaceRef, patterns []string) ([]knowledge.AutoloadEntry, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	docs, err := s.allDocs(ctx, ref.ID())
	if err != nil {
		return nil, err
	}
	var out []knowledge.AutoloadEntry
	seen := map[string]bool{}
	for _, d := range docs {
		for _, p := range patterns {
			if ok, _ := filepath.Match(p, d.Path); ok && !seen[d.Path] {
				seen[d.Path] = true
				out = append(out, knowledge.AutoloadEntry{Path: d.Path, Content: d.Content})
				break
			}
		}
	}
	return out, nil
}

func (s *MongoMemoryStore) ListDocuments(ctx context.Context, ref knowledge.SpaceRef, dir string) ([]knowledge.DocumentMeta, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	docs, err := s.allDocs(ctx, ref.ID())
	if err != nil {
		return nil, err
	}
	prefix := dir
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	var out []knowledge.DocumentMeta
	for _, d := range docs {
		if prefix != "" && (len(d.Path) < len(prefix) || d.Path[:len(prefix)] != prefix) {
			continue
		}
		out = append(out, metaOf(d))
	}
	return out, nil
}

func (s *MongoMemoryStore) ReadDocument(ctx context.Context, ref knowledge.SpaceRef, path string) (knowledge.Document, error) {
	if err := ref.Validate(); err != nil {
		return knowledge.Document{}, err
	}
	var d memDoc
	err := s.docs.FindOne(ctx, docFilter(ref.ID(), path)).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return knowledge.Document{}, fmt.Errorf("%w: %q", knowledge.ErrDocNotFound, path)
	}
	if err != nil {
		return knowledge.Document{}, fmt.Errorf("memory: read doc: %w", err)
	}
	return knowledge.Document{Meta: metaOf(d), Content: d.Content}, nil
}

func metaOf(d memDoc) knowledge.DocumentMeta {
	return knowledge.DocumentMeta{
		Path: d.Path, Title: d.Title, Description: d.Description, Tags: d.Tags,
		Size: d.Size, Checksum: d.Checksum, Revision: d.Revision,
		UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt,
	}
}

func (s *MongoMemoryStore) UsageBytes(ctx context.Context, ref knowledge.SpaceRef) (int64, int64, error) {
	if err := ref.Validate(); err != nil {
		return 0, 0, err
	}
	var sp memSpaceDoc
	err := s.spaces.FindOne(ctx, bson.M{"_id": ref.ID()}).Decode(&sp)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, knowledge.DefaultQuotaFor(ref.Visibility), nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("memory: usage: %w", err)
	}
	return sp.UsedBytes, sp.QuotaBytes, nil
}

func (s *MongoMemoryStore) DeleteDocument(ctx context.Context, ref knowledge.SpaceRef, path string) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	var d memDoc
	err := s.docs.FindOne(ctx, docFilter(ref.ID(), path)).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("memory: delete lookup: %w", err)
	}
	if _, err := s.docs.DeleteOne(ctx, docFilter(ref.ID(), path)); err != nil {
		return fmt.Errorf("memory: delete doc: %w", err)
	}
	if d.Size > 0 {
		_, _ = s.bumpSpace(ctx, ref.ID(), -d.Size)
		if ref.TenantID != "" {
			_, _ = s.bumpTenant(ctx, ref.TenantID, -d.Size)
		}
	}
	return nil
}

// WriteDocument is the transactional path. See package comment.
func (s *MongoMemoryStore) WriteDocument(ctx context.Context, ref knowledge.SpaceRef, in knowledge.DocumentInput) (knowledge.DocumentMeta, error) {
	if err := ref.Validate(); err != nil {
		return knowledge.DocumentMeta{}, err
	}
	newSize := int64(len(in.Content))
	if newSize > knowledge.DefaultMaxDocumentSize {
		return knowledge.DocumentMeta{}, &knowledge.QuotaError{Aggregate: false, Used: 0, Delta: newSize, Quota: knowledge.DefaultMaxDocumentSize}
	}
	spaceID := ref.ID()

	var existing memDoc
	var oldSize, prevRev int64
	if err := s.docs.FindOne(ctx, docFilter(spaceID, in.Path)).Decode(&existing); err == nil {
		oldSize, prevRev = existing.Size, existing.Revision
	}
	if in.ExpectedRev != 0 && in.ExpectedRev != prevRev {
		return knowledge.DocumentMeta{}, fmt.Errorf("memory: revision conflict (expected %d, have %d)", in.ExpectedRev, prevRev)
	}
	delta := newSize - oldSize

	if err := s.ensureSpace(ctx, ref); err != nil {
		return knowledge.DocumentMeta{}, err
	}
	tenantID := ref.TenantID

	// 1) org aggregate ceiling.
	if tenantID != "" && delta > 0 {
		if err := s.ensureTenant(ctx, tenantID); err != nil {
			return knowledge.DocumentMeta{}, err
		}
		ok, err := s.bumpTenant(ctx, tenantID, delta)
		if err != nil {
			return knowledge.DocumentMeta{}, err
		}
		if !ok {
			used, quota := s.tenantUsage(ctx, tenantID)
			return knowledge.DocumentMeta{}, &knowledge.QuotaError{Aggregate: true, Used: used, Delta: delta, Quota: quota}
		}
	}

	// 2) per-space sub-cap.
	if delta > 0 {
		ok, err := s.bumpSpace(ctx, spaceID, delta)
		if err != nil || !ok {
			if tenantID != "" {
				_, _ = s.bumpTenant(ctx, tenantID, -delta) // rollback aggregate
			}
			if err != nil {
				return knowledge.DocumentMeta{}, err
			}
			used, quota := s.spaceUsage(ctx, spaceID)
			return knowledge.DocumentMeta{}, &knowledge.QuotaError{Aggregate: false, Used: used, Delta: delta, Quota: quota}
		}
	} else if delta < 0 {
		_, _ = s.bumpSpace(ctx, spaceID, delta)
		if tenantID != "" {
			_, _ = s.bumpTenant(ctx, tenantID, delta)
		}
	}

	// 3) upsert the doc.
	title, desc, tags := knowledge.ParseMarkdownMeta(in.Content)
	now := time.Now().UTC()
	doc := memDoc{
		ID: memDocID{SpaceID: spaceID, Path: in.Path}, TenantID: tenantID, SpaceID: spaceID,
		Path: in.Path, Title: title, Description: desc, Tags: tags,
		Size: newSize, Checksum: knowledge.ChecksumHex(in.Content), Revision: prevRev + 1,
		Content: in.Content, UpdatedBy: in.UpdatedBy, UpdatedAt: now,
	}
	if _, err := s.docs.ReplaceOne(ctx, docFilter(spaceID, in.Path), doc, options.Replace().SetUpsert(true)); err != nil {
		if delta != 0 { // roll counters back on a failed body write
			_, _ = s.bumpSpace(ctx, spaceID, -delta)
			if tenantID != "" {
				_, _ = s.bumpTenant(ctx, tenantID, -delta)
			}
		}
		return knowledge.DocumentMeta{}, fmt.Errorf("memory: write doc: %w", err)
	}
	return metaOf(doc), nil
}

// ---- quota helpers ----

func (s *MongoMemoryStore) ensureTenant(ctx context.Context, tenant string) error {
	_, err := s.tenant.UpdateOne(ctx, bson.M{"_id": tenant},
		bson.M{"$setOnInsert": bson.M{"used_bytes": int64(0), "quota_bytes": knowledge.DefaultOrgAggregateQuota}},
		options.UpdateOne().SetUpsert(true))
	return err
}

func (s *MongoMemoryStore) bumpTenant(ctx context.Context, tenant string, delta int64) (bool, error) {
	if delta <= 0 {
		_, err := s.tenant.UpdateOne(ctx, bson.M{"_id": tenant}, bson.M{"$inc": bson.M{"used_bytes": delta}})
		_ = s.clampUsage(ctx, s.tenant, tenant)
		return true, err
	}
	res, err := s.tenant.UpdateOne(ctx, bson.M{"_id": tenant, "$expr": lteAddExpr(delta)}, bson.M{"$inc": bson.M{"used_bytes": delta}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

func (s *MongoMemoryStore) ensureSpace(ctx context.Context, ref knowledge.SpaceRef) error {
	now := time.Now().UTC()
	_, err := s.spaces.UpdateOne(ctx, bson.M{"_id": ref.ID()}, bson.M{
		"$setOnInsert": bson.M{
			"tenant_id": ref.TenantID, "visibility": string(ref.Visibility),
			"project_id": ref.ProjectID, "bot_id": ref.BotID, "user_id": ref.UserID,
			"name": ref.Name, "used_bytes": int64(0),
			"quota_bytes": knowledge.DefaultQuotaFor(ref.Visibility),
			"mode":        "markdown", "version": 1, "created_at": now,
		},
		"$set": bson.M{"updated_at": now},
	}, options.UpdateOne().SetUpsert(true))
	return err
}

func (s *MongoMemoryStore) bumpSpace(ctx context.Context, spaceID string, delta int64) (bool, error) {
	if delta <= 0 {
		_, err := s.spaces.UpdateOne(ctx, bson.M{"_id": spaceID}, bson.M{"$inc": bson.M{"used_bytes": delta}})
		_ = s.clampUsage(ctx, s.spaces, spaceID)
		return true, err
	}
	// quota_bytes == 0 means "no sub-cap" (e.g. global) → always allow.
	res, err := s.spaces.UpdateOne(ctx, bson.M{"_id": spaceID, "$or": []bson.M{
		{"quota_bytes": int64(0)},
		{"$expr": lteAddExpr(delta)},
	}}, bson.M{"$inc": bson.M{"used_bytes": delta}})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// lteAddExpr builds {$lte: [{$add: ["$used_bytes", delta]}, "$quota_bytes"]}.
func lteAddExpr(delta int64) bson.M {
	return bson.M{"$lte": bson.A{bson.M{"$add": bson.A{"$used_bytes", delta}}, "$quota_bytes"}}
}

// clampUsage resets a negative used_bytes (accounting drift) back to 0.
func (s *MongoMemoryStore) clampUsage(ctx context.Context, coll *mongo.Collection, id string) error {
	_, err := coll.UpdateOne(ctx, bson.M{"_id": id, "used_bytes": bson.M{"$lt": 0}}, bson.M{"$set": bson.M{"used_bytes": int64(0)}})
	return err
}

func (s *MongoMemoryStore) tenantUsage(ctx context.Context, tenant string) (used, quota int64) {
	var t struct {
		Used  int64 `bson:"used_bytes"`
		Quota int64 `bson:"quota_bytes"`
	}
	if err := s.tenant.FindOne(ctx, bson.M{"_id": tenant}).Decode(&t); err != nil {
		return 0, knowledge.DefaultOrgAggregateQuota
	}
	return t.Used, t.Quota
}

func (s *MongoMemoryStore) spaceUsage(ctx context.Context, spaceID string) (used, quota int64) {
	var sp memSpaceDoc
	if err := s.spaces.FindOne(ctx, bson.M{"_id": spaceID}).Decode(&sp); err != nil {
		return 0, 0
	}
	return sp.UsedBytes, sp.QuotaBytes
}
