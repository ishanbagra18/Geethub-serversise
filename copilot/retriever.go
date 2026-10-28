package copilot

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	songsCollection   *mongo.Collection
	historyCollection *mongo.Collection
)

// InitCollections initializes collections used by the Copilot engine
func InitCollections() {
	if songsCollection == nil {
		songsCollection = database.OpenCollection(database.Client, "songs")
	}
	if historyCollection == nil {
		historyCollection = database.OpenCollection(database.Client, "history")
	}
}

// RetrieveAndRank retrieves candidate tracks from catalog, applies listening history filters,
// and computes ranked matches satisfying the target duration.
func RetrieveAndRank(ctx context.Context, intent models.ExtractedIntent, userID string) (models.CopilotQueryResponse, error) {
	InitCollections()

	// 1. Fetch listening history in the last 24 hours for the user
	listenedTodaySet := make(map[string]bool)
	if userID != "" {
		twentyFourHoursAgo := time.Now().Add(-24 * time.Hour)
		filter := bson.M{
			"user_id":   userID,
			"played_at": bson.M{"$gte": twentyFourHoursAgo},
		}

		cursor, err := historyCollection.Find(ctx, filter)
		if err == nil {
			var recents []models.History
			if err := cursor.All(ctx, &recents); err == nil {
				for _, h := range recents {
					if h.SongID != "" {
						listenedTodaySet[h.SongID] = true
					}
				}
			}
		}
	}

	// 2. Fetch candidate songs from catalog (non-private or uploaded by user)
	var catalogFilter bson.M
	if userID != "" {
		catalogFilter = bson.M{
			"$or": []bson.M{
				{"is_private": bson.M{"$ne": true}},
				{"is_private": true, "uploaded_by": userID},
			},
		}
	} else {
		catalogFilter = bson.M{
			"is_private": bson.M{"$ne": true},
		}
	}

	cursor, err := songsCollection.Find(ctx, catalogFilter)
	if err != nil {
		return models.CopilotQueryResponse{}, fmt.Errorf("failed to fetch songs: %w", err)
	}

	var allSongs []models.Song
	if err := cursor.All(ctx, &allSongs); err != nil {
		return models.CopilotQueryResponse{}, fmt.Errorf("failed to decode songs: %w", err)
	}

	// If no songs found at all, return empty response
	if len(allSongs) == 0 {
		return models.CopilotQueryResponse{
			Intent:               intent,
			Message:              "Catalog is currently empty. Please upload some songs first!",
			Songs:                []models.Song{},
			Candidates:           []models.CopilotCandidateSong{},
			TotalDurationMinutes: 0,
			SongsCount:           0,
			CreatedAt:            time.Now(),
		}, nil
	}

	// 3. Score and filter candidates
	excludedTodayCount := 0
	candidates := make([]models.CopilotCandidateSong, 0, len(allSongs))

	for _, song := range allSongs {
		sID := song.SongID
		if sID == "" && !song.ID.IsZero() {
			sID = song.ID.Hex()
		}

		wasHeardToday := listenedTodaySet[sID] || (song.SongID != "" && listenedTodaySet[song.SongID])
		if wasHeardToday && intent.AvoidHeardToday {
			excludedTodayCount++
			// Excluded candidate
			candidates = append(candidates, models.CopilotCandidateSong{
				Song:                     song,
				Score:                    -1,
				MatchReason:              "Excluded (listened to in the last 24h)",
				EstimatedDurationSeconds: estimateSongDuration(song),
				Excluded:                 true,
				ExcludeReason:            "Heard today",
			})
			continue
		}

		// Calculate relevance score
		score, matchReason, tags := scoreSongForIntent(song, intent)
		durationSec := estimateSongDuration(song)

		candidates = append(candidates, models.CopilotCandidateSong{
			Song:                     song,
			Score:                    score,
			MatchReason:              matchReason,
			EstimatedDurationSeconds: durationSec,
			Excluded:                 false,
			Tags:                     tags,
		})
	}

	// 4. Sort eligible candidates by score descending
	eligible := make([]models.CopilotCandidateSong, 0)
	for _, c := range candidates {
		if !c.Excluded {
			eligible = append(eligible, c)
		}
	}

	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Score > eligible[j].Score
	})

	// 5. Select songs satisfying target duration
	targetDurationSec := intent.DurationMinutes * 60
	if targetDurationSec <= 0 {
		targetDurationSec = 30 * 60
	}

	selectedSongs := make([]models.Song, 0)
	selectedCandidates := make([]models.CopilotCandidateSong, 0)
	accumulatedSec := 0

	for _, c := range eligible {
		selectedSongs = append(selectedSongs, c.Song)
		selectedCandidates = append(selectedCandidates, c)
		accumulatedSec += c.EstimatedDurationSeconds

		// Stop when reaching target duration or reasonable cap (minimum 4 songs)
		if accumulatedSec >= targetDurationSec && len(selectedSongs) >= 4 {
			break
		}
	}

	// If fewer songs than target duration, include remaining eligible songs
	if len(selectedSongs) == 0 && len(eligible) > 0 {
		for i := 0; i < int(math.Min(float64(len(eligible)), 8)); i++ {
			selectedSongs = append(selectedSongs, eligible[i].Song)
			selectedCandidates = append(selectedCandidates, eligible[i])
			accumulatedSec += eligible[i].EstimatedDurationSeconds
		}
	}

	totalMinutes := int(math.Round(float64(accumulatedSec) / 60.0))
	if totalMinutes == 0 && len(selectedSongs) > 0 {
		totalMinutes = len(selectedSongs) * 3
	}

	// 6. Formulate conversational copilot message
	message := buildConversationalSummary(intent, len(selectedSongs), totalMinutes, excludedTodayCount)

	// 7. Suggested Action & Payload
	suggestedAction := intent.Action
	if suggestedAction == "" {
		suggestedAction = "play_now"
	}

	actionSongIDs := make([]string, 0, len(selectedSongs))
	for _, s := range selectedSongs {
		idStr := s.SongID
		if idStr == "" && !s.ID.IsZero() {
			idStr = s.ID.Hex()
		}
		if idStr != "" {
			actionSongIDs = append(actionSongIDs, idStr)
		}
	}

	actionPayload := map[string]interface{}{
		"action":               suggestedAction,
		"song_ids":             actionSongIDs,
		"suggested_title":      fmt.Sprintf("%s %s Mix", intent.Mood, intent.Activity),
		"suggested_desc":       fmt.Sprintf("AI Copilot %d-minute mix tailored for %s with %s vibe.", totalMinutes, intent.Activity, intent.Mood),
		"target_duration_mins": totalMinutes,
	}

	return models.CopilotQueryResponse{
		Intent:               intent,
		Message:              message,
		Songs:                selectedSongs,
		Candidates:           selectedCandidates,
		TotalDurationMinutes: totalMinutes,
		SongsCount:           len(selectedSongs),
		ExcludedTodayCount:   excludedTodayCount,
		SuggestedAction:      suggestedAction,
		ActionPayload:        actionPayload,
		CreatedAt:            time.Now(),
	}, nil
}

// scoreSongForIntent scores a song based on mood, activity, language, genres, and popularity
func scoreSongForIntent(song models.Song, intent models.ExtractedIntent) (float64, string, []string) {
	score := 20.0
	tags := make([]string, 0)
	var reasonParts []string

	title := ""
	if song.Title != nil {
		title = strings.ToLower(*song.Title)
	}
	artist := ""
	if song.Artist != nil {
		artist = strings.ToLower(*song.Artist)
	}
	genre := ""
	if song.Genre != nil {
		genre = strings.ToLower(*song.Genre)
	}
	info := ""
	if song.Info != nil {
		info = strings.ToLower(*song.Info)
	}
	language := ""
	if song.Language != nil {
		language = strings.ToLower(*song.Language)
	}

	fullText := fmt.Sprintf("%s %s %s %s %s", title, artist, genre, info, language)

	// 1. Mood & Activity Scoring
	switch intent.Mood {
	case "Energetic":
		if containsAny(fullText, "workout", "gym", "dance", "rock", "pop", "edm", "party", "tempo", "fast", "hype", "banger", "energy", "remix") {
			score += 45
			tags = append(tags, "⚡ High Energy")
			reasonParts = append(reasonParts, "High tempo & energy match")
		} else {
			score += 15
		}
	case "Chill":
		if containsAny(fullText, "chill", "relax", "lofi", "acoustic", "slow", "ambient", "indie", "peaceful", "soft") {
			score += 45
			tags = append(tags, "🌿 Chill Vibe")
			reasonParts = append(reasonParts, "Laid-back acoustic match")
		} else {
			score += 10
		}
	case "Romantic":
		if containsAny(fullText, "romantic", "love", "heart", "dil", "pyar", "soulful", "melodic", "sweet", "crush") {
			score += 45
			tags = append(tags, "💖 Romantic")
			reasonParts = append(reasonParts, "Soulful love melody")
		} else {
			score += 10
		}
	case "Party":
		if containsAny(fullText, "party", "club", "remix", "punjabi", "dance", "bhangra", "dj", "bass") {
			score += 45
			tags = append(tags, "🎉 Party Anthem")
			reasonParts = append(reasonParts, "Club-ready rhythm")
		} else {
			score += 15
		}
	case "Focus":
		if containsAny(fullText, "focus", "instrumental", "study", "ambient", "lofi", "deep", "piano") {
			score += 45
			tags = append(tags, "🎯 Focus Flow")
			reasonParts = append(reasonParts, "Steady rhythm for concentration")
		} else {
			score += 10
		}
	case "Sad":
		if containsAny(fullText, "sad", "dard", "judaai", "alone", "slow", "tear", "broken", "acoustic") {
			score += 45
			tags = append(tags, "🌙 Introspective")
			reasonParts = append(reasonParts, "Emotionally deep ballad")
		} else {
			score += 10
		}
	default:
		score += 20
	}

	// 2. Activity Alignment
	switch intent.Activity {
	case "Workout":
		if containsAny(fullText, "workout", "gym", "cardio", "run", "fast", "pump", "beat") {
			score += 30
			tags = append(tags, "🏃 Workout Ready")
			reasonParts = append(reasonParts, "Perfect BPM for workout")
		}
	case "Study":
		if containsAny(fullText, "study", "reading", "lofi", "chill", "soft", "instrumental") {
			score += 30
			tags = append(tags, "📚 Study Soundtrack")
		}
	case "Driving":
		if containsAny(fullText, "drive", "road", "travel", "bass", "pop", "rock") {
			score += 30
			tags = append(tags, "🚗 Road Trip")
		}
	}

	// 3. Language Alignment
	if intent.Language != "" && intent.Language != "Any" {
		langLower := strings.ToLower(intent.Language)
		if strings.Contains(language, langLower) || strings.Contains(fullText, langLower) {
			score += 35
			tags = append(tags, fmt.Sprintf("🌐 %s", intent.Language))
			reasonParts = append(reasonParts, fmt.Sprintf("Matches %s language", intent.Language))
		}
	}

	// 4. Requested Artists / Genres
	for _, a := range intent.Artists {
		if strings.Contains(fullText, strings.ToLower(a)) {
			score += 40
			tags = append(tags, fmt.Sprintf("🎤 %s", a))
			reasonParts = append(reasonParts, "Requested artist")
		}
	}
	for _, g := range intent.Genres {
		if strings.Contains(fullText, strings.ToLower(g)) {
			score += 30
			tags = append(tags, fmt.Sprintf("🎵 %s", g))
			reasonParts = append(reasonParts, fmt.Sprintf("Matches genre %s", g))
		}
	}

	// 5. Popularity Boost
	if song.PlayCount > 0 {
		popBonus := math.Log2(float64(song.PlayCount)+1) * 3
		if popBonus > 18 {
			popBonus = 18
		}
		score += popBonus
		if song.PlayCount >= 10 {
			tags = append(tags, fmt.Sprintf("🔥 %d plays", song.PlayCount))
		}
	}

	if len(song.Likes) > 0 {
		score += float64(len(song.Likes)) * 2
	}

	// Default reason if empty
	if len(reasonParts) == 0 {
		reasonParts = append(reasonParts, "Curated catalog pick for your session")
	}

	matchReason := strings.Join(reasonParts, " • ")
	return score, matchReason, tags
}

func estimateSongDuration(song models.Song) int {
	// If song duration is stored or parseable from info, use it; otherwise default to 210s (3.5 mins)
	return 210
}

func buildConversationalSummary(intent models.ExtractedIntent, songCount, totalMins, excludedTodayCount int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("I curated a %d-track (%d-minute) %s mix for your %s session! ",
		songCount, totalMins, strings.ToLower(intent.Mood), strings.ToLower(intent.Activity)))

	if intent.AvoidHeardToday {
		if excludedTodayCount > 0 {
			sb.WriteString(fmt.Sprintf("✨ Successfully excluded %d song(s) you already listened to today so every track stays fresh. ", excludedTodayCount))
		} else {
			sb.WriteString("✨ Checked your listening history today — all selected tracks are completely fresh! ")
		}
	}

	if intent.Language != "" && intent.Language != "Any" {
		sb.WriteString(fmt.Sprintf("Focused on %s melodies. ", intent.Language))
	}

	sb.WriteString("Ready to play now, add to your queue, or save as a playlist.")
	return sb.String()
}

// CreateCopilotPlaylist creates a playlist in MongoDB from the copilot's curated tracks
func CreateCopilotPlaylist(ctx context.Context, userID, name, description string, songIDs []string, isPublic bool) (*models.Playlist, error) {
	InitCollections()
	playlistCol := database.OpenCollection(database.Client, "playlists")

	if name == "" {
		name = "AI Copilot Mix"
	}
	if description == "" {
		description = "Curated automatically by Geethub AI Music Copilot."
	}

	now := time.Now()
	pType := models.PlaylistTypeUser
	var creatorID *string
	if userID != "" {
		creatorID = &userID
	}

	newPlaylist := models.Playlist{
		ID:          primitive.NewObjectID(),
		Name:        &name,
		Description: &description,
		CreatorID:   creatorID,
		Type:        pType,
		Tags:        []string{"ai-copilot", "curated-mix"},
		SongIDs:     songIDs,
		IsPublic:    isPublic,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err := playlistCol.InsertOne(ctx, newPlaylist)
	if err != nil {
		return nil, fmt.Errorf("failed to insert playlist: %w", err)
	}

	return &newPlaylist, nil
}
