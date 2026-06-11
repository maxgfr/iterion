package pat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/SocialGouv/iterion/pkg/internal/mongoutil"
)

const colTokens = "personal_access_tokens"

// MongoStore is the production PAT store.
type MongoStore struct{ col *mongo.Collection }

func NewMongoStore(db *mongo.Database) *MongoStore {
	return &MongoStore{col: db.Collection(colTokens)}
}

// EnsureSchema creates the PAT indexes idempotently.
func EnsureSchema(ctx context.Context, db *mongo.Database) error {
	col := db.Collection(colTokens)
	if _, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true).SetName("token_hash_unique")},
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("user_recent")},
	}); err != nil && !mongoutil.IsIndexConflict(err) {
		return fmt.Errorf("pat: ensure indexes: %w", err)
	}
	return nil
}

func (s *MongoStore) Create(ctx context.Context, t Token) error {
	if _, err := s.col.InsertOne(ctx, t); err != nil {
		return fmt.Errorf("pat: insert: %w", err)
	}
	return nil
}

func (s *MongoStore) GetByTokenHash(ctx context.Context, hash string) (Token, error) {
	var t Token
	err := s.col.FindOne(ctx, bson.M{"token_hash": hash}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, fmt.Errorf("pat: get by hash: %w", err)
	}
	return t, nil
}

func (s *MongoStore) Get(ctx context.Context, id string) (Token, error) {
	var t Token
	err := s.col.FindOne(ctx, bson.M{"_id": id}).Decode(&t)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, fmt.Errorf("pat: get: %w", err)
	}
	return t, nil
}

func (s *MongoStore) ListByUser(ctx context.Context, userID string) ([]Token, error) {
	cur, err := s.col.Find(ctx, bson.M{"user_id": userID}, options.Find().SetSort(bson.M{"created_at": -1}))
	if err != nil {
		return nil, fmt.Errorf("pat: list: %w", err)
	}
	defer cur.Close(ctx)
	var out []Token
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("pat: decode: %w", err)
	}
	return out, nil
}

func (s *MongoStore) Revoke(ctx context.Context, id string, at time.Time) error {
	res, err := s.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"revoked_at": at}})
	if err != nil {
		return fmt.Errorf("pat: revoke: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *MongoStore) MarkUsed(ctx context.Context, id string, at time.Time) error {
	_, err := s.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"last_used_at": at}})
	if err != nil {
		return fmt.Errorf("pat: mark used: %w", err)
	}
	return nil
}
