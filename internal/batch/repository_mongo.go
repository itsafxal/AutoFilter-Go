package batch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoProvider is a function that returns the primary MongoDB database instance.
type MongoProvider func() *mongo.Database

// MongoRepository manages persistent, authoritative storage of batches in MongoDB.
type MongoRepository struct {
	dbProvider MongoProvider
	staticDB   *mongo.Database
}

// NewMongoRepository creates a MongoRepository with a dynamic dbProvider.
func NewMongoRepository(provider MongoProvider) *MongoRepository {
	return &MongoRepository{dbProvider: provider}
}

// NewMongoRepositoryWithDB creates a MongoRepository with a static database pointer.
func NewMongoRepositoryWithDB(db *mongo.Database) *MongoRepository {
	return &MongoRepository{staticDB: db}
}

func (r *MongoRepository) metaCol() *mongo.Collection {
	if r == nil {
		return nil
	}
	if r.dbProvider != nil {
		if db := r.dbProvider(); db != nil {
			return db.Collection("batch_metadata")
		}
	}
	if r.staticDB != nil {
		return r.staticDB.Collection("batch_metadata")
	}
	return nil
}

func (r *MongoRepository) itemsCol() *mongo.Collection {
	if r == nil {
		return nil
	}
	if r.dbProvider != nil {
		if db := r.dbProvider(); db != nil {
			return db.Collection("batch_items")
		}
	}
	if r.staticDB != nil {
		return r.staticDB.Collection("batch_items")
	}
	return nil
}

// Init creates authoritative MongoDB indexes for metadata and items collections.
func (r *MongoRepository) Init(ctx context.Context) error {
	metaCol := r.metaCol()
	itemsCol := r.itemsCol()
	if metaCol == nil || itemsCol == nil {
		return errors.New("mongodb batch collections unavailable")
	}

	// 1. Metadata indexes
	metaIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "batch_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_batch_id"),
		},
		{
			Keys:    bson.D{{Key: "normalized_name", Value: 1}},
			Options: options.Index().SetName("idx_batch_norm_name"),
		},
		{
			Keys:    bson.D{{Key: "owner_id", Value: 1}},
			Options: options.Index().SetName("idx_batch_owner"),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_batch_created"),
		},
	}
	for _, m := range metaIndexes {
		_, _ = metaCol.Indexes().CreateOne(ctx, m)
	}

	// 2. Items indexes
	itemIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "batch_id", Value: 1}, {Key: "sequence", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("uniq_batch_item_seq"),
		},
		{
			Keys:    bson.D{{Key: "batch_id", Value: 1}},
			Options: options.Index().SetName("idx_item_batch_id"),
		},
	}
	for _, m := range itemIndexes {
		_, _ = itemsCol.Indexes().CreateOne(ctx, m)
	}

	return nil
}

// SaveMetadata inserts or replaces a batch metadata record in MongoDB.
func (r *MongoRepository) SaveMetadata(ctx context.Context, meta *BatchMetadata) error {
	if meta == nil || meta.BatchID == "" {
		return errors.New("cannot save nil or empty batch metadata")
	}
	col := r.metaCol()
	if col == nil {
		return errors.New("mongodb batch_metadata collection unavailable")
	}

	meta.NormalizedName = NormalizeBatchName(meta.BatchName)
	now := time.Now().UTC()
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now

	filter := bson.M{"batch_id": meta.BatchID}
	update := bson.M{"$set": meta}
	opts := options.Update().SetUpsert(true)

	_, err := col.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save batch metadata in mongodb: %w", err)
	}
	return nil
}

// SaveItem inserts or updates a single batch item in MongoDB.
func (r *MongoRepository) SaveItem(ctx context.Context, item *BatchItem) error {
	if item == nil || item.BatchID == "" || item.Sequence <= 0 {
		return errors.New("cannot save invalid batch item")
	}
	col := r.itemsCol()
	if col == nil {
		return errors.New("mongodb batch_items collection unavailable")
	}

	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}

	filter := bson.M{"batch_id": item.BatchID, "sequence": item.Sequence}
	update := bson.M{"$set": item}
	opts := options.Update().SetUpsert(true)

	_, err := col.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save batch item in mongodb: %w", err)
	}
	return nil
}

// SaveItems persists multiple batch items into MongoDB sequentially or using bulk write.
func (r *MongoRepository) SaveItems(ctx context.Context, items []*BatchItem) error {
	if len(items) == 0 {
		return nil
	}
	col := r.itemsCol()
	if col == nil {
		return errors.New("mongodb batch_items collection unavailable")
	}

	models := make([]mongo.WriteModel, 0, len(items))
	now := time.Now().UTC()
	for _, item := range items {
		if item == nil || item.BatchID == "" || item.Sequence <= 0 {
			continue
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = now
		}
		filter := bson.M{"batch_id": item.BatchID, "sequence": item.Sequence}
		update := bson.M{"$set": item}
		model := mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(true)
		models = append(models, model)
	}

	if len(models) == 0 {
		return nil
	}

	opts := options.BulkWrite().SetOrdered(true)
	_, err := col.BulkWrite(ctx, models, opts)
	if err != nil {
		return fmt.Errorf("failed bulk writing batch items into mongodb: %w", err)
	}
	return nil
}

// GetMetadata retrieves a batch metadata record by BatchID.
func (r *MongoRepository) GetMetadata(ctx context.Context, batchID string) (*BatchMetadata, error) {
	col := r.metaCol()
	if col == nil {
		return nil, errors.New("mongodb batch_metadata collection unavailable")
	}

	var meta BatchMetadata
	err := col.FindOne(ctx, bson.M{"batch_id": batchID}).Decode(&meta)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed finding batch metadata: %w", err)
	}
	return &meta, nil
}

// GetItems retrieves all items belonging to a batch sorted by Sequence ASC.
func (r *MongoRepository) GetItems(ctx context.Context, batchID string) ([]*BatchItem, error) {
	col := r.itemsCol()
	if col == nil {
		return nil, errors.New("mongodb batch_items collection unavailable")
	}

	opts := options.Find().SetSort(bson.D{{Key: "sequence", Value: 1}})
	cursor, err := col.Find(ctx, bson.M{"batch_id": batchID}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed querying batch items: %w", err)
	}
	defer cursor.Close(ctx)

	var items []*BatchItem
	for cursor.Next(ctx) {
		var it BatchItem
		if err := cursor.Decode(&it); err != nil {
			return nil, fmt.Errorf("failed decoding batch item: %w", err)
		}
		items = append(items, &it)
	}
	return items, cursor.Err()
}

// GetAllFinalizedMetadata retrieves all finalized batches from MongoDB ordered by creation date descending.
func (r *MongoRepository) GetAllFinalizedMetadata(ctx context.Context) ([]*BatchMetadata, error) {
	col := r.metaCol()
	if col == nil {
		return nil, errors.New("mongodb batch_metadata collection unavailable")
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := col.Find(ctx, bson.M{"status": StatusFinalized}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed querying finalized batches from mongodb: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*BatchMetadata
	for cursor.Next(ctx) {
		var meta BatchMetadata
		if err := cursor.Decode(&meta); err != nil {
			return nil, fmt.Errorf("failed decoding batch metadata: %w", err)
		}
		results = append(results, &meta)
	}
	return results, cursor.Err()
}

// DeleteBatch removes a batch and all its sequential items from MongoDB.
func (r *MongoRepository) DeleteBatch(ctx context.Context, batchID string) error {
	metaCol := r.metaCol()
	itemsCol := r.itemsCol()
	if metaCol == nil || itemsCol == nil {
		return errors.New("mongodb batch collections unavailable")
	}

	_, _ = metaCol.DeleteOne(ctx, bson.M{"batch_id": batchID})
	_, _ = itemsCol.DeleteMany(ctx, bson.M{"batch_id": batchID})
	return nil
}

// Count returns the total number of finalized batches in MongoDB.
func (r *MongoRepository) Count(ctx context.Context) (int64, error) {
	col := r.metaCol()
	if col == nil {
		return 0, nil
	}
	return col.CountDocuments(ctx, bson.M{"status": StatusFinalized})
}
