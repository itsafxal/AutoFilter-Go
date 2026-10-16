package mongodb

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestUser_BSONMapping(t *testing.T) {
	user := User{
		ID:   12345678,
		Name: "Alice",
		BanStatus: BanStatus{
			IsBanned:  true,
			BanReason: "Spamming",
		},
	}

	data, err := bson.Marshal(user)
	if err != nil {
		t.Fatalf("failed to marshal User: %v", err)
	}

	var decoded User
	if err := bson.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal User: %v", err)
	}

	if decoded.ID != 12345678 {
		t.Errorf("expected ID 12345678, got %d", decoded.ID)
	}
	if decoded.Name != "Alice" {
		t.Errorf("expected Name Alice, got %s", decoded.Name)
	}
	if !decoded.BanStatus.IsBanned || decoded.BanStatus.BanReason != "Spamming" {
		t.Errorf("unexpected BanStatus: %+v", decoded.BanStatus)
	}
}

func TestGroup_BSONMapping(t *testing.T) {
	group := Group{
		ID:    -1001234567890,
		Title: "Movie Group",
		ChatStatus: ChatStatus{
			IsDisabled: true,
			Reason:     "Maintenance",
		},
	}

	data, err := bson.Marshal(group)
	if err != nil {
		t.Fatalf("failed to marshal Group: %v", err)
	}

	var decoded Group
	if err := bson.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal Group: %v", err)
	}

	if decoded.ID != -1001234567890 {
		t.Errorf("expected ID -1001234567890, got %d", decoded.ID)
	}
	if decoded.Title != "Movie Group" {
		t.Errorf("expected Title Movie Group, got %s", decoded.Title)
	}
	if !decoded.ChatStatus.IsDisabled || decoded.ChatStatus.Reason != "Maintenance" {
		t.Errorf("unexpected ChatStatus: %+v", decoded.ChatStatus)
	}
}

func TestMediaDoc_BSONMapping(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	doc := MediaDoc{
		FileID:   "BAACAgUAAxkBAAI...",
		FileRef:  "ref_1234",
		FileName: "[S01 EP02] Breaking Bad.mkv",
		FileSize: 1048576000,
		Size:     "1000 MB",
		FileType: "video",
		MimeType: "video/x-matroska",
		Caption:  "Breaking Bad S01E02",
		Date:     now,
	}

	data, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal MediaDoc: %v", err)
	}

	var decoded MediaDoc
	if err := bson.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal MediaDoc: %v", err)
	}

	if decoded.FileID != doc.FileID {
		t.Errorf("expected _id %s, got %s", doc.FileID, decoded.FileID)
	}
	if decoded.FileName != doc.FileName {
		t.Errorf("expected file_name %s, got %s", doc.FileName, decoded.FileName)
	}
	if decoded.FileSize != doc.FileSize {
		t.Errorf("expected file_size %d, got %d", doc.FileSize, decoded.FileSize)
	}
}
