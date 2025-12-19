package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type PlaylistType string

const (
	PlaylistTypeUser   PlaylistType = "user"
	PlaylistTypeSystem PlaylistType = "system"
)

type Playlist struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name        *string            `bson:"name" json:"name" validate:"required,min=2,max=100"`
	Description *string            `bson:"description,omitempty" json:"description,omitempty" validate:"required,min=2,max=500"`
	CoverImage  *string            `bson:"cover_image,omitempty" json:"cover_image,omitempty"`
	CreatorID   *string            `bson:"creator_id,omitempty" json:"creator_id,omitempty"`
	Type        PlaylistType       `bson:"type" json:"type" validate:"required"`
	Tags        []string           `bson:"tags,omitempty" json:"tags,omitempty"`
	SongIDs     []string           `bson:"song_ids,omitempty" json:"song_ids,omitempty"`
	IsPublic    bool               `bson:"is_public" json:"is_public"`
	IsSeeded    bool               `bson:"is_seeded,omitempty" json:"is_seeded,omitempty"`
	PlayCount   int64              `bson:"play_count,omitempty" json:"play_count,omitempty"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}