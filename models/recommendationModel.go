//Recommendation engine added

package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserInteraction records an explicit or implicit engagement event
type UserInteraction struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	UserID          string             `bson:"user_id" json:"user_id"`
	SongID          string             `bson:"song_id" json:"song_id"`
	Action          string             `bson:"action" json:"action"` // "play", "like", "save", "skip", "playlist_add"
	DurationSeconds float64            `bson:"duration_seconds,omitempty" json:"duration_seconds,omitempty"`
	CompletionRatio float64            `bson:"completion_ratio,omitempty" json:"completion_ratio,omitempty"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
}

// RecommendationExplanation describes why a particular song was recommended
type RecommendationExplanation struct {
	Reason          string  `json:"reason"`            // e.g. "Similar to your recent favorites like Kesariya"
	Type            string  `json:"type"`              // "CONTENT_SIMILARITY", "COLLABORATIVE_FILTERING", "MOOD_ALIGNMENT", "ARTIST_AFFINITY", "COLD_START_POPULAR", "PLAYLIST_AFFINITY"
	SourceSongTitle string  `json:"source_song_title,omitempty"`
	SourceArtist    string  `json:"source_artist,omitempty"`
	ConfidenceScore float64 `json:"confidence_score"`  // 0.0 to 1.0
}

// RecommendedSong bundles a song with its recommendation score and explainable reason
type RecommendedSong struct {
	Song                 Song                      `json:"song"`
	Score                float64                   `json:"score"`
	MatchPct             int                       `json:"match_pct"`
	Explanation          RecommendationExplanation `json:"explanation"`
	AlgorithmicBreakdown map[string]float64        `json:"algorithmic_breakdown,omitempty"`
}

// RecommendationResponse is returned by GET /api/recommendations
type RecommendationResponse struct {
	Recommendations []RecommendedSong  `json:"recommendations"`
	Strategy        string             `json:"strategy"` // "hybrid", "cold_start_new_user", "content_based", etc.
	IsColdStart     bool               `json:"is_cold_start"`
	UserTopGenres   map[string]float64 `json:"user_top_genres,omitempty"`
	TotalCandidates int                `json:"total_candidates"`
	GeneratedAt     time.Time          `json:"generated_at"`
}

// EvaluationResult holds offline evaluation metrics for a specific K threshold
type EvaluationResult struct {
	K               int     `json:"k"`
	PrecisionAtK    float64 `json:"precision_at_k"`
	RecallAtK       float64 `json:"recall_at_k"`
	HitRateAtK      float64 `json:"hit_rate_at_k"`
	NDCGAtK         float64 `json:"ndcg_at_k"`
	MRR             float64 `json:"mrr"`
	CatalogCoverage float64 `json:"catalog_coverage"`
	DiversityScore  float64 `json:"diversity_score"`
	UserCount       int     `json:"user_count"`
	TestSetSize     int     `json:"test_set_size"`
	ModelName       string  `json:"model_name"`
}

// BenchmarkReport compares multiple recommendation algorithms across evaluation metrics
type BenchmarkReport struct {
	Models      map[string][]EvaluationResult `json:"models"` // "Hybrid", "ContentBased", "CollaborativeFiltering", "PopularityBaseline"
	EvaluatedAt time.Time                     `json:"evaluated_at"`
	Summary     string                        `json:"summary"`
}
