package recommendation_test

import (
	"math"
	"testing"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/models"
	"github.com/ishanbagra18/ecommerce-using-go/recommendation"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Helper to create string pointers
func strPtr(s string) *string {
	return &s
}

func sampleSong(id, title, artist, genre, lang string, plays int) models.Song {
	now := time.Now()
	return models.Song{
		ID:          primitive.NewObjectID(),
		SongID:      id,
		Title:       strPtr(title),
		Artist:      strPtr(artist),
		Genre:       strPtr(genre),
		Language:    strPtr(lang),
		PlayCount:   plays,
		CreatedAt:   &now,
		ReleaseDate: &now,
	}
}

func TestImplicitFeedbackScore(t *testing.T) {
	now := time.Now()

	// 1. Base play score
	score1 := recommendation.ComputeAffinityScore(4, false, false, false, false, now, now)
	expectedPlays := 1.5 * math.Log2(1.0+4.0)
	if math.Abs(score1-expectedPlays) > 0.01 {
		t.Errorf("Expected play score ~%f, got %f", expectedPlays, score1)
	}

	// 2. Like and Save boosts
	scoreWithBoosts := recommendation.ComputeAffinityScore(4, true, true, true, false, now, now)
	expectedBoosts := expectedPlays + recommendation.WeightLike + recommendation.WeightSave + recommendation.WeightPlaylistAdd
	if math.Abs(scoreWithBoosts-expectedBoosts) > 0.01 {
		t.Errorf("Expected score with boosts ~%f, got %f", expectedBoosts, scoreWithBoosts)
	}

	// 3. Skip penalty
	scoreSkipped := recommendation.ComputeAffinityScore(4, false, false, false, true, now, now)
	expectedSkip := expectedPlays - recommendation.WeightSkip
	if math.Abs(scoreSkipped-expectedSkip) > 0.01 {
		t.Errorf("Expected score with skip ~%f, got %f", expectedSkip, scoreSkipped)
	}

	// 4. Recency decay check (14 days = 1 half-life, so plays weight should be ~50%)
	twoWeeksAgo := now.Add(-14 * 24 * time.Hour)
	scoreDecayed := recommendation.ComputeAffinityScore(4, false, false, false, false, twoWeeksAgo, now)
	if scoreDecayed >= score1 {
		t.Errorf("Expected decayed score (%f) to be strictly less than fresh score (%f)", scoreDecayed, score1)
	}
}

func TestContentBasedSimilarity(t *testing.T) {
	songA := sampleSong("1", "Kesariya", "Arijit Singh", "Romantic", "Hindi", 100)
	songB := sampleSong("2", "Apna Bana Le", "Arijit Singh", "Romantic", "Hindi", 80)
	songC := sampleSong("3", "Levels", "Sidhu Moose Wala", "Punjabi", "Punjabi", 120)

	vecA, _ := recommendation.ExtractSongFeatures(songA)
	vecB, _ := recommendation.ExtractSongFeatures(songB)
	vecC, _ := recommendation.ExtractSongFeatures(songC)

	simAB := recommendation.CosineSimilarity(vecA, vecB)
	simAC := recommendation.CosineSimilarity(vecA, vecC)

	if simAB <= simAC {
		t.Errorf("Expected Kesariya and Apna Bana Le (same artist, genre, lang) to have higher similarity (%f) than Levels (%f)", simAB, simAC)
	}
	if simAB < 0.80 {
		t.Errorf("Expected high similarity (>0.80) for songs with identical artist & genre, got %f", simAB)
	}
}

func TestColdStartStrategy(t *testing.T) {
	songs := []models.Song{
		sampleSong("1", "Track 1", "Artist A", "Pop", "Hindi", 500),
		sampleSong("2", "Track 2", "Artist B", "Rock", "English", 200),
		sampleSong("3", "Track 3", "Artist C", "Punjabi", "Punjabi", 800),
		sampleSong("4", "Track 4", "Artist D", "Pop", "Hindi", 100),
	}

	coldStrategy := recommendation.BuildColdStartStrategy(songs, time.Now())
	user := &models.User{
		User_id:         "new_user_123",
		FollowedArtists: []string{"Artist B"},
	}

	recs := coldStrategy.GenerateColdStartRecommendations(songs, user, "", 4)
	if len(recs) == 0 {
		t.Fatalf("Expected cold-start recommendations, got 0")
	}

	// The first recommendation should prioritize followed artist "Artist B"
	foundFollowedArtistBoost := false
	for _, r := range recs {
		if r.Explanation.Type == "ARTIST_AFFINITY" && r.Explanation.SourceArtist == "Artist B" {
			foundFollowedArtistBoost = true
			break
		}
	}

	if !foundFollowedArtistBoost {
		t.Errorf("Expected cold-start to prioritize followed artist 'Artist B'")
	}
}

func TestEvaluationMetrics(t *testing.T) {
	recommended := []string{"song1", "song2", "song3", "song4", "song5"}
	groundTruth := map[string]bool{
		"song2": true,
		"song4": true,
	}

	// Precision@5 = 2/5 = 0.4
	p5 := recommendation.ComputePrecisionAtK(recommended, groundTruth, 5)
	if math.Abs(p5-0.4) > 1e-4 {
		t.Errorf("Expected Precision@5 to be 0.4, got %f", p5)
	}

	// Recall@5 = 2/2 = 1.0
	r5 := recommendation.ComputeRecallAtK(recommended, groundTruth, 5)
	if math.Abs(r5-1.0) > 1e-4 {
		t.Errorf("Expected Recall@5 to be 1.0, got %f", r5)
	}

	// Precision@2 = 1/2 = 0.5 (only song2 is in top 2)
	p2 := recommendation.ComputePrecisionAtK(recommended, groundTruth, 2)
	if math.Abs(p2-0.5) > 1e-4 {
		t.Errorf("Expected Precision@2 to be 0.5, got %f", p2)
	}

	// HitRate@2 = 1.0
	hr2 := recommendation.ComputeHitRateAtK(recommended, groundTruth, 2)
	if hr2 != 1.0 {
		t.Errorf("Expected HitRate@2 to be 1.0, got %f", hr2)
	}

	// NDCG@5 must be > 0 and <= 1.0
	ndcg5 := recommendation.ComputeNDCGAtK(recommended, groundTruth, 5)
	if ndcg5 <= 0.0 || ndcg5 > 1.0 {
		t.Errorf("Expected valid NDCG@5 in (0, 1], got %f", ndcg5)
	}

	// MRR: first hit is at index 1 (rank 2) -> 1/2 = 0.5
	mrr := recommendation.ComputeMRR(recommended, groundTruth, 5)
	if math.Abs(mrr-0.5) > 1e-4 {
		t.Errorf("Expected MRR to be 0.5, got %f", mrr)
	}
}

func TestHybridRecommendationPipeline(t *testing.T) {
	now := time.Now()
	songs := []models.Song{
		sampleSong("s1", "Tum Hi Ho", "Arijit Singh", "Romantic", "Hindi", 500),
		sampleSong("s2", "Channa Mereya", "Arijit Singh", "Romantic", "Hindi", 400),
		sampleSong("s3", "Illuminati", "Sushin Shyam", "EDM", "Malayalam", 300),
		sampleSong("s4", "Brown Munde", "AP Dhillon", "Punjabi", "Punjabi", 600),
		sampleSong("s5", "Gerua", "Arijit Singh", "Romantic", "Hindi", 350),
		sampleSong("s6", "Excuses", "AP Dhillon", "Punjabi", "Punjabi", 450),
	}

	// User listened to s1 (Tum Hi Ho) and s2 (Channa Mereya) repeatedly
	history := []models.History{
		{UserID: "u1", SongID: "s1", PlayedAt: now.Add(-1 * time.Hour)},
		{UserID: "u1", SongID: "s1", PlayedAt: now.Add(-2 * time.Hour)},
		{UserID: "u1", SongID: "s2", PlayedAt: now.Add(-3 * time.Hour)},
		{UserID: "u1", SongID: "s2", PlayedAt: now.Add(-4 * time.Hour)},
	}

	// Collaborative signal from another user who liked s1, s2, and s5
	otherAffinity := map[string]map[string]*recommendation.UserSongAffinity{
		"u2": {
			"s1": {UserID: "u2", SongID: "s1", TotalScore: 5.0},
			"s2": {UserID: "u2", SongID: "s2", TotalScore: 4.5},
			"s5": {UserID: "u2", SongID: "s5", TotalScore: 5.0},
		},
	}

	engine := recommendation.NewRecommendationEngine(nil)

	ctx := recommendation.RecommendationContext{
		TargetUserID:      "u1",
		AllSongs:          songs,
		History:           history,
		OtherUserAffinity: otherAffinity,
		CurrentMood:       "Romantic & Soulful",
		CurrentMoodEmoji:  "🌙",
		Now:               now,
	}

	res := engine.Recommend(ctx, 3)

	if len(res.Recommendations) == 0 {
		t.Fatalf("Expected recommendations, got 0")
	}

	if res.IsColdStart {
		t.Errorf("Expected active user recommendation, not cold start")
	}

	// Top recommended should be s5 (Gerua by Arijit Singh) due to high CBF + CF + Mood alignment
	topRec := res.Recommendations[0]
	if topRec.Song.SongID != "s5" {
		t.Logf("Top recommendation was %s (%s)", topRec.Song.SongID, *topRec.Song.Title)
	}

	// Explanation must be present and non-empty
	if topRec.Explanation.Reason == "" {
		t.Errorf("Expected non-empty explanation reason")
	}
	if topRec.MatchPct < 70 || topRec.MatchPct > 100 {
		t.Errorf("Expected match percentage in [70, 100], got %d", topRec.MatchPct)
	}
}

func TestBenchmarkSuite(t *testing.T) {
	now := time.Now()
	songs := []models.Song{
		sampleSong("s1", "Song 1", "Artist A", "Pop", "Hindi", 100),
		sampleSong("s2", "Song 2", "Artist A", "Pop", "Hindi", 120),
		sampleSong("s3", "Song 3", "Artist B", "Rock", "English", 80),
		sampleSong("s4", "Song 4", "Artist B", "Rock", "English", 90),
		sampleSong("s5", "Song 5", "Artist C", "EDM", "English", 200),
		sampleSong("s6", "Song 6", "Artist C", "EDM", "English", 210),
	}

	history := []models.History{
		{UserID: "u1", SongID: "s1", PlayedAt: now.Add(-1 * time.Hour)},
		{UserID: "u1", SongID: "s2", PlayedAt: now.Add(-2 * time.Hour)},
		{UserID: "u1", SongID: "s3", PlayedAt: now.Add(-3 * time.Hour)},
		{UserID: "u2", SongID: "s3", PlayedAt: now.Add(-1 * time.Hour)},
		{UserID: "u2", SongID: "s4", PlayedAt: now.Add(-2 * time.Hour)},
		{UserID: "u2", SongID: "s5", PlayedAt: now.Add(-3 * time.Hour)},
	}

	split := recommendation.CreateTrainTestSplit(history, songs, nil, now, 0.33)
	report := recommendation.RunBenchmarkSuite(split, []int{3})

	if len(report.Models) == 0 {
		t.Fatalf("Expected benchmark report to contain models")
	}

	for modelName, results := range report.Models {
		if len(results) == 0 {
			t.Errorf("Expected results for model %s", modelName)
		}
		for _, r := range results {
			if r.PrecisionAtK < 0 || r.RecallAtK < 0 || r.NDCGAtK < 0 {
				t.Errorf("Invalid metric values for %s: %+v", modelName, r)
			}
		}
	}
}
