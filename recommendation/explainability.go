package recommendation

import (
	"fmt"
	"math"
	"strings"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// GenerateExplanation builds a transparent, human-readable reason for a recommendation
func GenerateExplanation(
	candidate models.Song,
	sourceSong *models.Song,
	topNeighborSong *models.Song,
	cbfScore float64,
	cfScore float64,
	moodBonus float64,
	currentMood string,
	currentMoodEmoji string,
	userTaste *UserTasteProfile,
) models.RecommendationExplanation {
	candidateTitle := "this track"
	if candidate.Title != nil && *candidate.Title != "" {
		candidateTitle = *candidate.Title
	}

	candidateArtist := ""
	if candidate.Artist != nil {
		candidateArtist = *candidate.Artist
	}

	// 1. Check direct artist affinity
	if userTaste != nil && candidateArtist != "" {
		if artistWeight, exists := userTaste.TopArtists[candidateArtist]; exists && artistWeight > 5.0 {
			conf := math.Min(0.98, 0.85+(artistWeight/50.0))
			return models.RecommendationExplanation{
				Reason:          fmt.Sprintf("Because you frequently listen to %s", candidateArtist),
				Type:            "ARTIST_AFFINITY",
				SourceArtist:    candidateArtist,
				ConfidenceScore: conf,
			}
		}
	}

	// 2. Collaborative filtering reason: listeners with similar taste
	if cfScore > 0.45 && topNeighborSong != nil {
		sourceTitle := "your favorites"
		if topNeighborSong.Title != nil && *topNeighborSong.Title != "" {
			sourceTitle = *topNeighborSong.Title
		}
		sourceArtist := ""
		if topNeighborSong.Artist != nil {
			sourceArtist = *topNeighborSong.Artist
		}

		conf := math.Min(0.96, 0.80+cfScore*0.18)
		reasonText := fmt.Sprintf("Listeners who enjoy '%s' also listen to this", sourceTitle)
		if sourceArtist != "" {
			reasonText = fmt.Sprintf("Listeners who enjoy '%s' by %s also love this track", sourceTitle, sourceArtist)
		}

		return models.RecommendationExplanation{
			Reason:          reasonText,
			Type:            "COLLABORATIVE_FILTERING",
			SourceSongTitle: sourceTitle,
			SourceArtist:    sourceArtist,
			ConfidenceScore: conf,
		}
	}

	// 3. Mood alignment from Hugging Face inference
	if moodBonus > 0 && currentMood != "" {
		conf := 0.92
		emoji := "⚡"
		if currentMoodEmoji != "" {
			emoji = currentMoodEmoji
		}
		return models.RecommendationExplanation{
			Reason:          fmt.Sprintf("Matches your current %s %s vibe", emoji, currentMood),
			Type:            "MOOD_ALIGNMENT",
			ConfidenceScore: conf,
		}
	}

	// 4. Content similarity to a specific recent favorite
	if sourceSong != nil && cbfScore > 0.25 {
		sourceTitle := "your recent track"
		if sourceSong.Title != nil && *sourceSong.Title != "" {
			sourceTitle = *sourceSong.Title
		}
		sourceArtist := ""
		if sourceSong.Artist != nil {
			sourceArtist = *sourceSong.Artist
		}

		conf := math.Min(0.97, 0.78+cbfScore*0.22)
		reason := fmt.Sprintf("Similar sound and rhythm to '%s'", sourceTitle)
		if sourceArtist != "" {
			reason = fmt.Sprintf("Similar to your recent favorite '%s' by %s", sourceTitle, sourceArtist)
		}

		return models.RecommendationExplanation{
			Reason:          reason,
			Type:            "CONTENT_SIMILARITY",
			SourceSongTitle: sourceTitle,
			SourceArtist:    sourceArtist,
			ConfidenceScore: conf,
		}
	}

	// 5. Genre-based similarity fallback
	if candidate.Genre != nil && *candidate.Genre != "" {
		genre := *candidate.Genre
		if userTaste != nil {
			if _, exists := userTaste.TopGenres[genre]; exists {
				return models.RecommendationExplanation{
					Reason:          fmt.Sprintf("Top trending track in %s based on your preferences", strings.Title(genre)),
					Type:            "GENRE_AFFINITY",
					ConfidenceScore: 0.82,
				}
			}
		}
	}

	// 6. General high-confidence fallback
	_ = candidateTitle
	return models.RecommendationExplanation{
		Reason:          "Curated based on your GeetHub listening activity",
		Type:            "ACTIVITY_HYBRID",
		ConfidenceScore: 0.80,
	}
}
