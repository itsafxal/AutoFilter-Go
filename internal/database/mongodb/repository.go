package mongodb

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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

// MediaShard represents an authoritative MongoDB collection shard with a descriptive label.
type MediaShard struct {
	Label string
	Col   *mongo.Collection
}

// ShardCount represents the file count discovered in a MongoDB shard cluster.
type ShardCount struct {
	Label string
	Count int64
}

// MediaRepository provides read access to the authoritative MongoDB media collections
// across sharded database clusters (e.g. DB2, DB3).
type MediaRepository struct {
	shards []MediaShard
}

// NewMediaRepository creates a new MediaRepository referencing configured databases with DB1..DBn labels.
func NewMediaRepository(colName string, databases ...*mongo.Database) *MediaRepository {
	shards := make([]MediaShard, 0, len(databases))
	seen := make(map[*mongo.Database]struct{})

	for i, db := range databases {
		if db == nil {
			continue
		}
		if _, exists := seen[db]; exists {
			continue
		}
		seen[db] = struct{}{}
		shards = append(shards, MediaShard{
			Label: fmt.Sprintf("DB%d", i+1),
			Col:   db.Collection(colName),
		})
	}

	return &MediaRepository{
		shards: shards,
	}
}

// NewShardedMediaRepository creates a new MediaRepository from explicitly labeled shards.
func NewShardedMediaRepository(shards ...MediaShard) *MediaRepository {
	return &MediaRepository{
		shards: shards,
	}
}

// Shards returns the configured media collection shards.
func (r *MediaRepository) Shards() []MediaShard {
	if r == nil {
		return nil
	}
	return r.shards
}

// GetShardCounts inspects document counts across each shard cluster upfront using constant-time metadata.
func (r *MediaRepository) GetShardCounts(ctx context.Context) ([]ShardCount, int64, error) {
	if r == nil || len(r.shards) == 0 {
		return nil, 0, nil
	}

	counts := make([]ShardCount, len(r.shards))
	var total int64

	for i, shard := range r.shards {
		counts[i].Label = shard.Label
		if shard.Col == nil {
			continue
		}

		// Fast metadata estimate from MongoDB collection stats
		cnt, err := shard.Col.EstimatedDocumentCount(ctx)
		if err != nil {
			// Fallback to standard CountDocuments if estimated is unsupported
			cnt, err = shard.Col.CountDocuments(ctx, bson.M{})
		}
		if err == nil {
			counts[i].Count = cnt
			total += cnt
		}
	}

	return counts, total, nil
}

// TotalMediaCount counts documents across all configured media shards.
func (r *MediaRepository) TotalMediaCount(ctx context.Context) (int64, error) {
	_, total, err := r.GetShardCounts(ctx)
	return total, err
}

// StreamShard streams all MediaDoc records from a specific shard index using high-performance batching and field projections.
func (r *MediaRepository) StreamShard(ctx context.Context, shardIndex int, callback func(doc MediaDoc) error) (int64, error) {
	if r == nil || shardIndex < 0 || shardIndex >= len(r.shards) {
		return 0, fmt.Errorf("invalid shard index: %d", shardIndex)
	}

	shard := r.shards[shardIndex]
	if shard.Col == nil {
		return 0, fmt.Errorf("shard %s collection is nil", shard.Label)
	}

	// Optimize wire transfer by projecting only required fields and requesting 10,000 docs per network batch
	opts := options.Find().
		SetBatchSize(10000).
		SetNoCursorTimeout(true). // Prevent cursor timeout during heavy bulk migration across millions of records
		SetProjection(bson.M{
			"_id":       1,
			"file_ref":  1,
			"file_name": 1,
			"file_size": 1,
			"size":      1,
			"file_type": 1,
			"mime_type": 1,
			"caption":   1,
			"date":      1,
		})

	var cursor *mongo.Cursor
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		cursor, err = shard.Col.Find(ctx, bson.M{}, opts)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return 0, fmt.Errorf("failed to open cursor on shard %s (%s): %w", shard.Label, shard.Col.Name(), err)
		}
		time.Sleep(time.Duration(attempt) * 1 * time.Second)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to open cursor on shard %s (%s): %w", shard.Label, shard.Col.Name(), err)
	}
	defer cursor.Close(context.Background())

	var streamed int64
	for cursor.Next(ctx) {
		var doc MediaDoc
		if err := cursor.Decode(&doc); err != nil {
			// Skip malformed document rather than failing the entire migration
			continue
		}

		if doc.FileID == "" {
			continue
		}

		if err := callback(doc); err != nil {
			return streamed, err
		}
		streamed++
	}

	if err := cursor.Err(); err != nil {
		return streamed, fmt.Errorf("cursor error on shard %s: %w", shard.Label, err)
	}

	return streamed, nil
}

// StreamAllDocuments streams all MediaDoc records across all media collections in order to
// feed the SQLite synchronization engine.
func (r *MediaRepository) StreamAllDocuments(ctx context.Context, callback func(doc MediaDoc) error) (int64, error) {
	if r == nil || len(r.shards) == 0 {
		return 0, nil
	}

	var totalStreamed int64
	for i := range r.shards {
		streamed, err := r.StreamShard(ctx, i, callback)
		totalStreamed += streamed
		if err != nil {
			return totalStreamed, err
		}
	}

	return totalStreamed, nil
}

// GetFileDetails fetches details of a media document by file_id from authoritative MongoDB across shards.
func (r *MediaRepository) GetFileDetails(ctx context.Context, fileID string) (*MediaDoc, error) {
	if r == nil || len(r.shards) == 0 || fileID == "" {
		return nil, nil
	}

	for _, shard := range r.shards {
		if shard.Col == nil {
			continue
		}
		var doc MediaDoc
		err := shard.Col.FindOne(ctx, bson.M{"_id": fileID}).Decode(&doc)
		if err == nil {
			return &doc, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}

	return nil, nil
}
