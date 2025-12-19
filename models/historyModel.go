package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// models/history.go

type History struct {
    ID       primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    UserID   string             `bson:"user_id" json:"user_id"`
    SongID   string             `bson:"song_id" json:"song_id"`
    PlayedAt time.Time          `bson:"played_at" json:"played_at"`
}
