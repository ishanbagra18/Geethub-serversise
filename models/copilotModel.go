package models

import "time"

// CopilotQueryRequest represents the incoming user natural language request
type CopilotQueryRequest struct {
	Query          string `json:"query" binding:"required"`
	Action         string `json:"action,omitempty"`          // Optional override action
	TargetDuration int    `json:"target_duration,omitempty"` // In minutes (optional)
	UserID         string `json:"user_id,omitempty"`
}

// ExtractedIntent holds the LLM-extracted structured parameters from the user's prompt
type ExtractedIntent struct {
	Mood             string   `json:"mood"`
	Activity         string   `json:"activity"`
	Language         string   `json:"language"`
	DurationMinutes  int      `json:"duration_minutes"`
	AvoidHeardToday  bool     `json:"avoid_heard_today"`
	Action           string   `json:"action"` // "play_now", "queue_songs", "create_playlist", "recommend"
	Genres           []string `json:"genres,omitempty"`
	Artists          []string `json:"artists,omitempty"`
	Preferences      []string `json:"preferences,omitempty"`
	IntentSummary    string   `json:"intent_summary"`
	ExtractionMethod string   `json:"extraction_method"` // "gemini", "huggingface", "semantic_parser"
}

// CopilotCandidateSong represents a candidate song with AI ranking metadata
type CopilotCandidateSong struct {
	Song                     Song     `json:"song"`
	Score                    float64  `json:"score"`
	MatchReason              string   `json:"match_reason"`
	EstimatedDurationSeconds int      `json:"estimated_duration_seconds"`
	Excluded                 bool     `json:"excluded"`
	ExcludeReason            string   `json:"exclude_reason,omitempty"`
	Tags                     []string `json:"tags,omitempty"`
}

// CopilotQueryResponse is returned to the user/client
type CopilotQueryResponse struct {
	Query                string                 `json:"query"`
	Intent               ExtractedIntent        `json:"intent"`
	Message              string                 `json:"message"`
	Songs                []Song                 `json:"songs"`
	Candidates           []CopilotCandidateSong `json:"candidates"`
	TotalDurationMinutes int                    `json:"total_duration_minutes"`
	SongsCount           int                    `json:"songs_count"`
	ExcludedTodayCount   int                    `json:"excluded_today_count"`
	SuggestedAction      string                 `json:"suggested_action"`
	ActionPayload        map[string]interface{} `json:"action_payload,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
}

// CopilotActionRequest represents an action to execute
type CopilotActionRequest struct {
	ActionType          string   `json:"action_type" binding:"required"` // "create_playlist", "queue_songs", "play_now"
	PlaylistName        string   `json:"playlist_name,omitempty"`
	PlaylistDescription string   `json:"playlist_description,omitempty"`
	SongIDs             []string `json:"song_ids" binding:"required"`
	CoverImage          string   `json:"cover_image,omitempty"`
	IsPublic            bool     `json:"is_public"`
}

// CopilotActionResponse represents the result of executing an action
type CopilotActionResponse struct {
	Success    bool      `json:"success"`
	ActionType string    `json:"action_type"`
	Message    string    `json:"message"`
	PlaylistID string    `json:"playlist_id,omitempty"`
	Playlist   *Playlist `json:"playlist,omitempty"`
	Count      int       `json:"count"`
}
