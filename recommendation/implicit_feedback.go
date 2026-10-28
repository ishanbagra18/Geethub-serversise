package recommendation

import (
	"math"
	"time"

	"github.com/ishanbagra18/ecommerce-using-go/models"
)

// Weights for different user interactions in implicit feedback
const (
	WeightPlay        = 1.5  // Base weight per log-scaled play
	WeightLike        = 3.0  // Positive engagement
	WeightSave        = 4.0  // High intent engagement (saved to personal library)
	WeightPlaylistAdd = 5.0  // Highest positive intent (added to custom playlist)
	WeightSkip        = 3.5  // Negative signal (user explicitly skipped song)
	HalfLifeDays      = 14.0 // Recency half-life in days
)

// UserSongAffinity stores aggregate interaction metrics between a user and a song
type UserSongAffinity struct {
	UserID        string
	SongID        string
	Plays         int
	Liked         bool
	Saved         bool
	InPlaylist    bool
	Skipped       bool
	LastPlayedAt  time.Time
	TotalScore    float64
}

// ComputeAffinityScore calculates the recency-weighted implicit feedback score
func ComputeAffinityScore(plays int, liked, saved, inPlaylist, skipped bool, lastPlayedAt time.Time, now time.Time) float64 {
	// 1. Play frequency with logarithmic scaling: log2(1 + plays)
	playScore := 0.0
	if plays > 0 {
		playScore = WeightPlay * math.Log2(1.0+float64(plays))
	}

	// 2. Recency decay: e^(-lambda * dt) where lambda = ln(2) / half_life
	recencyWeight := 1.0
	if !lastPlayedAt.IsZero() && now.After(lastPlayedAt) {
		daysSince := now.Sub(lastPlayedAt).Hours() / 24.0
		lambda := math.Ln2 / HalfLifeDays
		recencyWeight = math.Exp(-lambda * daysSince)
		// Clamp minimum recency weight to avoid completely discarding historical taste
		if recencyWeight < 0.2 {
			recencyWeight = 0.2
		}
	}

	weightedPlays := playScore * recencyWeight

	// 3. Positive action boosts
	likeBoost := 0.0
	if liked {
		likeBoost = WeightLike
	}

	saveBoost := 0.0
	if saved {
		saveBoost = WeightSave
	}

	playlistBoost := 0.0
	if inPlaylist {
		playlistBoost = WeightPlaylistAdd
	}

	// 4. Negative skip penalty
	skipPenalty := 0.0
	if skipped {
		skipPenalty = WeightSkip
	}

	total := weightedPlays + likeBoost + saveBoost + playlistBoost - skipPenalty
	return total
}

// BuildUserAffinityMap constructs user-item affinity scores across all interactions
func BuildUserAffinityMap(
	history []models.History,
	songs []models.Song,
	playlists []models.Playlist,
	interactions []models.UserInteraction,
	targetUserID string,
	now time.Time,
) map[string]*UserSongAffinity {
	affinityMap := make(map[string]*UserSongAffinity)

	getOrCreate := func(songID string) *UserSongAffinity {
		if songID == "" {
			return nil
		}
		if aff, exists := affinityMap[songID]; exists {
			return aff
		}
		aff := &UserSongAffinity{
			UserID: targetUserID,
			SongID: songID,
		}
		affinityMap[songID] = aff
		return aff
	}

	// 1. Process listening history
	for _, h := range history {
		if h.UserID == targetUserID && h.SongID != "" {
			aff := getOrCreate(h.SongID)
			if aff != nil {
				aff.Plays++
				if h.PlayedAt.After(aff.LastPlayedAt) {
					aff.LastPlayedAt = h.PlayedAt
				}
			}
		}
	}

	// 2. Process likes & saves directly from songs
	for _, s := range songs {
		sID := s.SongID
		if sID == "" && !s.ID.IsZero() {
			sID = s.ID.Hex()
		}
		if sID == "" {
			continue
		}

		// Check likes
		for _, uID := range s.Likes {
			if uID == targetUserID {
				aff := getOrCreate(sID)
				if aff != nil {
					aff.Liked = true
				}
				break
			}
		}

		// Check saves
		for _, uID := range s.Saves {
			if uID == targetUserID {
				aff := getOrCreate(sID)
				if aff != nil {
					aff.Saved = true
				}
				break
			}
		}
	}

	// 3. Process playlist additions
	for _, pl := range playlists {
		if pl.CreatorID != nil && *pl.CreatorID == targetUserID {
			for _, sID := range pl.SongIDs {
				aff := getOrCreate(sID)
				if aff != nil {
					aff.InPlaylist = true
				}
			}
		}
	}

	// 4. Process interaction events (skips, plays, etc.)
	for _, inter := range interactions {
		if inter.UserID == targetUserID && inter.SongID != "" {
			aff := getOrCreate(inter.SongID)
			if aff != nil {
				if inter.Action == "skip" {
					aff.Skipped = true
				} else if inter.Action == "like" {
					aff.Liked = true
				} else if inter.Action == "save" {
					aff.Saved = true
				} else if inter.Action == "playlist_add" {
					aff.InPlaylist = true
				}
			}
		}
	}

	// 5. Compute total scores
	for _, aff := range affinityMap {
		aff.TotalScore = ComputeAffinityScore(
			aff.Plays,
			aff.Liked,
			aff.Saved,
			aff.InPlaylist,
			aff.Skipped,
			aff.LastPlayedAt,
			now,
		)
	}

	return affinityMap
}
