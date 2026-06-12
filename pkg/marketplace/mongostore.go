package marketplace

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/SocialGouv/iterion/pkg/internal/mongoutil"
)

// colMarketplace is the Mongo collection holding registry entries.
const colMarketplace = "marketplace"

// MongoStore is the cloud-mode Store. Slug is the _id; ReplaceOne with
// upsert handles submit, and the install counter is bumped via $inc.
// Indexes mirror the JSON store's query surface (text search across
// Name/Description/Tags; popularity sort).
type MongoStore struct{ col *mongo.Collection }

// NewMongoStore wraps a Mongo database into a marketplace.Store.
func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{col: db.Collection(colMarketplace)}
}

// EnsureSchema creates the marketplace indexes idempotently. Mirrors
// the pattern used by orgusage.EnsureSchema / webhooks.EnsureSchema.
func EnsureSchema(ctx context.Context, db *mongo.Database) error {
	col := db.Collection(colMarketplace)
	if _, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		// Compound index for the popularity sort served by List —
		// (installs desc, slug asc) matches the JSON store's order.
		{Keys: bson.D{{Key: "installs", Value: -1}, {Key: "_id", Value: 1}},
			Options: options.Index().SetName("popularity")},
		// A multikey index on tags powers exact-tag filtering.
		{Keys: bson.D{{Key: "tags", Value: 1}},
			Options: options.Index().SetName("tags")},
		// Mongo text index across the fields List's text filter
		// searches. Lets the query planner serve large registries
		// without a full-collection scan; the JSON store equivalent
		// is a linear scan since the dataset there is tiny.
		{Keys: bson.D{
			{Key: "name", Value: "text"},
			{Key: "display_name", Value: "text"},
			{Key: "description", Value: "text"},
			{Key: "author", Value: "text"},
			{Key: "tags", Value: "text"},
		}, Options: options.Index().SetName("text_search")},
	}); err != nil && !mongoutil.IsIndexConflict(err) {
		return fmt.Errorf("marketplace: ensure indexes: %w", err)
	}
	return nil
}

// List returns every entry matching q, sorted by Installs desc, then
// Slug asc — same shape as JSONStore.List.
func (s *MongoStore) List(ctx context.Context, q Query) ([]Entry, error) {
	filter := bson.M{}
	if t := q.Tag; t != "" {
		filter["tags"] = t
	}
	if text := q.Text; text != "" {
		// $text uses the text index above; falls back to a regex
		// scan over slug+name when callers query a substring the
		// stemmed text index would miss.
		filter["$or"] = bson.A{
			bson.M{"$text": bson.M{"$search": text}},
			bson.M{"_id": bson.M{"$regex": text, "$options": "i"}},
			bson.M{"name": bson.M{"$regex": text, "$options": "i"}},
		}
	}
	cur, err := s.col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "installs", Value: -1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("marketplace: find: %w", err)
	}
	defer cur.Close(ctx)
	var out []Entry
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("marketplace: decode: %w", err)
	}
	return out, nil
}

func (s *MongoStore) Get(ctx context.Context, slug string) (*Entry, bool, error) {
	var e Entry
	err := s.col.FindOne(ctx, bson.M{"_id": slug}).Decode(&e)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("marketplace: get: %w", err)
	}
	return &e, true, nil
}

// Upsert replaces (or inserts) the entry keyed by Slug. When the
// caller passes a zero Installs (submit-form case) the existing
// counter is preserved via a follow-up $set excluding it; in the
// common "first submit" case the document doesn't exist yet, so the
// $setOnInsert path seeds it to 0.
func (s *MongoStore) Upsert(ctx context.Context, e Entry) error {
	if e.Slug == "" {
		return errors.New("marketplace: slug required")
	}
	set := bson.M{
		"name":         e.Name,
		"display_name": e.DisplayName,
		"description":  e.Description,
		"author":       e.Author,
		"tags":         e.Tags,
		"repo_url":     e.RepoURL,
		"ref":          e.Ref,
		"subpath":      e.Subpath,
		"version":      e.Version,
		"readme":       e.README,
		"presets":      e.Presets,
		"updated_at":   e.UpdatedAt,
	}
	setOnInsert := bson.M{
		"installs":   0,
		"created_at": e.CreatedAt,
	}
	if e.Installs > 0 {
		// Caller (e.g. an admin reseed) explicitly carries an install
		// count over — honor it.
		set["installs"] = e.Installs
		delete(setOnInsert, "installs")
	}
	_, err := s.col.UpdateOne(ctx,
		bson.M{"_id": e.Slug},
		bson.M{"$set": set, "$setOnInsert": setOnInsert},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("marketplace: upsert: %w", err)
	}
	return nil
}

// IncrementInstalls bumps installs by 1. Returns an error when slug is
// unknown; the install endpoint validates the entry exists before
// calling.
func (s *MongoStore) IncrementInstalls(ctx context.Context, slug string) error {
	res, err := s.col.UpdateOne(ctx, bson.M{"_id": slug}, bson.M{"$inc": bson.M{"installs": 1}})
	if err != nil {
		return fmt.Errorf("marketplace: increment installs: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("marketplace: entry %q not found", slug)
	}
	return nil
}
