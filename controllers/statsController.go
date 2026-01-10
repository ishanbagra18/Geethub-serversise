package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

// GetMyListeningStats returns listening statistics for the authenticated user
func GetMyListeningStats() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 GetMyListeningStats endpoint hit")

		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// Get time range from query parameter (default: weekly)
		timeRange := c.DefaultQuery("range", "weekly")

		var startTime time.Time
		now := time.Now()

		switch timeRange {
		case "weekly":
			startTime = now.AddDate(0, 0, -7) // Last 7 days
		case "monthly":
			startTime = now.AddDate(0, -1, 0) // Last 30 days
		default:
			startTime = now.AddDate(0, 0, -7) // Default to weekly
			timeRange = "weekly"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Get all history entries for this user in the time range
		filter := bson.M{
			"user_id": userID.(string),
			"played_at": bson.M{
				"$gte": startTime,
				"$lte": now,
			},
		}

		cursor, err := historyCollection.Find(ctx, filter)
		if err != nil {
			log.Println("❌ Failed to fetch history:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch listening stats"})
			return
		}
		defer cursor.Close(ctx)

		type HistoryEntry struct {
			SongID   string    `bson:"song_id"`
			PlayedAt time.Time `bson:"played_at"`
			Duration int       `bson:"duration,omitempty"`
		}

		var historyEntries []HistoryEntry
		if err := cursor.All(ctx, &historyEntries); err != nil {
			log.Println("❌ Failed to parse history:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse history"})
			return
		}

		// Calculate total minutes listened (assuming average 3 mins per song if no duration)
		totalMinutes := 0
		songPlayCounts := make(map[string]int)

		for _, entry := range historyEntries {
			duration := entry.Duration
			if duration == 0 {
				duration = 180 // Default 3 minutes in seconds
			}
			totalMinutes += duration / 60
			songPlayCounts[entry.SongID]++
		}

		// Find top song
		var topSongID string
		maxPlays := 0
		for songID, plays := range songPlayCounts {
			if plays > maxPlays {
				maxPlays = plays
				topSongID = songID
			}
		}

		// Fetch top song details
		var topSong interface{}
		if topSongID != "" {
			var song bson.M
			err := songcollection.FindOne(ctx, bson.M{"song_id": topSongID}).Decode(&song)
			if err == nil {
				topSong = gin.H{
					"song_id": topSongID,
					"title":   song["title"],
					"artist":  song["artist"],
					"image":   song["image_url"],
					"plays":   maxPlays,
				}
			}
		}

		// Find top artist by aggregating play counts
		artistPlayCounts := make(map[string]int)

		// Get all unique song IDs
		uniqueSongIDs := make([]string, 0, len(songPlayCounts))
		for songID := range songPlayCounts {
			uniqueSongIDs = append(uniqueSongIDs, songID)
		}

		// Fetch all songs to get their artists
		if len(uniqueSongIDs) > 0 {
			songCursor, err := songcollection.Find(ctx, bson.M{"song_id": bson.M{"$in": uniqueSongIDs}})
			if err == nil {
				defer songCursor.Close(ctx)

				var songs []bson.M
				if err := songCursor.All(ctx, &songs); err == nil {
					for _, song := range songs {
						songID := song["song_id"].(string)
						plays := songPlayCounts[songID]

						if artist, ok := song["artist"].(string); ok && artist != "" {
							artistPlayCounts[artist] += plays
						}
					}
				}
			}
		}

		// Find top artist
		var topArtist interface{}
		var topArtistName string
		maxArtistPlays := 0
		for artist, plays := range artistPlayCounts {
			if plays > maxArtistPlays {
				maxArtistPlays = plays
				topArtistName = artist
			}
		}

		if topArtistName != "" {
			topArtist = gin.H{
				"name":  topArtistName,
				"plays": maxArtistPlays,
			}
		}

		log.Printf("✅ Stats: %d minutes, top song plays: %d, top artist plays: %d\n", totalMinutes, maxPlays, maxArtistPlays)

		c.JSON(http.StatusOK, gin.H{
			"range":            timeRange,
			"minutes_listened": totalMinutes,
			"top_song":         topSong,
			"top_artist":       topArtist,
		})
	}
}
