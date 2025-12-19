package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Song struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Title       *string            `bson:"title" json:"title" validate:"required,min=2,max=100"`
	Artist      *string            `bson:"artist" json:"artist" validate:"required,min=2,max=100"`
	Album       *string            `bson:"album" json:"album"`
	Info        *string            `bson:"info" json:"info"`
	Genre       *string            `bson:"genre" json:"genre"`
	Language    *string            `bson:"language" json:"language"`                       // New: Language of the song
	FileURL     *string            `bson:"file_url" json:"file_url" validate:"required"`   // Cloudinary song file URL
	ImageURL    *string            `bson:"image_url,omitempty" json:"image_url,omitempty"` // Optional Cloudinary image URL
	UploadedBy  *string            `bson:"uploaded_by" json:"uploaded_by" validate:"required"`
	Likes       []string           `bson:"likes,omitempty" json:"likes,omitempty"` // User IDs who liked
	Saves       []string           `bson:"saves,omitempty" json:"saves,omitempty"` // User IDs who saved
	CreatedAt   *time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt   *time.Time         `bson:"updated_at" json:"updated_at"`
	SongID      string             `bson:"song_id" json:"song_id"`
	ReleaseDate *time.Time         `bson:"release_date,omitempty" json:"release_date,omitempty"`
}
