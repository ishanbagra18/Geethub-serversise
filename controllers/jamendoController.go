package controllers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/models"
	"github.com/ishanbagra18/ecommerce-using-go/services"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SearchMusic handles GET /api/music/search?q=...&limit=...&source=...
// Retrieves songs from Jamendo API and merges with MongoDB local catalog
func SearchMusic() gin.HandlerFunc {
	return func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		source := strings.ToLower(strings.TrimSpace(c.Query("source"))) // "all", "jamendo", "catalog"
		limitStr := c.DefaultQuery("limit", "20")
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 50 {
			limit = 20
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var jamendoSongs []models.Song
		var catalogSongs []models.Song

		// 1. Fetch Jamendo Tracks if source is "all" or "jamendo"
		if source != "catalog" {
			jTracks, err := services.SearchJamendoTracks(ctx, query, limit)
			if err == nil {
				jamendoSongs = jTracks
			}
		}

		// 2. Fetch Local Catalog Tracks from MongoDB if source is "all" or "catalog"
		if source != "jamendo" && songcollection != nil {
			var filter bson.M
			if query != "" {
				regexPattern := "(?i)" + query
				filter = bson.M{
					"is_private": bson.M{"$ne": true},
					"$or": []bson.M{
						{"title": bson.M{"$regex": regexPattern}},
						{"artist": bson.M{"$regex": regexPattern}},
						{"album": bson.M{"$regex": regexPattern}},
						{"genre": bson.M{"$regex": regexPattern}},
						{"info": bson.M{"$regex": regexPattern}},
					},
				}
			} else {
				filter = bson.M{"is_private": bson.M{"$ne": true}}
			}

			findOpts := options.Find().SetLimit(int64(limit)).SetSort(bson.M{"play_count": -1})
			cursor, err := songcollection.Find(ctx, filter, findOpts)
			if err == nil {
				_ = cursor.All(ctx, &catalogSongs)
			}
		}

		// 3. Merge or filter according to requested source
		merged := make([]models.Song, 0, len(catalogSongs)+len(jamendoSongs))
		merged = append(merged, catalogSongs...)
		merged = append(merged, jamendoSongs...)

		c.JSON(http.StatusOK, gin.H{
			"query":         query,
			"source":        source,
			"count":         len(merged),
			"songs":         merged,
			"catalog_count": len(catalogSongs),
			"jamendo_count": len(jamendoSongs),
		})
	}
}

// GetExternalSong handles GET /api/music/song/:id
func GetExternalSong() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Song ID required"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		song, err := services.GetJamendoTrackByID(ctx, id)
		if err != nil || song == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "External song not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"song": song})
	}
}
