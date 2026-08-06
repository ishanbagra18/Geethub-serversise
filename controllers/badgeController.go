package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

var badgeUserCollection *mongo.Collection
var badgeHistoryCollection *mongo.Collection
var badgeSongCollection *mongo.Collection
var badgePlaylistCollection *mongo.Collection

func InitBadgeController() {
	badgeUserCollection = database.OpenCollection(database.Client, "users")
	badgeHistoryCollection = database.OpenCollection(database.Client, "history")
	badgeSongCollection = database.OpenCollection(database.Client, "songs")
	badgePlaylistCollection = database.OpenCollection(database.Client, "playlists")
}

// GetMyBadges computes and returns the authenticated user's earned badges
func GetMyBadges() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 GetMyBadges endpoint hit")

		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		uid := userID.(string)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// 1. Get user info (for account age)
		var user bson.M
		err := badgeUserCollection.FindOne(ctx, bson.M{"user_id": uid}).Decode(&user)
		if err != nil {
			log.Println("❌ Failed to fetch user for badges:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user"})
			return
		}

		// 2. Get total minutes listened and unique songs count from history
		historyFilter := bson.M{"user_id": uid}
		historyCursor, err := badgeHistoryCollection.Find(ctx, historyFilter)
		if err != nil {
			log.Println("❌ Failed to fetch history for badges:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to compute badges"})
			return
		}
		defer historyCursor.Close(ctx)

		type HistoryEntry struct {
			SongID   string `bson:"song_id"`
			Duration int    `bson:"duration,omitempty"`
		}

		totalMinutes := 0
		uniqueSongs := make(map[string]bool)

		for historyCursor.Next(ctx) {
			var entry HistoryEntry
			if err := historyCursor.Decode(&entry); err == nil {
				duration := entry.Duration
				if duration == 0 {
					duration = 180 // Default 3 minutes in seconds
				}
				totalMinutes += duration / 60
				uniqueSongs[entry.SongID] = true
			}
		}

		// 3. Count user's uploaded songs (Verified Artist check)
		uploadedCount, err := badgeSongCollection.CountDocuments(ctx, bson.M{"uploaded_by": uid})
		if err != nil {
			uploadedCount = 0
		}

		// 4. Count user's playlists
		playlistCount, err := badgePlaylistCollection.CountDocuments(ctx, bson.M{"created_by": uid})
		if err != nil {
			playlistCount = 0
		}

		// 5. Compute account age
		var accountAge time.Duration
		if createdAt, ok := user["created_at"]; ok {
			switch v := createdAt.(type) {
			case primitive.DateTime:
				accountAge = time.Since(v.Time())
			case time.Time:
				accountAge = time.Since(v)
			case *time.Time:
				if v != nil {
					accountAge = time.Since(*v)
				}
			}
		}

		now := time.Now()

		// Build badges list
		badges := []models.Badge{
			{
				ID:          "top_listener",
				Name:        "Top Listener",
				Description: "Listened to 500+ minutes of music",
				Icon:        "🎧",
				Color:       "#6366f1",
				Earned:      totalMinutes >= 500,
			},
			{
				ID:          "verified_artist",
				Name:        "Verified Artist",
				Description: "Uploaded at least 1 song to the platform",
				Icon:        "✅",
				Color:       "#10b981",
				Earned:      uploadedCount >= 1,
			},
			{
				ID:          "1000_minutes",
				Name:        "1000 Minutes",
				Description: "Reached 1000 minutes of total listening time",
				Icon:        "⏱️",
				Color:       "#f59e0b",
				Earned:      totalMinutes >= 1000,
			},
			{
				ID:          "playlist_master",
				Name:        "Playlist Master",
				Description: "Created 5 or more playlists",
				Icon:        "📋",
				Color:       "#8b5cf6",
				Earned:      playlistCount >= 5,
			},
			{
				ID:          "early_user",
				Name:        "Early User",
				Description: "Account is older than 6 months",
				Icon:        "🌟",
				Color:       "#ec4899",
				Earned:      accountAge >= 6*30*24*time.Hour,
			},
			{
				ID:          "music_explorer",
				Name:        "Music Explorer",
				Description: "Listened to 50+ unique songs",
				Icon:        "🧭",
				Color:       "#06b6d4",
				Earned:      len(uniqueSongs) >= 50,
			},
		}

		// Set earned_at for earned badges
		for i := range badges {
			if badges[i].Earned {
				badges[i].EarnedAt = &now
			}
		}

		// Count earned
		earnedCount := 0
		for _, b := range badges {
			if b.Earned {
				earnedCount++
			}
		}

		log.Printf("✅ Badges computed for user %s: %d/%d earned\n", uid, earnedCount, len(badges))

		c.JSON(http.StatusOK, gin.H{
			"badges":        badges,
			"total_badges":  len(badges),
			"earned_count":  earnedCount,
			"stats": gin.H{
				"total_minutes": totalMinutes,
				"unique_songs":  len(uniqueSongs),
				"uploads":       uploadedCount,
				"playlists":     playlistCount,
				"account_days":  int(accountAge.Hours() / 24),
			},
		})
	}
}
