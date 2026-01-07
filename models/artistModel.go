package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Artist struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name          *string            `bson:"name" json:"name" validate:"required,min=2,max=100"`
	Bio           *string            `bson:"bio" json:"bio"`
	Genre         []string           `bson:"genre" json:"genre"`
	ImageURL      *string            `bson:"image_url,omitempty" json:"image_url,omitempty"`
	FollowerCount int                `bson:"follower_count" json:"follower_count,omitempty"`
	Followers     []string           `bson:"followers,omitempty" json:"followers,omitempty"`
	Created_at    *time.Time         `bson:"created_at" json:"created_at,omitempty"`
	Updated_at    *time.Time         `bson:"updated_at" json:"updated_at,omitempty"`
	Artist_id     string             `bson:"artist_id" json:"artist_id,omitempty"`
	Verified      bool               `bson:"verified" json:"verified,omitempty"`
	SocialLinks   *SocialLinks       `bson:"social_links,omitempty" json:"social_links,omitempty"`
}

type SocialLinks struct {
	Instagram *string `bson:"instagram,omitempty" json:"instagram,omitempty"`
	Twitter   *string `bson:"twitter,omitempty" json:"twitter,omitempty"`
	Facebook  *string `bson:"facebook,omitempty" json:"facebook,omitempty"`
	Website   *string `bson:"website,omitempty" json:"website,omitempty"`
}
