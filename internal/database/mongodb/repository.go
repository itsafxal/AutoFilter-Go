package mongodb

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
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

// DeleteUser removes a deactivated or permanently invalid user document.
func (r *UserRepository) DeleteUser(ctx context.Context, id int64) error {
	if r == nil || r.col == nil || id <= 0 {
		return errors.New("invalid repository or user id")
	}
	_, err := r.col.DeleteOne(ctx, bson.M{"id": id})
	return err
}

// ActiveUsersCount returns the count of active (non-banned) users.
func (r *UserRepository) ActiveUsersCount(ctx context.Context) (int64, error) {
	if r == nil || r.col == nil {
		return 0, nil
	}
	filter := bson.M{
		"ban_status.is_banned": bson.M{"$ne": true},
	}
	return r.col.CountDocuments(ctx, filter)
}

// GetActiveUsersCursor returns a cursor streaming active (non-banned) users with ID projection.
func (r *UserRepository) GetActiveUsersCursor(ctx context.Context, skip int64) (*mongo.Cursor, error) {
	if r == nil || r.col == nil {
		return nil, errors.New("repository uninitialized")
	}
	findOpts := options.Find().SetProjection(bson.M{"id": 1}).SetSort(bson.M{"_id": 1})
	if skip > 0 {
		findOpts.SetSkip(skip)
	}
	filter := bson.M{
		"ban_status.is_banned": bson.M{"$ne": true},
	}
	return r.col.Find(ctx, filter, findOpts)
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

// DeleteGroup removes an inaccessible, deleted or disabled group document.
func (r *GroupRepository) DeleteGroup(ctx context.Context, id int64) error {
	if r == nil || r.col == nil || id == 0 {
		return errors.New("invalid repository or group id")
	}
	_, err := r.col.DeleteOne(ctx, bson.M{"id": id})
	return err
}

// DisableGroup flags a group as disabled with an optional reason.
func (r *GroupRepository) DisableGroup(ctx context.Context, id int64, reason string) error {
	if r == nil || r.col == nil || id == 0 {
		return errors.New("invalid repository or group id")
	}
	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"id": id},
		bson.M{
			"$set": bson.M{
				"chat_status.is_disabled": true,
				"chat_status.reason":      reason,
			},
		},
	)
	return err
}

// EnableGroup re-enables a previously disabled group.
func (r *GroupRepository) EnableGroup(ctx context.Context, id int64) error {
	if r == nil || r.col == nil || id == 0 {
		return errors.New("invalid repository or group id")
	}
	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"id": id},
		bson.M{
			"$set": bson.M{
				"chat_status.is_disabled": false,
				"chat_status.reason":      "",
			},
		},
	)
	return err
}

// ActiveGroupsCount returns total active (non-disabled) group chats.
func (r *GroupRepository) ActiveGroupsCount(ctx context.Context) (int64, error) {
	if r == nil || r.col == nil {
		return 0, nil
	}
	filter := bson.M{
		"chat_status.is_disabled": bson.M{"$ne": true},
	}
	return r.col.CountDocuments(ctx, filter)
}

// GetActiveGroupsCursor returns a cursor streaming active (non-disabled) groups with ID projection.
func (r *GroupRepository) GetActiveGroupsCursor(ctx context.Context, skip int64) (*mongo.Cursor, error) {
	if r == nil || r.col == nil {
		return nil, errors.New("repository uninitialized")
	}
	findOpts := options.Find().SetProjection(bson.M{"id": 1}).SetSort(bson.M{"_id": 1})
	if skip > 0 {
		findOpts.SetSkip(skip)
	}
	filter := bson.M{
		"chat_status.is_disabled": bson.M{"$ne": true},
	}
	return r.col.Find(ctx, filter, findOpts)
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
	shards        []MediaShard
	counter       uint64
	blockedMu     sync.RWMutex
	blockedShards map[string]time.Time
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
		shards:        shards,
		blockedShards: make(map[string]time.Time),
	}
}

// NewShardedMediaRepository creates a new MediaRepository from explicitly labeled shards.
func NewShardedMediaRepository(shards ...MediaShard) *MediaRepository {
	return &MediaRepository{
		shards:        shards,
		blockedShards: make(map[string]time.Time),
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

	// Optimize wire transfer without projection overhead (matching TGxCyberBot cursor streaming)
	// and requesting 10,000 documents per wire batch
	opts := options.Find().
		SetBatchSize(10000)

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

// StreamShardBatches streams documents in slices of batchSize to eliminate per-document channel overhead.
func (r *MediaRepository) StreamShardBatches(ctx context.Context, shardIndex int, batchSize int, callback func(batch []MediaDoc) error) (int64, error) {
	if r == nil || shardIndex < 0 || shardIndex >= len(r.shards) {
		return 0, fmt.Errorf("invalid shard index: %d", shardIndex)
	}

	shard := r.shards[shardIndex]
	if shard.Col == nil {
		return 0, fmt.Errorf("shard %s collection is nil", shard.Label)
	}

	if batchSize <= 0 {
		batchSize = 10000
	}

	opts := options.Find().
		SetBatchSize(int32(batchSize))

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
	batch := make([]MediaDoc, 0, batchSize)

	for cursor.Next(ctx) {
		var doc MediaDoc
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		if doc.FileID == "" {
			continue
		}
		batch = append(batch, doc)
		if len(batch) >= batchSize {
			streamed += int64(len(batch))
			if err := callback(batch); err != nil {
				return streamed, err
			}
			batch = make([]MediaDoc, 0, batchSize)
		}
	}

	if len(batch) > 0 {
		streamed += int64(len(batch))
		if err := callback(batch); err != nil {
			return streamed, err
		}
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

// Search executes a fast concurrent search across all media shards for documents matching the query.
func (r *MediaRepository) Search(ctx context.Context, query string, limit int) ([]MediaDoc, error) {
	if r == nil || len(r.shards) == 0 {
		return nil, nil
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, nil
	}

	if limit <= 0 {
		limit = 100
	}

	words := strings.Fields(trimmed)
	if len(words) == 0 {
		return nil, nil
	}

	var andConds []bson.M
	for _, w := range words {
		cleaned := strings.TrimSpace(w)
		if cleaned == "" {
			continue
		}
		andConds = append(andConds, bson.M{
			"file_name": bson.M{
				"$regex":   regexp.QuoteMeta(cleaned),
				"$options": "i",
			},
		})
	}

	if len(andConds) == 0 {
		return nil, nil
	}

	var filter bson.M
	if len(andConds) == 1 {
		filter = andConds[0]
	} else {
		filter = bson.M{"$and": andConds}
	}

	opts := options.Find().
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "date", Value: -1}}).
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

	searchCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	var (
		mu      sync.Mutex
		results []MediaDoc
		seen    = make(map[string]struct{})
		wg      sync.WaitGroup
	)

	for _, shard := range r.shards {
		if shard.Col == nil {
			continue
		}
		wg.Add(1)
		go func(sh MediaShard) {
			defer wg.Done()
			cur, err := sh.Col.Find(searchCtx, filter, opts)
			if err != nil {
				return
			}
			defer cur.Close(context.Background())

			var docs []MediaDoc
			for cur.Next(searchCtx) {
				var d MediaDoc
				if err := cur.Decode(&d); err == nil && d.FileID != "" {
					docs = append(docs, d)
				}
			}

			if len(docs) > 0 {
				mu.Lock()
				for _, d := range docs {
					if _, exists := seen[d.FileID]; !exists {
						seen[d.FileID] = struct{}{}
						results = append(results, d)
					}
				}
				mu.Unlock()
			}
		}(shard)
	}

	wg.Wait()
	return results, nil
}

// SearchCandidateTitles fetches candidate file names from MongoDB shards for spell check suggestions.
func (r *MediaRepository) SearchCandidateTitles(ctx context.Context, prefix string, limit int) ([]string, error) {
	if r == nil || len(r.shards) == 0 {
		return nil, nil
	}
	trimmed := strings.TrimSpace(prefix)
	if len(trimmed) < 2 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 30
	}

	filter := bson.M{
		"file_name": bson.M{
			"$regex":   regexp.QuoteMeta(trimmed),
			"$options": "i",
		},
	}

	opts := options.Find().
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "date", Value: -1}}).
		SetProjection(bson.M{
			"file_name": 1,
		})

	searchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var (
		mu     sync.Mutex
		titles []string
		seen   = make(map[string]struct{})
		wg     sync.WaitGroup
	)

	for _, shard := range r.shards {
		if shard.Col == nil {
			continue
		}
		wg.Add(1)
		go func(sh MediaShard) {
			defer wg.Done()
			cur, err := sh.Col.Find(searchCtx, filter, opts)
			if err != nil {
				return
			}
			defer cur.Close(context.Background())

			var shardTitles []string
			for cur.Next(searchCtx) {
				var doc struct {
					FileName string `bson:"file_name"`
				}
				if err := cur.Decode(&doc); err == nil && doc.FileName != "" {
					shardTitles = append(shardTitles, doc.FileName)
				}
			}

			if len(shardTitles) > 0 {
				mu.Lock()
				for _, t := range shardTitles {
					if _, exists := seen[t]; !exists {
						seen[t] = struct{}{}
						titles = append(titles, t)
					}
				}
				mu.Unlock()
			}
		}(shard)
	}

	wg.Wait()
	return titles, nil
}

// CheckFileExists checks if a document with fileID exists in any media shard.
func (r *MediaRepository) CheckFileExists(ctx context.Context, fileID string) (bool, error) {
	if r == nil || len(r.shards) == 0 || fileID == "" {
		return false, nil
	}

	for _, shard := range r.shards {
		if shard.Col == nil {
			continue
		}
		count, err := shard.Col.CountDocuments(ctx, bson.M{"_id": fileID}, options.Count().SetLimit(1))
		if err != nil {
			return false, fmt.Errorf("failed to check file existence in shard %s: %w", shard.Label, err)
		}
		if count > 0 {
			return true, nil
		}
	}

	return false, nil
}

// BulkCheckFiles concurrently checks if the given file IDs exist in any media shard.
// Returns a map indicating existence (true = exists, false = new).
func (r *MediaRepository) BulkCheckFiles(ctx context.Context, fileIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(fileIDs))
	for _, id := range fileIDs {
		result[id] = false
	}
	if r == nil || len(r.shards) == 0 || len(fileIDs) == 0 {
		return result, nil
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	filter := bson.M{"_id": bson.M{"$in": fileIDs}}
	opts := options.Find().SetProjection(bson.M{"_id": 1})

	for _, shard := range r.shards {
		if shard.Col == nil {
			continue
		}
		wg.Add(1)
		go func(sh MediaShard) {
			defer wg.Done()
			cur, err := sh.Col.Find(ctx, filter, opts)
			if err != nil {
				return
			}
			defer cur.Close(context.Background())

			for cur.Next(ctx) {
				var doc struct {
					ID string `bson:"_id"`
				}
				if err := cur.Decode(&doc); err == nil && doc.ID != "" {
					mu.Lock()
					result[doc.ID] = true
					mu.Unlock()
				}
			}
		}(shard)
	}

	wg.Wait()
	return result, nil
}

// FindByNameAndSize finds a media document across all shards matching original file name and size in bytes.
func (r *MediaRepository) FindByNameAndSize(ctx context.Context, fileName string, fileSize int64) (*MediaDoc, error) {
	if r == nil || len(r.shards) == 0 || fileName == "" || fileSize <= 0 {
		return nil, nil
	}

	filter := bson.M{"file_name": fileName, "file_size": fileSize}
	opts := options.FindOne()

	for _, shard := range r.shards {
		if shard.Col == nil {
			continue
		}
		var doc MediaDoc
		err := shard.Col.FindOne(ctx, filter, opts).Decode(&doc)
		if err == nil {
			return &doc, nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, err
		}
	}

	return nil, nil
}

// BulkCheckFilesByMetadata checks if media documents already exist across shards by matching either _id OR (file_name, file_size).
// Returns a map keyed by FileID with existence status (true = exists, false = new).
func (r *MediaRepository) BulkCheckFilesByMetadata(ctx context.Context, docs []MediaDoc) (map[string]bool, error) {
	result := make(map[string]bool, len(docs))
	for _, d := range docs {
		result[d.FileID] = false
	}
	if r == nil || len(r.shards) == 0 || len(docs) == 0 {
		return result, nil
	}

	// 1. First pass: check by _id
	fileIDs := make([]string, 0, len(docs))
	for _, d := range docs {
		if d.FileID != "" {
			fileIDs = append(fileIDs, d.FileID)
		}
	}
	if len(fileIDs) > 0 {
		idMap, err := r.BulkCheckFiles(ctx, fileIDs)
		if err == nil {
			for id, exists := range idMap {
				if exists {
					result[id] = true
				}
			}
		}
	}

	// 2. Second pass: for remaining unfound docs, check by (file_name, file_size)
	var remaining []MediaDoc
	for _, d := range docs {
		if !result[d.FileID] && d.FileName != "" && d.FileSize > 0 {
			remaining = append(remaining, d)
		}
	}

	if len(remaining) == 0 {
		return result, nil
	}

	const batchSize = 50
	for i := 0; i < len(remaining); i += batchSize {
		end := i + batchSize
		if end > len(remaining) {
			end = len(remaining)
		}
		chunk := remaining[i:end]

		orFilters := make([]bson.M, 0, len(chunk))
		for _, d := range chunk {
			orFilters = append(orFilters, bson.M{
				"file_name": d.FileName,
				"file_size": d.FileSize,
			})
		}

		filter := bson.M{"$or": orFilters}
		opts := options.Find().SetProjection(bson.M{"file_name": 1, "file_size": 1})

		for _, shard := range r.shards {
			if shard.Col == nil {
				continue
			}
			cur, err := shard.Col.Find(ctx, filter, opts)
			if err != nil {
				continue
			}

			for cur.Next(ctx) {
				var found struct {
					FileName string `bson:"file_name"`
					FileSize int64  `bson:"file_size"`
				}
				if cur.Decode(&found) == nil {
					for _, d := range chunk {
						if d.FileName == found.FileName && d.FileSize == found.FileSize {
							result[d.FileID] = true
						}
					}
				}
			}
			cur.Close(context.Background())
		}
	}

	return result, nil
}

// IsQuotaOrWriteBlockedError reports whether an error indicates MongoDB Atlas or cluster
// storage quota exhaustion or blocked writes.
func IsQuotaOrWriteBlockedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "space quota") ||
		strings.Contains(msg, "writes are blocked") ||
		strings.Contains(msg, "quota exceeded") ||
		strings.Contains(msg, "storage quota")
}

// MarkShardBlocked marks a shard as write-blocked due to quota limits.
func (r *MediaRepository) MarkShardBlocked(label string) {
	if r == nil || label == "" {
		return
	}
	r.blockedMu.Lock()
	defer r.blockedMu.Unlock()
	if r.blockedShards == nil {
		r.blockedShards = make(map[string]time.Time)
	}
	r.blockedShards[label] = time.Now()
}

// IsShardBlocked checks if a shard is currently flagged as write-blocked.
func (r *MediaRepository) IsShardBlocked(label string) bool {
	if r == nil || label == "" {
		return false
	}
	r.blockedMu.RLock()
	defer r.blockedMu.RUnlock()
	if r.blockedShards == nil {
		return false
	}
	blockedAt, exists := r.blockedShards[label]
	if !exists {
		return false
	}
	// Re-attempt writes after 15 minutes in case storage was cleared or cluster tier was upgraded
	if time.Since(blockedAt) > 15*time.Minute {
		return false
	}
	return true
}

// ClearShardBlocked removes the blocked status for a shard upon a successful write.
func (r *MediaRepository) ClearShardBlocked(label string) {
	if r == nil || label == "" {
		return
	}
	r.blockedMu.Lock()
	defer r.blockedMu.Unlock()
	if r.blockedShards != nil {
		delete(r.blockedShards, label)
	}
}

// SaveMediaWithFailover attempts to save or upsert a MediaDoc to the preferred shard.
// If the preferred shard is blocked or returns a quota/write-blocked error, it automatically
// fails over to any remaining healthy shards.
func (r *MediaRepository) SaveMediaWithFailover(ctx context.Context, doc MediaDoc, preferredIdx int) (string, error) {
	if r == nil || len(r.shards) == 0 {
		return "", errors.New("no media shards configured")
	}
	if doc.FileID == "" {
		return "", errors.New("cannot save media document with empty file_id")
	}
	if doc.Date.IsZero() {
		doc.Date = time.Now().UTC()
	}

	n := len(r.shards)
	startIdx := preferredIdx % n
	if startIdx < 0 {
		startIdx = 0
	}

	// First prioritize unblocked shards starting from startIdx
	attemptIndices := make([]int, 0, n)
	for i := 0; i < n; i++ {
		idx := (startIdx + i) % n
		if !r.IsShardBlocked(r.shards[idx].Label) {
			attemptIndices = append(attemptIndices, idx)
		}
	}
	// If all shards were marked blocked, still attempt all in case quota was cleared
	if len(attemptIndices) == 0 {
		for i := 0; i < n; i++ {
			attemptIndices = append(attemptIndices, (startIdx+i)%n)
		}
	}

	var lastErr error
	opts := options.Replace().SetUpsert(true)

	for _, idx := range attemptIndices {
		shard := r.shards[idx]
		if shard.Col == nil {
			continue
		}

		_, err := shard.Col.ReplaceOne(ctx, bson.M{"_id": doc.FileID}, doc, opts)
		if err == nil {
			r.ClearShardBlocked(shard.Label)
			return shard.Label, nil
		}

		lastErr = err
		if IsQuotaOrWriteBlockedError(err) {
			r.MarkShardBlocked(shard.Label)
			continue
		}

		// Fatal or context canceled error
		if ctx.Err() != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("failed to save media %s across available shards (last error on %s): %w",
		doc.FileID, r.shards[attemptIndices[len(attemptIndices)-1]].Label, lastErr)
}

// SaveMedia saves or upserts a MediaDoc document to the specified shard index.
// If the target shard is blocked or over quota, it automatically fails over to a healthy shard.
func (r *MediaRepository) SaveMedia(ctx context.Context, doc MediaDoc, shardIndex int) error {
	_, err := r.SaveMediaWithFailover(ctx, doc, shardIndex)
	return err
}

// SaveMediaAlternating atomically alternates saves between healthy media shards (DB2, DB3, etc.)
// If a shard is full or returns a quota error, it automatically fails over to the next healthy shard.
// Returns the label of the shard where the document was saved.
func (r *MediaRepository) SaveMediaAlternating(ctx context.Context, doc MediaDoc) (string, error) {
	if r == nil || len(r.shards) == 0 {
		return "", errors.New("no media shards configured")
	}

	idx := 0
	if len(r.shards) > 1 {
		idx = int((atomic.AddUint64(&r.counter, 1) - 1) % uint64(len(r.shards)))
	}
	return r.SaveMediaWithFailover(ctx, doc, idx)
}

// BulkSaveMedia performs high-performance bulk insertion of MediaDocs to the specified shard index.
// If the preferred shard is blocked or returns a quota error, it automatically fails over to a healthy shard.
// Duplicate key errors are counted and skipped cleanly.
func (r *MediaRepository) BulkSaveMedia(ctx context.Context, docs []MediaDoc, shardIndex int) (saved int, duplicates int, err error) {
	if r == nil || len(r.shards) == 0 || len(docs) == 0 {
		return 0, 0, nil
	}

	n := len(r.shards)
	startIdx := shardIndex % n
	if startIdx < 0 {
		startIdx = 0
	}

	models := make([]mongo.WriteModel, 0, len(docs))
	now := time.Now().UTC()
	for _, doc := range docs {
		if doc.FileID == "" {
			continue
		}
		if doc.Date.IsZero() {
			doc.Date = now
		}
		models = append(models, mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": doc.FileID}).
			SetReplacement(doc).
			SetUpsert(true))
	}
	if len(models) == 0 {
		return 0, 0, nil
	}

	opts := options.BulkWrite().SetOrdered(false)

	attemptIndices := make([]int, 0, n)
	for i := 0; i < n; i++ {
		idx := (startIdx + i) % n
		if !r.IsShardBlocked(r.shards[idx].Label) {
			attemptIndices = append(attemptIndices, idx)
		}
	}
	if len(attemptIndices) == 0 {
		for i := 0; i < n; i++ {
			attemptIndices = append(attemptIndices, (startIdx+i)%n)
		}
	}

	var lastErr error
	for _, idx := range attemptIndices {
		shard := r.shards[idx]
		if shard.Col == nil {
			continue
		}

		res, bErr := shard.Col.BulkWrite(ctx, models, opts)
		if res != nil {
			saved = int(res.InsertedCount + res.UpsertedCount + res.ModifiedCount + res.MatchedCount)
		}

		if bErr != nil {
			var bwe mongo.BulkWriteException
			if errors.As(bErr, &bwe) {
				hasOtherError := false
				for _, we := range bwe.WriteErrors {
					if we.Code == 11000 {
						duplicates++
					} else {
						hasOtherError = true
					}
				}
				if !hasOtherError {
					r.ClearShardBlocked(shard.Label)
					return saved, duplicates, nil
				}
			}

			if IsQuotaOrWriteBlockedError(bErr) {
				r.MarkShardBlocked(shard.Label)
				lastErr = bErr
				continue
			}

			return saved, duplicates, bErr
		}

		r.ClearShardBlocked(shard.Label)
		return saved, duplicates, nil
	}

	return saved, duplicates, lastErr
}

// BulkSaveMediaAlternating saves docs in alternating batches (e.g. 100 to DB2, 100 to DB3).
// If any shard is blocked or full, batches automatically failover to healthy shards.
func (r *MediaRepository) BulkSaveMediaAlternating(ctx context.Context, docs []MediaDoc, chunkSize int) (saved int, duplicates int, err error) {
	if r == nil || len(r.shards) == 0 || len(docs) == 0 {
		return 0, 0, nil
	}
	if chunkSize <= 0 {
		chunkSize = 100
	}

	numShards := len(r.shards)
	for i := 0; i < len(docs); i += chunkSize {
		end := i + chunkSize
		if end > len(docs) {
			end = len(docs)
		}
		chunk := docs[i:end]
		shardIdx := (i / chunkSize) % numShards

		s, d, cErr := r.BulkSaveMedia(ctx, chunk, shardIdx)
		saved += s
		duplicates += d
		if cErr != nil {
			return saved, duplicates, cErr
		}
	}

	return saved, duplicates, nil
}
