package recommendation

import (
	"math"
	"strings"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// ColdStartStrategy determines how to generate recommendations when interaction data is scarce
type ColdStartStrategy struct {
	PopularityScores map[string]float64
	GenreTopSongs    map[string][]models.Song
}

// ComputeGlobalPopularity calculates a composite popularity and freshness score for songs
func ComputeGlobalPopularity(songs []models.Song, now time.Time) map[string]float64 {
	scores := make(map[string]float64)

	for _, s := range songs {
		sID := s.SongID
		if sID == "" && !s.ID.IsZero() {
			sID = s.ID.Hex()
		}
		if sID == "" {
			continue
		}

		// Composite engagement formula
		playScore := math.Log2(1.0 + float64(s.PlayCount))
		saveScore := float64(len(s.Saves)) * 2.0
		likeScore := float64(len(s.Likes)) * 1.5

		// Freshness booster for recently released tracks (30 day decay)
		freshnessBoost := 1.0
		var releaseDate time.Time
		if s.ReleaseDate != nil {
			releaseDate = *s.ReleaseDate
		} else if s.CreatedAt != nil {
			releaseDate = *s.CreatedAt
		}

		if !releaseDate.IsZero() && now.After(releaseDate) {
			daysOld := now.Sub(releaseDate).Hours() / 24.0
			freshnessBoost = 1.0 + math.Exp(-daysOld/30.0) // Up to 2x boost for brand new songs
		}

		rawScore := (playScore + saveScore + likeScore) * freshnessBoost
		scores[sID] = rawScore
	}

	return scores
}

// BuildColdStartStrategy organizes songs by popularity and genre diversity
func BuildColdStartStrategy(songs []models.Song, now time.Time) *ColdStartStrategy {
	popScores := ComputeGlobalPopularity(songs, now)
	genreSongs := make(map[string][]models.Song)

	for _, s := range songs {
		g := "general"
		if s.Genre != nil && *s.Genre != "" {
			g = strings.ToLower(*s.Genre)
		}
		genreSongs[g] = append(genreSongs[g], s)
	}

	// Sort genre buckets by popularity
	for g := range genreSongs {
		bucket := genreSongs[g]
		for i := 0; i < len(bucket); i++ {
			for j := i + 1; j < len(bucket); j++ {
				idI := bucket[i].SongID
				if idI == "" {
					idI = bucket[i].ID.Hex()
				}
				idJ := bucket[j].SongID
				if idJ == "" {
					idJ = bucket[j].ID.Hex()
				}
				if popScores[idJ] > popScores[idI] {
					bucket[i], bucket[j] = bucket[j], bucket[i]
				}
			}
		}
		genreSongs[g] = bucket
	}

	return &ColdStartStrategy{
		PopularityScores: popScores,
		GenreTopSongs:    genreSongs,
	}
}

// GenerateColdStartRecommendations generates diverse recommendations for a new user
func (cs *ColdStartStrategy) GenerateColdStartRecommendations(
	allSongs []models.Song,
	user *models.User,
	targetVibe string,
	limit int,
) []models.RecommendedSong {
	if limit <= 0 {
		limit = 12
	}

	results := make([]models.RecommendedSong, 0, limit)
	seen := make(map[string]bool)

	// Set of artists followed by user (if any)
	followedMap := make(map[string]bool)
	if user != nil {
		for _, a := range user.FollowedArtists {
			followedMap[strings.ToLower(strings.TrimSpace(a))] = true
		}
	}

	vibeLower := strings.ToLower(strings.TrimSpace(targetVibe))

	type candidateWithMeta struct {
		song        models.Song
		score       float64
		reason      string
		reasonType  string
		confidence  float64
		sourceName  string
	}

	candidates := make([]candidateWithMeta, 0, len(allSongs))

	for _, s := range allSongs {
		sID := s.SongID
		if sID == "" && !s.ID.IsZero() {
			sID = s.ID.Hex()
		}
		if sID == "" {
			continue
		}

		basePop := cs.PopularityScores[sID]
		score := basePop
		reason := "Popular trending track on GeetHub"
		reasonType := "COLD_START_POPULAR"
		confidence := 0.75
		sourceName := ""

		// Check followed artists bonus
		artistName := ""
		if s.Artist != nil {
			artistName = *s.Artist
		}
		if artistName != "" && followedMap[strings.ToLower(artistName)] {
			score += 15.0
			reason = "Because you follow " + artistName
			reasonType = "ARTIST_AFFINITY"
			confidence = 0.95
			sourceName = artistName
		}

		// Check mood / vibe alignment
		if vibeLower != "" {
			matchesVibe := false
			if s.Genre != nil && strings.Contains(strings.ToLower(*s.Genre), vibeLower) {
				matchesVibe = true
			}
			if s.Info != nil && strings.Contains(strings.ToLower(*s.Info), vibeLower) {
				matchesVibe = true
			}

			if matchesVibe {
				score += 10.0
				reason = "Matches your current vibe: " + targetVibe
				reasonType = "MOOD_ALIGNMENT"
				confidence = 0.90
			}
		}

		candidates = append(candidates, candidateWithMeta{
			song:       s,
			score:      score,
			reason:     reason,
			reasonType: reasonType,
			confidence: confidence,
			sourceName: sourceName,
		})
	}

	// Sort candidates descending by score
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Slotting: Pick top songs while guaranteeing genre diversity (Exploration)
	genreQuota := make(map[string]int)
	maxPerGenre := 3

	for _, c := range candidates {
		if len(results) >= limit {
			break
		}

		sID := c.song.SongID
		if sID == "" {
			sID = c.song.ID.Hex()
		}
		if seen[sID] {
			continue
		}

		g := "general"
		if c.song.Genre != nil && *c.song.Genre != "" {
			g = strings.ToLower(*c.song.Genre)
		}

		// Enforce genre exploration diversity unless we are running out of songs
		if genreQuota[g] >= maxPerGenre && len(candidates)-len(seen) > (limit-len(results)) {
			continue
		}

		seen[sID] = true
		genreQuota[g]++

		matchPct := int(c.confidence * 100)
		if matchPct > 98 {
			matchPct = 98
		}

		results = append(results, models.RecommendedSong{
			Song:     c.song,
			Score:    c.score,
			MatchPct: matchPct,
			Explanation: models.RecommendationExplanation{
				Reason:          c.reason,
				Type:            c.reasonType,
				SourceArtist:    c.sourceName,
				ConfidenceScore: c.confidence,
			},
			AlgorithmicBreakdown: map[string]float64{
				"cold_start_popularity": c.score,
			},
		})
	}

	return results
}
