package mongodb

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UserRepository handles user persistence in the authoritative MongoDB "users" collection.
type UserRepository struct {
	col *mongo.Collection
	mu  sync.RWMutex
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db *mongo.Database) *UserRepository {
	if db == nil {
		return nil
	}
	return &UserRepository{
		col: db.Collection("users"),
	}
}

// AddUser adds a new user if not already present.
func (r *UserRepository) AddUser(ctx context.Context, id int64, name string) error {
	if r == nil || r.col == nil || id <= 0 {
		return errors.New("invalid repository or user id")
	}

	exists, err := r.IsUserExist(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	userName := name
	if userName == "" {
		userName = "User"
	}

	userDoc := User{
		ID:   id,
		Name: userName,
		BanStatus: BanStatus{
			IsBanned:  false,
			BanReason: "",
		},
	}

	_, err = r.col.InsertOne(ctx, userDoc)
	if err != nil {
		// Ignore duplicate key error if another concurrent call inserted
		if mongo.IsDuplicateKeyError(err) {
			return nil
		}
		return fmt.Errorf("failed to insert user %d: %w", id, err)
	}

	return nil
}

// IsUserExist checks whether a user with the given ID exists in the collection.
func (r *UserRepository) IsUserExist(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.col == nil || id <= 0 {
		return false, nil
	}

	count, err := r.col.CountDocuments(ctx, bson.M{"id": id}, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}
	return count > 0, nil
}

// GetUser retrieves a user by ID.
func (r *UserRepository) GetUser(ctx context.Context, id int64) (*User, error) {
	if r == nil || r.col == nil || id <= 0 {
		return nil, errors.New("invalid user id")
	}

	var user User
	err := r.col.FindOne(ctx, bson.M{"id": id}).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find user %d: %w", id, err)
	}
	return &user, nil
}

// TotalUsersCount returns the total number of registered users.
func (r *UserRepository) TotalUsersCount(ctx context.Context) (int64, error) {
	if r == nil || r.col == nil {
		return 0, nil
	}
	return r.col.CountDocuments(ctx, bson.M{})
}

// BanUser updates user ban status.
func (r *UserRepository) BanUser(ctx context.Context, id int64, reason string) error {
	if r == nil || r.col == nil {
		return errors.New("repository uninitialized")
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"id": id}, bson.M{
		"$set": bson.M{
			"ban_status": bson.M{
				"is_banned":  true,
				"ban_reason": reason,
			},
		},
	})
	return err
}

// RemoveBan clears user ban status.
func (r *UserRepository) RemoveBan(ctx context.Context, id int64) error {
	if r == nil || r.col == nil {
		return errors.New("repository uninitialized")
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"id": id}, bson.M{
		"$set": bson.M{
			"ban_status": bson.M{
				"is_banned":  false,
				"ban_reason": "",
			},
		},
	})
	return err
}

// GroupRepository handles group persistence in the authoritative MongoDB "groups" collection.
type GroupRepository struct {
	col *mongo.Collection
}

// NewGroupRepository creates a new GroupRepository.
func NewGroupRepository(db *mongo.Database) *GroupRepository {
	if db == nil {
		return nil
	}
	return &GroupRepository{
		col: db.Collection("groups"),
	}
}

// AddGroup adds a new group chat if not already present.
func (r *GroupRepository) AddGroup(ctx context.Context, id int64, title string) error {
	if r == nil || r.col == nil || id == 0 {
		return errors.New("invalid repository or chat id")
	}

	exists, err := r.IsGroupExist(ctx, id)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	chatTitle := title
	if chatTitle == "" {
		chatTitle = "Group"
	}

	groupDoc := Group{
		ID:    id,
		Title: chatTitle,
		ChatStatus: ChatStatus{
			IsDisabled: false,
			Reason:     "",
		},
	}

	_, err = r.col.InsertOne(ctx, groupDoc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil
		}
		return fmt.Errorf("failed to insert group %d: %w", id, err)
	}

	return nil
}

// IsGroupExist checks whether a group chat exists.
func (r *GroupRepository) IsGroupExist(ctx context.Context, id int64) (bool, error) {
	if r == nil || r.col == nil || id == 0 {
		return false, nil
	}

	count, err := r.col.CountDocuments(ctx, bson.M{"id": id}, options.Count().SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("failed to check group existence: %w", err)
	}
	return count > 0, nil
}

// GetGroup retrieves a group document by ID.
func (r *GroupRepository) GetGroup(ctx context.Context, id int64) (*Group, error) {
	if r == nil || r.col == nil || id == 0 {
		return nil, errors.New("invalid group id")
	}

	var grp Group
	err := r.col.FindOne(ctx, bson.M{"id": id}).Decode(&grp)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find group %d: %w", id, err)
	}
	return &grp, nil
}

// TotalGroupsCount returns total connected group chats.
func (r *GroupRepository) TotalGroupsCount(ctx context.Context) (int64, error) {
	if r == nil || r.col == nil {
		return 0, nil
	}
	return r.col.CountDocuments(ctx, bson.M{})
}

// MediaRepository provides read access to the authoritative MongoDB media collections
// across primary and replica/sharded databases (DB2, DB3).
type MediaRepository struct {
	collections []*mongo.Collection
}

// NewMediaRepository creates a new MediaRepository referencing configured media collections.
func NewMediaRepository(colName string, databases ...*mongo.Database) *MediaRepository {
	cols := make([]*mongo.Collection, 0, len(databases))
	seen := make(map[string]struct{})

	for _, db := range databases {
		if db == nil {
			continue
		}
		key := db.Name() + "/" + colName
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		cols = append(cols, db.Collection(colName))
	}

	return &MediaRepository{
		collections: cols,
	}
}

// TotalMediaCount counts documents across all configured media collections.
func (r *MediaRepository) TotalMediaCount(ctx context.Context) (int64, error) {
	if r == nil || len(r.collections) == 0 {
		return 0, nil
	}

	var total int64
	for _, col := range r.collections {
		count, err := col.CountDocuments(ctx, bson.M{})
		if err != nil {
			return total, fmt.Errorf("error counting documents in %s: %w", col.Name(), err)
		}
		total += count
	}
	return total, nil
}

// StreamAllDocuments streams all MediaDoc records across all media collections in order to
// feed the SQLite synchronization engine with minimal memory consumption.
func (r *MediaRepository) StreamAllDocuments(ctx context.Context, callback func(doc MediaDoc) error) (int64, error) {
	if r == nil || len(r.collections) == 0 {
		return 0, nil
	}

	var totalStreamed int64

	for _, col := range r.collections {
		cursor, err := col.Find(ctx, bson.M{}, options.Find().SetBatchSize(1000))
		if err != nil {
			return totalStreamed, fmt.Errorf("failed to open cursor on collection %s: %w", col.Name(), err)
		}

		for cursor.Next(ctx) {
			var doc MediaDoc
			if err := cursor.Decode(&doc); err != nil {
				// Log or skip malformed document rather than failing entire synchronization
				continue
			}

			if err := callback(doc); err != nil {
				_ = cursor.Close(ctx)
				return totalStreamed, err
			}
			totalStreamed++
		}

		if err := cursor.Err(); err != nil {
			_ = cursor.Close(ctx)
			return totalStreamed, fmt.Errorf("cursor error on collection %s: %w", col.Name(), err)
		}
		_ = cursor.Close(ctx)
	}

	return totalStreamed, nil
}

// GetFileDetails fetches details of a media document by file_id from authoritative MongoDB.
func (r *MediaRepository) GetFileDetails(ctx context.Context, fileID string) (*MediaDoc, error) {
	if r == nil || len(r.collections) == 0 || fileID == "" {
		return nil, nil
	}

	for _, col := range r.collections {
		var doc MediaDoc
		err := col.FindOne(ctx, bson.M{"_id": fileID}).Decode(&doc)
		if err == nil {
			return &doc, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}

	return nil, nil
}
