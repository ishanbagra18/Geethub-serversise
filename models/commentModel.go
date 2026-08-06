package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Comment struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	SongID    string             `bson:"song_id" json:"song_id" validate:"required"`
	UserID    string             `bson:"user_id" json:"user_id"`
	UserName  string             `bson:"user_name" json:"user_name"`
	UserEmoji string             `bson:"user_emoji,omitempty" json:"user_emoji,omitempty"`
	Text      string             `bson:"text" json:"text" validate:"required,min=1,max=500"`
	Likes     []string           `bson:"likes,omitempty" json:"likes,omitempty"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}
