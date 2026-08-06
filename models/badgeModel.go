package models

import "time"

type Badge struct {
	ID          string     `bson:"id" json:"id"`
	Name        string     `bson:"name" json:"name"`
	Description string     `bson:"description" json:"description"`
	Icon        string     `bson:"icon" json:"icon"`
	Color       string     `bson:"color" json:"color"`
	Earned      bool       `bson:"earned" json:"earned"`
	EarnedAt    *time.Time `bson:"earned_at,omitempty" json:"earned_at,omitempty"`
}
