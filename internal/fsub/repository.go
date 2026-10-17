package fsub

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DBProvider is a function that returns the current primary MongoDB database.
type DBProvider func() *mongo.Database

// Repository manages persistence for FSub loadouts and join requests in MongoDB.
type Repository struct {
	db         *mongo.Database
	dbProvider DBProvider
}

// NewRepository creates a new FSub repository given a static MongoDB database.
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{db: db}
}

// NewRepositoryWithProvider creates a new FSub repository given a dynamic DB provider.
func NewRepositoryWithProvider(provider DBProvider) *Repository {
	return &Repository{dbProvider: provider}
}

func (r *Repository) col(name string) *mongo.Collection {
	if r == nil {
		return nil
	}
	if r.dbProvider != nil {
		if db := r.dbProvider(); db != nil {
			return db.Collection(name)
		}
	}
	if r.db != nil {
		return r.db.Collection(name)
	}
	return nil
}

func (r *Repository) getReqCol(slot int) *mongo.Collection {
	switch slot {
	case 1:
		return r.col("reqone")
	case 2:
		return r.col("reqtwo")
	case 3:
		return r.col("reqthree")
	default:
		return nil
	}
}

// GetLoadout fetches the current force-subscribe configuration for the given bot ID.
func (r *Repository) GetLoadout(ctx context.Context, botID int64) (*Loadout, error) {
	col := r.col("admins")
	if col == nil {
		return &Loadout{ID: botID}, nil
	}

	var loadout Loadout
	err := col.FindOne(ctx, bson.M{"id": botID}).Decode(&loadout)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			// Try finding any existing record if stored with another or default id
			if err2 := col.FindOne(ctx, bson.M{}).Decode(&loadout); err2 == nil {
				return &loadout, nil
			}
			// Create a new loadout document matching legacy behavior
			defaultLoadout := Loadout{ID: botID}
			_, _ = col.InsertOne(ctx, defaultLoadout)
			return &defaultLoadout, nil
		}
		return nil, fmt.Errorf("failed to get loadout: %w", err)
	}
	return &loadout, nil
}

// UpdateLoadout updates a specific field in the admins document.
func (r *Repository) UpdateLoadout(ctx context.Context, botID int64, key string, val any) error {
	col := r.col("admins")
	if col == nil {
		return nil
	}
	opts := options.Update().SetUpsert(true)
	_, err := col.UpdateOne(ctx, bson.M{"id": botID}, bson.M{"$set": bson.M{key: val}}, opts)
	if err != nil {
		return fmt.Errorf("failed to update loadout field %s: %w", key, err)
	}
	return nil
}

// UpdateThreshold updates the threshold count for a slot across loadouts.
func (r *Repository) UpdateThreshold(ctx context.Context, key string, threshold int) error {
	col := r.col("admins")
	if col == nil {
		return nil
	}
	field := fmt.Sprintf("%s_threshold", key)
	_, err := col.UpdateMany(ctx, bson.M{}, bson.M{"$set": bson.M{field: threshold}})
	if err != nil {
		return fmt.Errorf("failed to update threshold %s: %w", field, err)
	}
	return nil
}

// AddReq records a user's join request in the appropriate slot collection.
func (r *Repository) AddReq(ctx context.Context, slot int, userID int64) error {
	col := r.getReqCol(slot)
	if col == nil {
		return nil
	}
	doc := bson.M{
		"user_id":    userID,
		"id":         userID,
		"created_at": time.Now(),
	}
	_, err := col.InsertOne(ctx, doc)
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("failed to insert join request in slot %d: %w", slot, err)
	}
	return nil
}

// HasReq checks if a user has sent a join request for the given slot.
func (r *Repository) HasReq(ctx context.Context, slot int, userID int64) (bool, error) {
	col := r.getReqCol(slot)
	if col == nil {
		return false, nil
	}
	filter := bson.M{
		"$or": []bson.M{
			{"user_id": userID},
			{"id": userID},
		},
	}
	count, err := col.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("failed to check join request in slot %d: %w", slot, err)
	}
	return count > 0, nil
}

// GetReqCount returns total join requests recorded for a slot.
func (r *Repository) GetReqCount(ctx context.Context, slot int) (int64, error) {
	col := r.getReqCol(slot)
	if col == nil {
		return 0, nil
	}
	count, err := col.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to count join requests in slot %d: %w", slot, err)
	}
	return count, nil
}

// DeleteAllReq removes all join request records for a slot (e.g. after channel switch or delsub).
func (r *Repository) DeleteAllReq(ctx context.Context, slot int) error {
	col := r.getReqCol(slot)
	if col == nil {
		return nil
	}
	_, err := col.DeleteMany(ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to delete join requests in slot %d: %w", slot, err)
	}
	return nil
}
