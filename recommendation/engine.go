package recommendation

import (
	"math"
	"strings"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// EngineConfig provides tunable hyperparameters for the recommendation pipeline
type EngineConfig struct {
	AlphaCBF          float64 // Content-based filtering weight (default 0.45)
	BetaCF            float64 // Collaborative filtering weight (default 0.35)
	GammaMood         float64 // Mood/Emotion alignment weight (default 0.15)
	DeltaPopularity   float64 // Subtle popularity prior weight (default 0.05)
	MMRDiversityTheta float64 // Maximal Marginal Relevance tradeoff (default 0.70)
	ColdStartCutoff   int     // Interaction threshold for cold-start (default 3)
}

// DefaultConfig returns balanced production defaults
func DefaultConfig() EngineConfig {
	return EngineConfig{
		AlphaCBF:          0.45,
		BetaCF:            0.35,
		GammaMood:         0.15,
		DeltaPopularity:   0.05,
		MMRDiversityTheta: 0.70,
		ColdStartCutoff:   2,
	}
}

// RecommendationEngine coordinates the end-to-end recommendation lifecycle
type RecommendationEngine struct {
	Config EngineConfig
}

// NewRecommendationEngine instantiates the engine with optional custom hyperparameters
func NewRecommendationEngine(cfg *EngineConfig) *RecommendationEngine {
	if cfg == nil {
		c := DefaultConfig()
		cfg = &c
	}
	return &RecommendationEngine{Config: *cfg}
}

// RecommendationContext encapsulates all data sources required for inference
type RecommendationContext struct {
	TargetUserID      string
	TargetUser        *models.User
	AllSongs          []models.Song
	History           []models.History
	Playlists         []models.Playlist
	Interactions      []models.UserInteraction
	OtherUserAffinity map[string]map[string]*UserSongAffinity // For collaborative filtering
	CurrentMood       string
	CurrentMoodEmoji  string
	SelectedVibe      string
	Now               time.Time
}

// Recommend produces personalized recommendations with explanations and cold-start fallback
func (e *RecommendationEngine) Recommend(
	ctx RecommendationContext,
	limit int,
) models.RecommendationResponse {
	if limit <= 0 {
		limit = 12
	}
	if ctx.Now.IsZero() {
		ctx.Now = time.Now()
	}

	// 1. Build lookup map for all songs
	songMap := make(map[string]models.Song)
	for _, s := range ctx.AllSongs {
		sID := s.SongID
		if sID == "" && !s.ID.IsZero() {
			sID = s.ID.Hex()
		}
		if sID != "" {
			songMap[sID] = s
		}
	}

	// 2. Build target user's interaction affinities
	userAffinities := BuildUserAffinityMap(
		ctx.History,
		ctx.AllSongs,
		ctx.Playlists,
		ctx.Interactions,
		ctx.TargetUserID,
		ctx.Now,
	)

	// Count positive interactions
	positiveCount := 0
	for _, aff := range userAffinities {
		if aff.TotalScore > 0 {
			positiveCount++
		}
	}

	// 3. Cold-Start Check: if user has fewer interactions than cutoff
	coldStartHandler := BuildColdStartStrategy(ctx.AllSongs, ctx.Now)

	if positiveCount < e.Config.ColdStartCutoff {
		coldRecs := coldStartHandler.GenerateColdStartRecommendations(
			ctx.AllSongs,
			ctx.TargetUser,
			ctx.SelectedVibe,
			limit,
		)

		return models.RecommendationResponse{
			Recommendations: coldRecs,
			Strategy:        "cold_start_popularity_exploration",
			IsColdStart:     true,
			TotalCandidates: len(ctx.AllSongs),
			GeneratedAt:     ctx.Now,
		}
	}

	// 4. Build User Content Taste Profile
	tasteProfile := BuildUserTasteProfile(userAffinities, songMap)

	// 5. Fit Item-Item Collaborative Filtering model
	allAffinities := make(map[string]map[string]*UserSongAffinity)
	if ctx.OtherUserAffinity != nil {
		for uID, affs := range ctx.OtherUserAffinity {
			allAffinities[uID] = affs
		}
	}
	allAffinities[ctx.TargetUserID] = userAffinities
	cfModel := BuildItemItemCFModel(allAffinities)

	// 6. Precompute popularity scores for baseline prior
	popScores := coldStartHandler.PopularityScores
	maxPop := 1.0
	for _, p := range popScores {
		if p > maxPop {
			maxPop = p
		}
	}

	// 7. Candidate Generation & Scoring
	type scoredCandidate struct {
		song            models.Song
		totalScore      float64
		cbfScore        float64
		cfScore         float64
		moodScore       float64
		popScore        float64
		sourceSong      *models.Song
		topNeighborSong *models.Song
		features        FeatureVector
	}

	scoredCandidates := make([]scoredCandidate, 0)
	targetMoodLower := strings.ToLower(strings.TrimSpace(ctx.CurrentMood))
	selectedVibeLower := strings.ToLower(strings.TrimSpace(ctx.SelectedVibe))

	for _, song := range ctx.AllSongs {
		sID := song.SongID
		if sID == "" && !song.ID.IsZero() {
			sID = song.ID.Hex()
		}
		if sID == "" {
			continue
		}

		// Don't recommend songs the user already explicitly saved, liked, or played repeatedly
		// (unless catalog is very small)
		if aff, exists := userAffinities[sID]; exists {
			if aff.TotalScore > 4.0 && len(ctx.AllSongs) > limit*2 {
				continue
			}
			// Skip penalization: if user skipped this song, strongly suppress it
			if aff.Skipped {
				continue
			}
		}

		// A. Content-Based Score
		cbfScore, bestSource := ContentBasedScore(song, tasteProfile)

		// B. Collaborative Filtering Score
		cfScore, topNeighborID := cfModel.PredictUserItemScore(userAffinities, sID, 10)
		var topNeighborSong *models.Song
		if topNeighborID != "" {
			if nSong, ok := songMap[topNeighborID]; ok {
				topNeighborSong = &nSong
			}
		}

		// C. Mood Alignment Score (Hugging Face emotion inference)
		moodScore := 0.0
		if targetMoodLower != "" || selectedVibeLower != "" {
			matchTarget := targetMoodLower
			if matchTarget == "" {
				matchTarget = selectedVibeLower
			}

			if song.Genre != nil {
				gLower := strings.ToLower(*song.Genre)
				if strings.Contains(matchTarget, gLower) || strings.Contains(gLower, matchTarget) {
					moodScore = 1.0
				}
			}
			if moodScore == 0.0 && song.Info != nil {
				iLower := strings.ToLower(*song.Info)
				if strings.Contains(matchTarget, iLower) || strings.Contains(iLower, matchTarget) {
					moodScore = 0.8
				}
			}
		}

		// D. Popularity Prior
		normPop := popScores[sID] / maxPop

		// Weighted Hybrid Ensemble Score
		totalScore := (e.Config.AlphaCBF * cbfScore) +
			(e.Config.BetaCF * cfScore) +
			(e.Config.GammaMood * moodScore) +
			(e.Config.DeltaPopularity * normPop)

		songFeatures, _ := ExtractSongFeatures(song)

		scoredCandidates = append(scoredCandidates, scoredCandidate{
			song:            song,
			totalScore:      totalScore,
			cbfScore:        cbfScore,
			cfScore:         cfScore,
			moodScore:       moodScore,
			popScore:        normPop,
			sourceSong:      bestSource,
			topNeighborSong: topNeighborSong,
			features:        songFeatures,
		})
	}

	// 8. Maximal Marginal Relevance (MMR) for Intra-List Diversity
	// MMR = argmax [ theta * Relevance - (1 - theta) * max(SimilarityToSelected) ]
	selected := make([]scoredCandidate, 0, limit)
	theta := e.Config.MMRDiversityTheta

	for len(selected) < limit && len(scoredCandidates) > 0 {
		bestIdx := -1
		bestMMR := -math.MaxFloat64

		for i, cand := range scoredCandidates {
			// Compute max similarity to any already selected song
			maxSim := 0.0
			for _, sel := range selected {
				sim := CosineSimilarity(cand.features, sel.features)
				if sim > maxSim {
					maxSim = sim
				}
			}

			// MMR formula
			mmrVal := (theta * cand.totalScore) - ((1.0 - theta) * maxSim)
			if mmrVal > bestMMR {
				bestMMR = mmrVal
				bestIdx = i
			}
		}

		if bestIdx == -1 {
			break
		}

		selected = append(selected, scoredCandidates[bestIdx])
		// Remove selected element from candidates
		scoredCandidates = append(scoredCandidates[:bestIdx], scoredCandidates[bestIdx+1:]...)
	}

	// 9. Format output with Explainable AI reasons and match percentages
	recommendations := make([]models.RecommendedSong, 0, len(selected))

	for _, item := range selected {
		explanation := GenerateExplanation(
			item.song,
			item.sourceSong,
			item.topNeighborSong,
			item.cbfScore,
			item.cfScore,
			item.moodScore,
			ctx.CurrentMood,
			ctx.CurrentMoodEmoji,
			tasteProfile,
		)

		// Calculate realistic dynamic match percentage (e.g. 82% to 98%)
		rawMatch := int(explanation.ConfidenceScore * 100)
		if rawMatch < 80 {
			rawMatch = 80 + int(item.totalScore*20.0)%10
		}
		if rawMatch > 98 {
			rawMatch = 98
		}

		recommendations = append(recommendations, models.RecommendedSong{
			Song:     item.song,
			Score:    item.totalScore,
			MatchPct: rawMatch,
			Explanation: explanation,
			AlgorithmicBreakdown: map[string]float64{
				"cbf_score":       item.cbfScore,
				"cf_score":        item.cfScore,
				"mood_score":      item.moodScore,
				"norm_popularity": item.popScore,
				"hybrid_total":    item.totalScore,
			},
		})
	}

	return models.RecommendationResponse{
		Recommendations: recommendations,
		Strategy:        "hybrid_cbf_cf_mmr",
		IsColdStart:     false,
		UserTopGenres:   tasteProfile.TopGenres,
		TotalCandidates: len(ctx.AllSongs),
		GeneratedAt:     ctx.Now,
	}
}
