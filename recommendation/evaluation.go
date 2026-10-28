package recommendation

import (
	"math"
	"math/rand"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// TrainTestSplit holds separated interaction sets for offline evaluation
type TrainTestSplit struct {
	TrainHistory      []models.History
	TestGroundTruth   map[string]map[string]bool // userID -> set of ground truth relevant songIDs
	TrainAffinities   map[string]map[string]*UserSongAffinity
	AllSongs          []models.Song
	AllUsers          []string
}

// ComputePrecisionAtK calculates the proportion of recommended items in Top-K that are relevant
func ComputePrecisionAtK(recommended []string, groundTruth map[string]bool, k int) float64 {
	if k <= 0 || len(recommended) == 0 || len(groundTruth) == 0 {
		return 0.0
	}
	limit := k
	if len(recommended) < limit {
		limit = len(recommended)
	}

	hits := 0
	for i := 0; i < limit; i++ {
		if groundTruth[recommended[i]] {
			hits++
		}
	}
	return float64(hits) / float64(k)
}

// ComputeRecallAtK calculates the proportion of relevant items found in Top-K
func ComputeRecallAtK(recommended []string, groundTruth map[string]bool, k int) float64 {
	if k <= 0 || len(recommended) == 0 || len(groundTruth) == 0 {
		return 0.0
	}
	limit := k
	if len(recommended) < limit {
		limit = len(recommended)
	}

	hits := 0
	for i := 0; i < limit; i++ {
		if groundTruth[recommended[i]] {
			hits++
		}
	}
	return float64(hits) / float64(len(groundTruth))
}

// ComputeHitRateAtK returns 1.0 if any relevant item is in Top-K, 0.0 otherwise
func ComputeHitRateAtK(recommended []string, groundTruth map[string]bool, k int) float64 {
	if k <= 0 || len(recommended) == 0 || len(groundTruth) == 0 {
		return 0.0
	}
	limit := k
	if len(recommended) < limit {
		limit = len(recommended)
	}

	for i := 0; i < limit; i++ {
		if groundTruth[recommended[i]] {
			return 1.0
		}
	}
	return 0.0
}

// ComputeNDCGAtK calculates Normalized Discounted Cumulative Gain at threshold K
func ComputeNDCGAtK(recommended []string, groundTruth map[string]bool, k int) float64 {
	if k <= 0 || len(recommended) == 0 || len(groundTruth) == 0 {
		return 0.0
	}
	limit := k
	if len(recommended) < limit {
		limit = len(recommended)
	}

	// Calculate DCG@K
	dcg := 0.0
	for i := 0; i < limit; i++ {
		rel := 0.0
		if groundTruth[recommended[i]] {
			rel = 1.0
		}
		if rel > 0 {
			// Rank is 1-indexed (i + 1), discount is log2(rank + 1)
			rank := float64(i + 1)
			gain := (math.Pow(2.0, rel) - 1.0) / math.Log2(rank+1.0)
			dcg += gain
		}
	}

	// Calculate Ideal DCG (IDCG@K) - all top positions filled with relevant items
	idcg := 0.0
	idealHits := int(math.Min(float64(k), float64(len(groundTruth))))
	for i := 0; i < idealHits; i++ {
		rank := float64(i + 1)
		idcg += (math.Pow(2.0, 1.0) - 1.0) / math.Log2(rank+1.0)
	}

	if idcg == 0.0 {
		return 0.0
	}

	return dcg / idcg
}

// ComputeMRR calculates Reciprocal Rank (1/rank of first relevant item)
func ComputeMRR(recommended []string, groundTruth map[string]bool, k int) float64 {
	limit := k
	if len(recommended) < limit {
		limit = len(recommended)
	}

	for i := 0; i < limit; i++ {
		if groundTruth[recommended[i]] {
			return 1.0 / float64(i+1)
		}
	}
	return 0.0
}

// ComputeIntraListDiversity calculates average pairwise cosine distance between recommended items
func ComputeIntraListDiversity(recommended []string, songMap map[string]models.Song) float64 {
	if len(recommended) < 2 {
		return 1.0
	}

	totalDist := 0.0
	pairs := 0

	for i := 0; i < len(recommended); i++ {
		songA, okA := songMap[recommended[i]]
		if !okA {
			continue
		}
		vecA, _ := ExtractSongFeatures(songA)

		for j := i + 1; j < len(recommended); j++ {
			songB, okB := songMap[recommended[j]]
			if !okB {
				continue
			}
			vecB, _ := ExtractSongFeatures(songB)

			sim := CosineSimilarity(vecA, vecB)
			// Distance = 1.0 - CosineSimilarity
			dist := 1.0 - sim
			totalDist += dist
			pairs++
		}
	}

	if pairs == 0 {
		return 0.0
	}
	return totalDist / float64(pairs)
}

// CreateTrainTestSplit partitions user histories into 80% train and 20% test hold-out sets
func CreateTrainTestSplit(
	history []models.History,
	songs []models.Song,
	playlists []models.Playlist,
	now time.Time,
	testRatio float64,
) *TrainTestSplit {
	if testRatio <= 0 || testRatio >= 1.0 {
		testRatio = 0.20
	}

	// Group history by user
	userHistory := make(map[string][]models.History)
	for _, h := range history {
		if h.UserID != "" && h.SongID != "" {
			userHistory[h.UserID] = append(userHistory[h.UserID], h)
		}
	}

	trainHist := make([]models.History, 0)
	testGroundTruth := make(map[string]map[string]bool)
	allUsers := make([]string, 0, len(userHistory))

	r := rand.New(rand.NewSource(42)) // Fixed seed for reproducible benchmarks

	for uID, hList := range userHistory {
		allUsers = append(allUsers, uID)

		if len(hList) < 3 {
			// Keep small histories in train to retain user profile
			trainHist = append(trainHist, hList...)
			continue
		}

		// Shuffle and split
		shuffled := make([]models.History, len(hList))
		copy(shuffled, hList)
		r.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		testCount := int(math.Ceil(float64(len(shuffled)) * testRatio))
		if testCount < 1 {
			testCount = 1
		}
		if testCount >= len(shuffled) {
			testCount = len(shuffled) - 1
		}

		testItems := shuffled[:testCount]
		trainItems := shuffled[testCount:]

		trainHist = append(trainHist, trainItems...)

		testGroundTruth[uID] = make(map[string]bool)
		for _, t := range testItems {
			testGroundTruth[uID][t.SongID] = true
		}
	}

	// Build training affinities
	trainAffinities := make(map[string]map[string]*UserSongAffinity)
	for _, uID := range allUsers {
		affMap := BuildUserAffinityMap(trainHist, songs, playlists, nil, uID, now)
		trainAffinities[uID] = affMap
	}

	return &TrainTestSplit{
		TrainHistory:    trainHist,
		TestGroundTruth: testGroundTruth,
		TrainAffinities: trainAffinities,
		AllSongs:        songs,
		AllUsers:        allUsers,
	}
}

// EvaluateModel evaluates a recommendation engine across users for a given threshold K
func EvaluateModel(
	engine *RecommendationEngine,
	split *TrainTestSplit,
	k int,
	modelName string,
) models.EvaluationResult {
	songMap := make(map[string]models.Song)
	for _, s := range split.AllSongs {
		sID := s.SongID
		if sID == "" && !s.ID.IsZero() {
			sID = s.ID.Hex()
		}
		if sID != "" {
			songMap[sID] = s
		}
	}

	totalPrecision := 0.0
	totalRecall := 0.0
	totalHitRate := 0.0
	totalNDCG := 0.0
	totalMRR := 0.0
	totalDiversity := 0.0

	evaluatedUsers := 0
	uniqueRecommended := make(map[string]bool)
	totalTestSize := 0

	for _, uID := range split.AllUsers {
		truth, hasTest := split.TestGroundTruth[uID]
		if !hasTest || len(truth) == 0 {
			continue
		}

		totalTestSize += len(truth)

		// Create recommendation context restricted to training data
		ctx := RecommendationContext{
			TargetUserID:      uID,
			AllSongs:          split.AllSongs,
			History:           split.TrainHistory,
			OtherUserAffinity: split.TrainAffinities,
			Now:               time.Now(),
		}

		res := engine.Recommend(ctx, k)

		recommendedIDs := make([]string, 0, len(res.Recommendations))
		for _, r := range res.Recommendations {
			sID := r.Song.SongID
			if sID == "" {
				sID = r.Song.ID.Hex()
			}
			recommendedIDs = append(recommendedIDs, sID)
			uniqueRecommended[sID] = true
		}

		p := ComputePrecisionAtK(recommendedIDs, truth, k)
		rec := ComputeRecallAtK(recommendedIDs, truth, k)
		hr := ComputeHitRateAtK(recommendedIDs, truth, k)
		ndcg := ComputeNDCGAtK(recommendedIDs, truth, k)
		mrr := ComputeMRR(recommendedIDs, truth, k)
		div := ComputeIntraListDiversity(recommendedIDs, songMap)

		totalPrecision += p
		totalRecall += rec
		totalHitRate += hr
		totalNDCG += ndcg
		totalMRR += mrr
		totalDiversity += div
		evaluatedUsers++
	}

	if evaluatedUsers == 0 {
		return models.EvaluationResult{
			K:         k,
			ModelName: modelName,
		}
	}

	n := float64(evaluatedUsers)
	catalogCoverage := float64(len(uniqueRecommended)) / math.Max(1.0, float64(len(split.AllSongs)))

	return models.EvaluationResult{
		K:               k,
		PrecisionAtK:    math.Round((totalPrecision/n)*10000.0) / 10000.0,
		RecallAtK:       math.Round((totalRecall/n)*10000.0) / 10000.0,
		HitRateAtK:      math.Round((totalHitRate/n)*10000.0) / 10000.0,
		NDCGAtK:         math.Round((totalNDCG/n)*10000.0) / 10000.0,
		MRR:             math.Round((totalMRR/n)*10000.0) / 10000.0,
		CatalogCoverage: math.Round(catalogCoverage*10000.0) / 10000.0,
		DiversityScore:  math.Round((totalDiversity/n)*10000.0) / 10000.0,
		UserCount:       evaluatedUsers,
		TestSetSize:     totalTestSize,
		ModelName:       modelName,
	}
}

// RunBenchmarkSuite compares multiple algorithms (Hybrid, CBF, CF, Popularity) across K values
func RunBenchmarkSuite(split *TrainTestSplit, kValues []int) models.BenchmarkReport {
	if len(kValues) == 0 {
		kValues = []int{5, 10, 20}
	}

	modelsToBenchmark := map[string]EngineConfig{
		"Hybrid (CBF+CF+MMR)": {
			AlphaCBF:          0.45,
			BetaCF:            0.35,
			GammaMood:         0.15,
			DeltaPopularity:   0.05,
			MMRDiversityTheta: 0.70,
			ColdStartCutoff:   1,
		},
		"Content-Based (CBF)": {
			AlphaCBF:          1.00,
			BetaCF:            0.00,
			GammaMood:         0.00,
			DeltaPopularity:   0.00,
			MMRDiversityTheta: 1.00,
			ColdStartCutoff:   1,
		},
		"Collaborative Filtering (CF)": {
			AlphaCBF:          0.00,
			BetaCF:            1.00,
			GammaMood:         0.00,
			DeltaPopularity:   0.00,
			MMRDiversityTheta: 1.00,
			ColdStartCutoff:   1,
		},
		"Popularity Baseline": {
			AlphaCBF:          0.00,
			BetaCF:            0.00,
			GammaMood:         0.00,
			DeltaPopularity:   1.00,
			MMRDiversityTheta: 1.00,
			ColdStartCutoff:   999, // Force cold-start popularity
		},
	}

	report := models.BenchmarkReport{
		Models:      make(map[string][]models.EvaluationResult),
		EvaluatedAt: time.Now(),
		Summary:     "Offline evaluation comparing Recall@K, Precision@K, NDCG@K, MRR, and Diversity across recommendation algorithms on test splits.",
	}

	for name, cfg := range modelsToBenchmark {
		engine := NewRecommendationEngine(&cfg)
		results := make([]models.EvaluationResult, 0, len(kValues))
		for _, k := range kValues {
			res := EvaluateModel(engine, split, k, name)
			results = append(results, res)
		}
		report.Models[name] = results
	}

	return report
}
