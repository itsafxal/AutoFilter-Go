package gfilter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoProvider is a function that returns the primary MongoDB database instance.
type MongoProvider func() *mongo.Database

// MongoRepository manages persistent storage of Global Filters in the primary MongoDB database.
type MongoRepository struct {
	dbProvider MongoProvider
	staticDB   *mongo.Database
}

// NewMongoRepository creates a new MongoRepository with a static or dynamic database provider.
func NewMongoRepository(provider MongoProvider) *MongoRepository {
	return &MongoRepository{dbProvider: provider}
}

// NewMongoRepositoryWithDB creates a new MongoRepository with a static database pointer.
func NewMongoRepositoryWithDB(db *mongo.Database) *MongoRepository {
	return &MongoRepository{staticDB: db}
}

func (r *MongoRepository) col() *mongo.Collection {
	if r == nil {
		return nil
	}
	if r.dbProvider != nil {
		if db := r.dbProvider(); db != nil {
			return db.Collection("global_filters")
		}
	}
	if r.staticDB != nil {
		return r.staticDB.Collection("global_filters")
	}
	return nil
}

// Init ensures the unique index on keyword_norm exists.
func (r *MongoRepository) Init(ctx context.Context) error {
	col := r.col()
	if col == nil {
		return errors.New("mongodb collection unavailable")
	}

	opts := options.Index().SetUnique(true).SetName("uniq_gfilter_keyword_norm")
	model := mongo.IndexModel{
		Keys:    bson.D{{Key: "keyword_norm", Value: 1}},
		Options: opts,
	}

	_, err := col.Indexes().CreateOne(ctx, model)
	if err != nil {
		// Index may already exist with equivalent key pattern
		return nil
	}
	return nil
}

// Save persists a Global Filter in MongoDB using an idempotent ReplaceOne upsert on keyword_norm.
func (r *MongoRepository) Save(ctx context.Context, f *GFilter) error {
	if f == nil {
		return errors.New("cannot save nil global filter")
	}
	col := r.col()
	if col == nil {
		return errors.New("mongodb collection unavailable")
	}

	f.KeywordNorm = NormalizeKeyword(f.Keyword)
	if f.KeywordNorm == "" {
		return errors.New("keyword cannot be empty")
	}

	now := time.Now().UTC()
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now

	f.PackButtons()

	// If ID is empty, generate a new hex string ID
	if f.ID == "" {
		f.ID = primitive.NewObjectID().Hex()
	}

	filter := bson.M{"keyword_norm": f.KeywordNorm}
	update := bson.M{
		"$set": bson.M{
			"keyword":      f.Keyword,
			"keyword_norm": f.KeywordNorm,
			"reply_text":   f.ReplyText,
			"media_type":   f.MediaType,
			"file_id":      f.FileID,
			"file_ref":     f.FileRef,
			"buttons_json": f.ButtonsJSON,
			"updated_at":   f.UpdatedAt,
		},
		"$setOnInsert": bson.M{
			"_id":        f.ID,
			"created_at": f.CreatedAt,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save global filter %q in mongodb: %w", f.Keyword, err)
	}

	return nil
}

// GetByKeywordNorm retrieves a Global Filter by its normalized keyword.
func (r *MongoRepository) GetByKeywordNorm(ctx context.Context, norm string) (*GFilter, error) {
	col := r.col()
	if col == nil {
		return nil, errors.New("mongodb collection unavailable")
	}

	var doc GFilter
	err := col.FindOne(ctx, bson.M{"keyword_norm": norm}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find global filter: %w", err)
	}

	doc.UnpackButtons()
	return &doc, nil
}

// GetAll retrieves all Global Filters from MongoDB ordered by keyword.
func (r *MongoRepository) GetAll(ctx context.Context) ([]*GFilter, error) {
	col := r.col()
	if col == nil {
		return nil, errors.New("mongodb collection unavailable")
	}

	opts := options.Find().SetSort(bson.D{{Key: "keyword", Value: 1}})
	cursor, err := col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed querying global filters: %w", err)
	}
	defer cursor.Close(ctx)

	var filters []*GFilter
	for cursor.Next(ctx) {
		var f GFilter
		if err := cursor.Decode(&f); err != nil {
			return nil, fmt.Errorf("failed decoding global filter: %w", err)
		}
		f.UnpackButtons()
		filters = append(filters, &f)
	}

	return filters, cursor.Err()
}

// DeleteByKeywordNorm deletes a Global Filter by its normalized keyword.
func (r *MongoRepository) DeleteByKeywordNorm(ctx context.Context, norm string) (bool, error) {
	col := r.col()
	if col == nil {
		return false, errors.New("mongodb collection unavailable")
	}

	res, err := col.DeleteOne(ctx, bson.M{"keyword_norm": norm})
	if err != nil {
		return false, fmt.Errorf("failed to delete global filter %q: %w", norm, err)
	}

	return res.DeletedCount > 0, nil
}

// DeleteAll deletes all Global Filters from MongoDB.
func (r *MongoRepository) DeleteAll(ctx context.Context) (int64, error) {
	col := r.col()
	if col == nil {
		return 0, errors.New("mongodb collection unavailable")
	}

	res, err := col.DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed deleting all global filters from mongodb: %w", err)
	}

	return res.DeletedCount, nil
}

// Count returns the total number of Global Filters stored in MongoDB.
func (r *MongoRepository) Count(ctx context.Context) (int64, error) {
	col := r.col()
	if col == nil {
		return 0, nil
	}
	return col.CountDocuments(ctx, bson.M{})
}
