package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	statsSongCollection    *mongo.Collection
	statsHistoryCollection *mongo.Collection
)

func InitStatsController() {
	statsSongCollection = database.OpenCollection(database.Client, "songs")
	statsHistoryCollection = database.OpenCollection(database.Client, "history")
}

func GetMyListeningStats() gin.HandlerFunc {
	return func(c *gin.Context) {

		// ================= AUTH =================
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// ================= RANGE =================
		rangeType := c.DefaultQuery("range", "weekly")

		now := time.Now()
		var from time.Time

		if rangeType == "monthly" {
			from = now.AddDate(0, -1, 0)
		} else {
			from = now.AddDate(0, 0, -7)
		}

		ctx := context.Background()

		// =================================================
		// 1️⃣ MINUTES LISTENED
		// =================================================
		minutesListened := 0

		minutesPipeline := []bson.M{
			{
				"$match": bson.M{
					"user_id": userID.(string),
					"played_at": bson.M{
						"$gte": from,
					},
				},
			},
			{
				"$group": bson.M{
					"_id":          nil,
					"totalSeconds": bson.M{"$sum": "$duration"},
				},
			},
		}

		cursor, err := statsHistoryCollection.Aggregate(ctx, minutesPipeline)
		if err == nil {
			var result []bson.M
			if cursor.All(ctx, &result) == nil && len(result) > 0 {
				switch v := result[0]["totalSeconds"].(type) {
				case int32:
					minutesListened = int(v) / 60
				case int64:
					minutesListened = int(v) / 60
				}
			}
		}

		// =================================================
		// 2️⃣ TOP SONG (WITH DETAILS)
		// =================================================
		var topSongData interface{} = nil

		topSongPipeline := []bson.M{
			{
				"$match": bson.M{
					"user_id": userID.(string),
					"played_at": bson.M{
						"$gte": from,
					},
				},
			},
			{
				"$group": bson.M{
					"_id":   "$song_id",
					"plays": bson.M{"$sum": 1},
				},
			},
			{"$sort": bson.M{"plays": -1}},
			{"$limit": 1},
		}

		cursor, err = statsHistoryCollection.Aggregate(ctx, topSongPipeline)
		if err == nil {
			var topSong []bson.M
			if cursor.All(ctx, &topSong) == nil && len(topSong) > 0 {

				songID := topSong[0]["_id"].(string)
				plays := topSong[0]["plays"]

				var song bson.M
				err := statsSongCollection.FindOne(
					ctx,
					bson.M{"song_id": songID},
				).Decode(&song)

				if err == nil {
					topSongData = gin.H{
						"song_id": songID,
						"title":   song["title"],
						"artist":  song["artist"],
						"image":   song["image_url"],
						"plays":   plays,
					}
				}
			}
		}

		// =================================================
		// 3️⃣ TOP ARTIST
		// =================================================
		var topArtistData interface{} = nil

		topArtistPipeline := []bson.M{
			{
				"$match": bson.M{
					"user_id": userID.(string),
					"played_at": bson.M{
						"$gte": from,
					},
				},
			},
			{
				"$lookup": bson.M{
					"from":         "songs",
					"localField":   "song_id",
					"foreignField": "song_id",
					"as":           "song",
				},
			},
			{"$unwind": "$song"},
			{
				"$group": bson.M{
					"_id":   "$song.artist",
					"plays": bson.M{"$sum": 1},
				},
			},
			{"$sort": bson.M{"plays": -1}},
			{"$limit": 1},
		}

		cursor, err = statsHistoryCollection.Aggregate(ctx, topArtistPipeline)
		if err == nil {
			var topArtist []bson.M
			if cursor.All(ctx, &topArtist) == nil && len(topArtist) > 0 {
				topArtistData = gin.H{
					"name":  topArtist[0]["_id"],
					"plays": topArtist[0]["plays"],
				}
			}
		}

		// =================================================
		// FINAL CLEAN RESPONSE
		// =================================================
		c.JSON(http.StatusOK, gin.H{
			"range":            rangeType,
			"minutes_listened": minutesListened,
			"top_song":         topSongData,   // null OR object
			"top_artist":       topArtistData, // null OR object
		})
	}
}



