package recommendation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/models"
	"github.com/ishanbagra18/ecommerce-using-go/recommendation"
)

// generateBenchCatalog produces a realistic synthetic catalog of N songs
func generateBenchCatalog(n int) []models.Song {
	genres := []string{"Romantic", "Pop", "Punjabi", "Rock", "EDM", "Lofi", "Acoustic", "Hip-Hop"}
	artists := []string{"Arijit Singh", "AP Dhillon", "Sidhu Moose Wala", "Diljit Dosanjh", "Shreya Ghoshal", "Pritam", "The Weeknd", "Dua Lipa"}
	languages := []string{"Hindi", "Punjabi", "English"}

	songs := make([]models.Song, 0, n)
	now := time.Now()

	for i := 0; i < n; i++ {
		title := fmt.Sprintf("Benchmark Track %d", i)
		artist := artists[i%len(artists)]
		genre := genres[i%len(genres)]
		lang := languages[i%len(languages)]
		sID := fmt.Sprintf("song_bench_%d", i)

		songs = append(songs, models.Song{
			SongID:      sID,
			Title:       &title,
			Artist:      &artist,
			Genre:       &genre,
			Language:    &lang,
			PlayCount:   100 + (i * 7) % 1000,
			CreatedAt:   &now,
			ReleaseDate: &now,
		})
	}
	return songs
}

// generateBenchHistory produces M playback history records across K users
func generateBenchHistory(songs []models.Song, userCount, eventsPerUser int) []models.History {
	history := make([]models.History, 0, userCount*eventsPerUser)
	now := time.Now()

	for u := 0; u < userCount; u++ {
		uID := fmt.Sprintf("user_bench_%d", u)
		for e := 0; e < eventsPerUser; e++ {
			sIdx := (u*3 + e*7) % len(songs)
			history = append(history, models.History{
				UserID:   uID,
				SongID:   songs[sIdx].SongID,
				PlayedAt: now.Add(-time.Duration(e*12) * time.Hour),
			})
		}
	}
	return history
}

// BenchmarkRecommendation_Hybrid measures the end-to-end execution of the hybrid pipeline
func BenchmarkRecommendation_Hybrid(b *testing.B) {
	songs := generateBenchCatalog(100)
	history := generateBenchHistory(songs, 10, 15)

	targetUserID := "user_bench_0"
	targetHistory := make([]models.History, 0)
	for _, h := range history {
		if h.UserID == targetUserID {
			targetHistory = append(targetHistory, h)
		}
	}

	engine := recommendation.NewRecommendationEngine(nil)
	ctx := recommendation.RecommendationContext{
		TargetUserID: targetUserID,
		AllSongs:     songs,
		History:      targetHistory,
		CurrentMood:  "Upbeat & Energetic",
		Now:          time.Now(),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		res := engine.Recommend(ctx, 12)
		if len(res.Recommendations) == 0 {
			b.Fatal("Expected recommendations")
		}
	}
}

// BenchmarkRecommendation_ContentBased measures feature extraction and vector cosine math
func BenchmarkRecommendation_ContentBased(b *testing.B) {
	songs := generateBenchCatalog(200)
	songMap := make(map[string]models.Song)
	for _, s := range songs {
		songMap[s.SongID] = s
	}

	// Build user taste profile from 10 songs
	affinities := make(map[string]*recommendation.UserSongAffinity)
	for i := 0; i < 10; i++ {
		sID := songs[i].SongID
		affinities[sID] = &recommendation.UserSongAffinity{
			UserID:     "u_test",
			SongID:     sID,
			TotalScore: 4.5,
		}
	}
	profile := recommendation.BuildUserTasteProfile(affinities, songMap)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		candidate := songs[i%len(songs)]
		score, _ := recommendation.ContentBasedScore(candidate, profile)
		if score < 0 {
			b.Fatal("Invalid score")
		}
	}
}

// BenchmarkRecommendation_CollaborativeFiltering measures item-item cosine similarity modeling
func BenchmarkRecommendation_CollaborativeFiltering(b *testing.B) {
	songs := generateBenchCatalog(50)
	allAffinities := make(map[string]map[string]*recommendation.UserSongAffinity)

	// Simulate 20 users
	for u := 0; u < 20; u++ {
		uID := fmt.Sprintf("u_%d", u)
		allAffinities[uID] = make(map[string]*recommendation.UserSongAffinity)
		for s := 0; s < 15; s++ {
			sIdx := (u*2 + s) % len(songs)
			sID := songs[sIdx].SongID
			allAffinities[uID][sID] = &recommendation.UserSongAffinity{
				UserID:     uID,
				SongID:     sID,
				TotalScore: float64(1 + (s % 5)),
			}
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		cfModel := recommendation.BuildItemItemCFModel(allAffinities)
		if len(cfModel.ItemSimilarities) == 0 {
			b.Fatal("Expected CF similarities")
		}
	}
}

// BenchmarkRecommendation_ImplicitFeedback measures recency-weighted implicit scoring
func BenchmarkRecommendation_ImplicitFeedback(b *testing.B) {
	now := time.Now()
	twoDaysAgo := now.Add(-48 * time.Hour)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		score := recommendation.ComputeAffinityScore(5, true, true, false, false, twoDaysAgo, now)
		if score <= 0 {
			b.Fatal("Expected positive affinity score")
		}
	}
}

// BenchmarkRecommendation_EvaluationSuite measures offline evaluation metrics computation
func BenchmarkRecommendation_EvaluationSuite(b *testing.B) {
	songs := generateBenchCatalog(30)
	history := generateBenchHistory(songs, 10, 8)
	split := recommendation.CreateTrainTestSplit(history, songs, nil, time.Now(), 0.20)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		report := recommendation.RunBenchmarkSuite(split, []int{5})
		if len(report.Models) == 0 {
			b.Fatal("Expected benchmark report")
		}
	}
}
