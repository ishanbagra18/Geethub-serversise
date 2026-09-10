package models

import "time"

type MoodResponse struct {
	Mood               string    `json:"mood"`
	Emoji              string    `json:"emoji"`
	Summary            string    `json:"summary"`
	Confidence         int       `json:"confidence"`
	RecommendedVibes   []string  `json:"recommended_vibes"`
	RecentSongsCount   int       `json:"recent_songs_count"`
	AnalyzedAt         time.Time `json:"analyzed_at"`
}
