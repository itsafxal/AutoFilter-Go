package fsub

import "time"

// Loadout represents the Force Subscribe configuration stored in MongoDB collection "admins".
type Loadout struct {
	ID                int64 `bson:"id" json:"id"`
	Channel1          int64 `bson:"channel1,omitempty" json:"channel1"`
	Channel2          int64 `bson:"channel2,omitempty" json:"channel2"`
	Channel3          int64 `bson:"channel3,omitempty" json:"channel3"`
	Channel1_2        int64 `bson:"channel1_2,omitempty" json:"channel1_2"`
	Channel2_2        int64 `bson:"channel2_2,omitempty" json:"channel2_2"`
	Channel3_2        int64 `bson:"channel3_2,omitempty" json:"channel3_2"`
	Channel1Threshold int   `bson:"channel1_threshold,omitempty" json:"channel1_threshold"`
	Channel2Threshold int   `bson:"channel2_threshold,omitempty" json:"channel2_threshold"`
	Channel3Threshold int   `bson:"channel3_threshold,omitempty" json:"channel3_threshold"`
}

// JoinRequestDoc represents a recorded join request in collections "reqone", "reqtwo", "reqthree".
type JoinRequestDoc struct {
	UserID    int64     `bson:"user_id,omitempty" json:"user_id"`
	ID        int64     `bson:"id,omitempty" json:"id"`
	CreatedAt time.Time `bson:"created_at,omitempty" json:"created_at"`
}
