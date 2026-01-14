package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)



type Message struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	SenderID   *string            `bson:"sender_id" json:"sender_id" validate:"required"`
	ReceiverID  *string           `bson:"receiver_id" json:"receiver_id" validate:"required"`
	MessageText  *string		   `bson:"message_text" json:"message_text"`
	Timestamp  *time.Time         `bson:"timestamp" json:"timestamp"`
	PhotoURL   *string            `bson:"photo_url,omitempty" json:"photo_url,omitempty"`
}