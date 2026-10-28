package recommendation

import (
	"math"
	"strings"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// FeatureVector represents normalized term weights for a song or user profile
type FeatureVector map[string]float64

// ContentItem holds a song's feature representation
type ContentItem struct {
	SongID   string
	Song     models.Song
	Features FeatureVector
	Norm     float64
}

// Tokenize and normalize text into clean feature tokens
func tokenize(text string) []string {
	clean := strings.ToLower(text)
	// Replace delimiters with space
	for _, char := range []string{",", "-", "/", "(", ")", "[", "]", "&", "|", "_", "."} {
		clean = strings.ReplaceAll(clean, char, " ")
	}

	rawTokens := strings.Fields(clean)
	tokens := make([]string, 0, len(rawTokens))

	// Stop words to filter out
	stopWords := map[string]bool{
		"the": true, "and": true, "a": true, "an": true, "in": true,
		"of": true, "to": true, "for": true, "with": true, "feat": true,
		"ft": true, "prod": true, "official": true, "audio": true, "video": true,
	}

	for _, t := range rawTokens {
		t = strings.TrimSpace(t)
		if len(t) > 1 && !stopWords[t] {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// ExtractSongFeatures creates a weighted feature vector from metadata
func ExtractSongFeatures(s models.Song) (FeatureVector, float64) {
	vec := make(FeatureVector)

	addTokens := func(prefix string, text string, weight float64) {
		if text == "" {
			return
		}
		tokens := tokenize(text)
		for _, tok := range tokens {
			key := prefix + ":" + tok
			vec[key] += weight
		}
	}

	// 1. Genre features (High weight)
	if s.Genre != nil {
		addTokens("genre", *s.Genre, 3.5)
	}

	// 2. Language features (Medium-high weight)
	if s.Language != nil {
		addTokens("lang", *s.Language, 2.5)
	}

	// 3. Artist features (Very high weight for direct artist affinity)
	if s.Artist != nil {
		addTokens("artist", *s.Artist, 4.0)
	}
	for _, a := range s.Artists {
		addTokens("artist", a, 3.0)
	}

	// 4. Info / Mood / Tags in description (Medium weight)
	if s.Info != nil {
		addTokens("vibe", *s.Info, 1.5)
	}
	if s.Album != nil {
		addTokens("album", *s.Album, 1.8)
	}

	// Calculate Euclidean L2 norm for cosine normalization
	sumSq := 0.0
	for _, val := range vec {
		sumSq += val * val
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		norm = 1.0
	}

	// Normalize vector
	for k, val := range vec {
		vec[k] = val / norm
	}

	return vec, 1.0
}

// CosineSimilarity computes cosine similarity between two normalized feature vectors
func CosineSimilarity(v1, v2 FeatureVector) float64 {
	dot := 0.0
	// Iterate through the smaller map for efficiency
	smaller, larger := v1, v2
	if len(v2) < len(v1) {
		smaller, larger = v2, v1
	}

	for k, val1 := range smaller {
		if val2, exists := larger[k]; exists {
			dot += val1 * val2
		}
	}

	if dot > 1.0 {
		return 1.0
	}
	if dot < 0.0 {
		return 0.0
	}
	return dot
}

// UserTasteProfile captures the aggregate preferences of a user
type UserTasteProfile struct {
	Vector       FeatureVector
	TopGenres    map[string]float64
	TopArtists   map[string]float64
	SourceSongs  map[string]models.Song
	SourceScores map[string]float64
}

// BuildUserTasteProfile aggregates content vectors of songs the user has interacted with
func BuildUserTasteProfile(
	affinities map[string]*UserSongAffinity,
	songMap map[string]models.Song,
) *UserTasteProfile {
	profileVec := make(FeatureVector)
	genreScores := make(map[string]float64)
	artistScores := make(map[string]float64)
	sourceSongs := make(map[string]models.Song)
	sourceScores := make(map[string]float64)

	totalWeight := 0.0

	for songID, aff := range affinities {
		if aff.TotalScore <= 0 {
			continue // Skip songs with net negative engagement (e.g., skipped)
		}

		song, exists := songMap[songID]
		if !exists {
			continue
		}

		sourceSongs[songID] = song
		sourceScores[songID] = aff.TotalScore

		songVec, _ := ExtractSongFeatures(song)
		weight := aff.TotalScore

		for k, val := range songVec {
			profileVec[k] += val * weight
		}
		totalWeight += weight

		if song.Genre != nil && *song.Genre != "" {
			genreScores[*song.Genre] += weight
		}
		if song.Artist != nil && *song.Artist != "" {
			artistScores[*song.Artist] += weight
		}
	}

	// Normalize user vector
	sumSq := 0.0
	for _, val := range profileVec {
		sumSq += val * val
	}
	norm := math.Sqrt(sumSq)
	if norm > 0 {
		for k, val := range profileVec {
			profileVec[k] = val / norm
		}
	}

	return &UserTasteProfile{
		Vector:       profileVec,
		TopGenres:    genreScores,
		TopArtists:   artistScores,
		SourceSongs:  sourceSongs,
		SourceScores: sourceScores,
	}
}

// ContentBasedScore evaluates how well a candidate song aligns with a user taste profile
// Also identifies the top source song responsible for the similarity match
func ContentBasedScore(candidate models.Song, profile *UserTasteProfile) (float64, *models.Song) {
	if profile == nil || len(profile.Vector) == 0 {
		return 0.0, nil
	}

	candidateVec, _ := ExtractSongFeatures(candidate)
	score := CosineSimilarity(profile.Vector, candidateVec)

	// Identify most similar source song from user's history
	var bestSource *models.Song
	maxSourceSim := -1.0

	for sID, srcSong := range profile.SourceSongs {
		srcVec, _ := ExtractSongFeatures(srcSong)
		sim := CosineSimilarity(srcVec, candidateVec)
		// Weight similarity by user's affinity score for that song
		weightedSim := sim * math.Log1p(profile.SourceScores[sID])
		if weightedSim > maxSourceSim {
			maxSourceSim = weightedSim
			sCopy := srcSong
			bestSource = &sCopy
		}
	}

	return score, bestSource
}
