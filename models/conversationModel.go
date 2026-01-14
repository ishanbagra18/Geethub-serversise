package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"

)


type Conversation struct {
	ID            primitive.ObjectID   `bson:"_id,omitempty" json:"id,omitempty"`
	ParticipantIDs []string             `bson:"participant_ids" json:"participant_ids" validate:"required,min=2"`
	Messages      []primitive.ObjectID `bson:"messages,omitempty" json:"messages,omitempty"`
	CreatedAt     *time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt     *time.Time           `bson:"updated_at" json:"updated_at"`
}



