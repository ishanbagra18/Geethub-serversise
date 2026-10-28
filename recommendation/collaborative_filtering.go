package recommendation

import (
	"math"
)

// ItemItemCFModel encapsulates the item-item similarity matrix and user-item interactions
type ItemItemCFModel struct {
	ItemSimilarities map[string]map[string]float64 // songID -> (otherSongID -> simScore)
	UserInteractions map[string]map[string]float64 // userID -> (songID -> affinity)
	ItemUsers        map[string]map[string]float64 // songID -> (userID -> affinity)
	ItemNorms        map[string]float64
}

// Shrinkage parameter to regularize items with very few overlapping listeners
const ShrinkageParameter = 2.0

// BuildItemItemCFModel constructs the collaborative filtering similarity matrix from dataset
func BuildItemItemCFModel(
	allUsersAffinities map[string]map[string]*UserSongAffinity,
) *ItemItemCFModel {
	userInteractions := make(map[string]map[string]float64)
	itemUsers := make(map[string]map[string]float64)
	itemNorms := make(map[string]float64)

	// Populate interaction matrices
	for uID, affMap := range allUsersAffinities {
		if userInteractions[uID] == nil {
			userInteractions[uID] = make(map[string]float64)
		}
		for sID, aff := range affMap {
			if aff.TotalScore > 0 {
				userInteractions[uID][sID] = aff.TotalScore

				if itemUsers[sID] == nil {
					itemUsers[sID] = make(map[string]float64)
				}
				itemUsers[sID][uID] = aff.TotalScore
			}
		}
	}

	// Compute Euclidean norms for items
	for sID, users := range itemUsers {
		sumSq := 0.0
		for _, score := range users {
			sumSq += score * score
		}
		itemNorms[sID] = math.Sqrt(sumSq)
	}

	// Compute regularized cosine similarity between items
	itemSimilarities := make(map[string]map[string]float64)

	for itemA, usersA := range itemUsers {
		if itemSimilarities[itemA] == nil {
			itemSimilarities[itemA] = make(map[string]float64)
		}

		for itemB, usersB := range itemUsers {
			if itemA == itemB {
				continue
			}

			// Dot product over co-rated users
			dot := 0.0
			for uID, scoreA := range usersA {
				if scoreB, ok := usersB[uID]; ok {
					dot += scoreA * scoreB
				}
			}

			if dot > 0 {
				normA := itemNorms[itemA]
				normB := itemNorms[itemB]
				// Cosine with shrinkage
				denominator := (normA * normB) + ShrinkageParameter
				if denominator > 0 {
					sim := dot / denominator
					if sim > 0.01 {
						itemSimilarities[itemA][itemB] = sim
					}
				}
			}
		}
	}

	return &ItemItemCFModel{
		ItemSimilarities: itemSimilarities,
		UserInteractions: userInteractions,
		ItemUsers:        itemUsers,
		ItemNorms:        itemNorms,
	}
}

// PredictUserItemScore predicts user affinity for candidate item using Item-Item CF
// Returns (predictedScore, topContributingNeighborSongID)
func (m *ItemItemCFModel) PredictUserItemScore(
	userAffinities map[string]*UserSongAffinity,
	candidateSongID string,
	kNeighbors int,
) (float64, string) {
	if m == nil || len(m.ItemSimilarities) == 0 {
		return 0.0, ""
	}

	similarities, exists := m.ItemSimilarities[candidateSongID]
	if !exists || len(similarities) == 0 {
		return 0.0, ""
	}

	numerator := 0.0
	denominator := 0.0
	bestSim := 0.0
	topNeighborID := ""

	count := 0
	for pastSongID, aff := range userAffinities {
		if aff.TotalScore <= 0 || pastSongID == candidateSongID {
			continue
		}

		sim, hasSim := similarities[pastSongID]
		if !hasSim || sim <= 0 {
			continue
		}

		numerator += sim * aff.TotalScore
		denominator += sim
		count++

		if sim > bestSim {
			bestSim = sim
			topNeighborID = pastSongID
		}

		if kNeighbors > 0 && count >= kNeighbors {
			break
		}
	}

	if denominator == 0 {
		return 0.0, ""
	}

	score := numerator / (denominator + 1e-6)
	// Normalize to 0.0 - 1.0 range
	normalized := math.Tanh(score / 5.0)
	return normalized, topNeighborID
}

// GetSimilarItems returns top K most similar items to target song (Item-to-Item CF)
func (m *ItemItemCFModel) GetSimilarItems(targetSongID string, k int) []struct {
	SongID     string
	Similarity float64
} {
	if m == nil {
		return nil
	}
	sims, ok := m.ItemSimilarities[targetSongID]
	if !ok {
		return nil
	}

	type simPair struct {
		SongID     string
		Similarity float64
	}

	pairs := make([]simPair, 0, len(sims))
	for sID, score := range sims {
		pairs = append(pairs, simPair{SongID: sID, Similarity: score})
	}

	// Simple sort descending
	for i := 0; i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].Similarity > pairs[i].Similarity {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	if k > 0 && len(pairs) > k {
		pairs = pairs[:k]
	}

	res := make([]struct {
		SongID     string
		Similarity float64
	}, len(pairs))
	for idx, p := range pairs {
		res[idx] = struct {
			SongID     string
			Similarity float64
		}{SongID: p.SongID, Similarity: p.Similarity}
	}
	return res
}
