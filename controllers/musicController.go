package controllers

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/helpers"
	"github.com/ishanbagra18/ecommerce-using-go/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var songcollection *mongo.Collection

func InitMusicController() {
	songcollection = database.OpenCollection(database.Client, "songs")
	log.Println("✔ Music collection initialized")
}

// controllers/music.go

func GetSongByID() gin.HandlerFunc {
	return func(c *gin.Context) {
		songID := c.Param("song_id")
		var song models.Song

		// 1️⃣ Fetch song
		err := songcollection.FindOne(
			context.Background(),
			bson.M{"song_id": songID},
		).Decode(&song)

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "song not found"})
			return
		}

		// 2️⃣ If user is logged in → history + play count logic
		userID, exists := c.Get("user_id")
		if exists {

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// 🔒 Prevent fake play count (10 seconds rule)
			recentFilter := bson.M{
				"user_id": userID.(string),
				"song_id": songID,
				"played_at": bson.M{
					"$gte": time.Now().Add(-10 * time.Second),
				},
			}

			count, _ := historyCollection.CountDocuments(ctx, recentFilter)
			if count > 0 {
				// Already played recently → don't increment
				c.JSON(http.StatusOK, gin.H{"song": song})
				return
			}

			// 3️⃣ Remove old history entry (keep latest only)
			_, _ = historyCollection.DeleteMany(ctx, bson.M{
				"user_id": userID.(string),
				"song_id": songID,
			})

			// 4️⃣ Insert new history record
			newHistory := models.History{
				ID:       primitive.NewObjectID(),
				UserID:   userID.(string),
				SongID:   songID,
				PlayedAt: time.Now(),
			}
			_, _ = historyCollection.InsertOne(ctx, newHistory)

			// 5️⃣ Increment play counts (atomic + safe)
			update := bson.M{
				"$inc": bson.M{
					"play_count":                          1,
					"user_play_counts." + userID.(string): 1,
				},
			}

			opts := options.Update().SetUpsert(true)

			_, err := songcollection.UpdateOne(
				ctx,
				bson.M{"song_id": songID},
				update,
				opts,
			)

			// Update response object (optional but good UX)
			if err == nil {
				song.PlayCount++
				if song.UserPlayCounts == nil {
					song.UserPlayCounts = make(map[string]int)
				}
				song.UserPlayCounts[userID.(string)]++
			}
		}

		// 6️⃣ Return song
		c.JSON(http.StatusOK, gin.H{"song": song})
	}
}

// UploadSong handles song upload with optional image
func UploadSong(c *gin.Context) {
	log.Println("🎵 UploadSong endpoint hit")

	c.Request.ParseMultipartForm(50 << 20)

	title := c.PostForm("title")
	artist := c.PostForm("artist")
	album := c.PostForm("album")
	genre := c.PostForm("genre")
	info := c.PostForm("info")
	language := c.PostForm("language")
	releaseDateStr := c.PostForm("release_date") // Expecting ISO8601 or yyyy-mm-dd

	// Debug: log incoming content type and form values to help troubleshooting
	contentType := c.Request.Header.Get("Content-Type")
	log.Printf("📥 Content-Type: %s | title=%q artist=%q album=%q genre=%q info=%q language=%q release_date=%q\n", contentType, title, artist, album, genre, info, language, releaseDateStr)

	safe := func(s string) *string {
		if s == "" {
			empty := ""
			return &empty
		}
		return &s
	}

	var releaseDatePtr *time.Time
	if releaseDateStr != "" {
		// Try parsing as RFC3339, then as yyyy-mm-dd
		if t, err := time.Parse(time.RFC3339, releaseDateStr); err == nil {
			releaseDatePtr = &t
		} else if t, err := time.Parse("2006-01-02", releaseDateStr); err == nil {
			releaseDatePtr = &t
		}
	}

	userIDInterface, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uploadedBy := userIDInterface.(string)

	// Upload audio
	songFile, songHeader, err := c.Request.FormFile("song_file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Song file is required"})
		return
	}
	defer songFile.Close()

	songURL, err := helpers.UploadFile(songFile, songHeader, "songs")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload song"})
		return
	}

	// Optional image
	var imageURL *string
	imageFile, imageHeader, err := c.Request.FormFile("image_file")
	if err == nil {
		defer imageFile.Close()
		imgURL, err := helpers.UploadFile(imageFile, imageHeader, "song_images")
		if err == nil {
			imageURL = &imgURL
		}
	}

	now := time.Now()
	newID := primitive.NewObjectID()

	log.Printf("DEBUG: songURL = %v", songURL)
	log.Printf("DEBUG: imageURL = %v", imageURL)

	song := models.Song{
		ID:          newID,
		Title:       safe(title),
		Artist:      safe(artist),
		Album:       safe(album),
		Genre:       safe(genre),
		Info:        safe(info),
		Language:    safe(language),
		FileURL:     &songURL,
		ImageURL:    imageURL,
		UploadedBy:  safe(uploadedBy),
		Likes:       []string{},
		Saves:       []string{},
		CreatedAt:   &now,
		UpdatedAt:   &now,
		SongID:      newID.Hex(),
		ReleaseDate: releaseDatePtr,

		PlayCount:      0,
		UserPlayCounts: map[string]int{},
	}

	_, err = songcollection.InsertOne(context.Background(), song)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save song"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Song uploaded successfully",
		"song_data": song,
	})
}

func GetAllSongs(c *gin.Context) {
	log.Println("🔹 GetAllSongs endpoint hit")

	if songcollection == nil {
		log.Println("❌ songcollection is nil!")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Song collection not initialized"})
		return
	}

	var songs []models.Song

	cursor, err := songcollection.Find(context.Background(), bson.M{})
	if err != nil {
		log.Println("❌ Failed to fetch songs:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch songs"})
		return
	}

	if err = cursor.All(context.Background(), &songs); err != nil {
		log.Println("❌ Failed to parse songs:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
		return
	}

	log.Printf("✅ Successfully fetched %d songs\n", len(songs))
	c.JSON(http.StatusOK, gin.H{"songs": songs})
}

func MyuploadedSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 MyuploadedSongs endpoint hit")
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		uploadedBy := userIDInterface.(string)

		var songs []models.Song
		filter := bson.M{"uploaded_by": uploadedBy}

		cursor, err := songcollection.Find(context.Background(), filter)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch songs"})
			return
		}
		if err = cursor.All(context.Background(), &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
			return
		}
		log.Printf("✅ Successfully fetched %d songs for user %s\n", len(songs), uploadedBy)
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func ToggleLikeSong(c *gin.Context) {
	log.Println("🔹 ToggleLikeSong endpoint hit")

	userId := c.GetString("user_id")
	songId := c.Param("song_id")

	var song models.Song

	err := songcollection.FindOne(context.Background(), bson.M{"song_id": songId}).Decode(&song)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch song"})
		return
	}

	alreadyLiked := false
	for _, id := range song.Likes {
		if id == userId {
			alreadyLiked = true
			break
		}
	}

	var update bson.M
	if alreadyLiked {
		update = bson.M{"$pull": bson.M{"likes": userId}}
	} else {
		update = bson.M{"$addToSet": bson.M{"likes": userId}}
	}

	_, err = songcollection.UpdateOne(context.Background(), bson.M{"song_id": songId}, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update like"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "like toggled"})
}

func ToggleSave(c *gin.Context) {
	// Get logged-in user ID from context
	userId := c.GetString("user_id")
	if userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	songID := c.Param("song_id")

	// Find the song in DB
	var song models.Song
	err := songcollection.FindOne(context.TODO(), bson.M{"song_id": songID}).Decode(&song)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Song not found"})
		return
	}

	// Check if user already saved the song
	alreadySaved := false
	for _, id := range song.Saves {
		if id == userId {
			alreadySaved = true
			break
		}
	}

	var update bson.M

	if alreadySaved {
		update = bson.M{
			"$pull": bson.M{"saves": userId},
		}
	} else {
		update = bson.M{
			"$addToSet": bson.M{"saves": userId},
		}
	}

	// Update MongoDB
	_, err = songcollection.UpdateOne(context.TODO(),
		bson.M{"song_id": songID}, update)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update save"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Save updated",
		"saved":   !alreadySaved,
	})
}

func MostLikedSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 MostLikedSongs endpoint hit")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var songs []models.Song

		findOptions := options.Find().
			SetSort(bson.D{{Key: "likes", Value: -1}}). // Sort: highest likes first
			SetLimit(10)                                // Limit: top 10

		cursor, err := songcollection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch songs"})
			return
		}
		defer cursor.Close(ctx)

		if err := cursor.All(ctx, &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse songs"})
			return
		}

		log.Printf("✅ Successfully fetched %d most liked songs\n", len(songs))
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func MostSavedSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 MostSavedSongs endpoint hit")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var songs []models.Song

		findOptions := options.Find().
			SetSort(bson.D{{Key: "saves", Value: -1}}). // Sort: highest saves first
			SetLimit(10)                                // Limit: top 10
		cursor, err := songcollection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch songs"})
			return
		}
		defer cursor.Close(ctx)

		if err := cursor.All(ctx, &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse songs"})
			return
		}
		log.Printf("✅ Successfully fetched %d most saved songs\n", len(songs))
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func SearchSongs(c *gin.Context) {
	search := strings.TrimSpace(c.Query("q"))
	searchType := strings.TrimSpace(c.Query("type")) // title | artist | genre | info

	if search == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	if searchType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'type' is required (title, artist, genre, info)"})
		return
	}

	// Allowed searchable fields
	validTypes := map[string]string{
		"title":  "title",
		"artist": "artist",
		"genre":  "genre",
		"info":   "info",
	}

	field, ok := validTypes[strings.ToLower(searchType)]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid type, use one of: title | artist | genre | info",
		})
		return
	}

	// Multi-word AND filtering
	parts := strings.Fields(search)
	andFilters := make([]bson.M, 0, len(parts))

	for _, p := range parts {
		esc := regexp.QuoteMeta(p)
		andFilters = append(andFilters, bson.M{
			field: bson.M{
				"$regex":   esc,
				"$options": "i",
			},
		})
	}

	filter := bson.M{"$and": andFilters}

	var songs []models.Song

	cursor, err := songcollection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error fetching songs",
			"details": err.Error(),
		})
		return
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &songs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Error parsing songs",
			"details": err.Error(),
		})
		return
	}

	if songs == nil {
		songs = []models.Song{}
	}

	c.JSON(http.StatusOK, gin.H{
		"songs": songs,
		"type":  searchType,
		"count": len(songs),
	})
}

func MylikedSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 MylikedSongs endpoint hit")
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		likedBy := userIDInterface.(string)

		var songs []models.Song
		filter := bson.M{"likes": likedBy}

		cursor, err := songcollection.Find(context.Background(), filter)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch songs"})
			return
		}

		if err = cursor.All(context.Background(), &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
			return
		}

		log.Printf("✅ Successfully fetched %d liked songs for user %s\n", len(songs), likedBy)
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}

}

func MysavedSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 MysavedSongs endpoint hit")
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		savedBy := userIDInterface.(string)

		var songs []models.Song
		filter := bson.M{"saves": savedBy}

		cursor, err := songcollection.Find(context.Background(), filter)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch songs"})
			return
		}

		if err = cursor.All(context.Background(), &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
			return
		}

		log.Printf("✅ Successfully fetched %d saved songs for user %s\n", len(songs), savedBy)
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func TrendingSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		var songs []models.Song

		opts := options.Find().
			SetSort(bson.D{{Key: "play_count", Value: -1}}).
			SetLimit(10)

		cursor, err := songcollection.Find(context.Background(), bson.M{}, opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trending songs"})
			return
		}

		if err := cursor.All(context.Background(), &songs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func PunjabiSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 PunjabiSongs endpoint hit")

		var songs []models.Song
		filter := bson.M{"language": "punjabi"}

		cursor, err := songcollection.Find(context.Background(), filter)

		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch songs"})
			return
		}

		if err = cursor.All(context.Background(), &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
			return
		}

		log.Printf("✅ Successfully fetched %d Punjabi songs\n", len(songs))
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func HindiSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 HindiSongs endpoint hit")

		var songs []models.Song
		filter := bson.M{"language": "hindi"}
		cursor, err := songcollection.Find(context.Background(), filter)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch songs"})
			return
		}

		if err = cursor.All(context.Background(), &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
			return
		}

		log.Printf("✅ Successfully fetched %d Hindi songs\n", len(songs))
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func LatestRelaseSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 LatestRelaseSongs endpoint hit")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var songs []models.Song

		// Sort by release_date in descending order (newest first) and limit to 10
		findOptions := options.Find().
			SetSort(bson.D{{Key: "release_date", Value: -1}}).
			SetLimit(10)

		cursor, err := songcollection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch latest songs"})
			return
		}
		defer cursor.Close(ctx)

		if err := cursor.All(ctx, &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse songs"})
			return
		}

		log.Printf("✅ Successfully fetched %d latest release songs\n", len(songs))
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func TopSongsByUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 TopSongsByUser endpoint hit")
		userIDInterface, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		userID := userIDInterface.(string)
		var songs []models.Song

		findOptions := options.Find().
			SetSort(bson.D{{Key: "user_play_counts." + userID, Value: -1}}).
			SetLimit(10)

		cursor, err := songcollection.Find(context.Background(), bson.M{"user_play_counts." + userID: bson.M{"$exists": true}}, findOptions)
		if err != nil {
			log.Println("❌ Failed to fetch songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch songs"})
			return
		}

		if err = cursor.All(context.Background(), &songs); err != nil {
			log.Println("❌ Failed to parse songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse songs"})
			return
		}

		log.Printf("✅ Successfully fetched %d top songs for user %s\n", len(songs), userID)
		c.JSON(http.StatusOK, gin.H{"songs": songs})
	}
}

func AutocompleteSearch(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))

	if query == "" {
		c.JSON(http.StatusOK, gin.H{"suggestions": []models.Song{}})
		return
	}

	log.Printf("🔍 Autocomplete search for: %q\n", query)

	// Escape special regex characters
	escapedQuery := regexp.QuoteMeta(query)

	// Search across title, artist, album, and genre fields
	filter := bson.M{
		"$or": []bson.M{
			{"title": bson.M{"$regex": escapedQuery, "$options": "i"}},
			{"artist": bson.M{"$regex": escapedQuery, "$options": "i"}},
			{"album": bson.M{"$regex": escapedQuery, "$options": "i"}},
			{"genre": bson.M{"$regex": escapedQuery, "$options": "i"}},
		},
	}

	var songs []models.Song
	findOptions := options.Find().SetLimit(10) // Limit to 10 suggestions

	cursor, err := songcollection.Find(context.Background(), filter, findOptions)
	if err != nil {
		log.Println("❌ Failed to fetch suggestions:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch suggestions"})
		return
	}
	defer cursor.Close(context.Background())

	if err := cursor.All(context.Background(), &songs); err != nil {
		log.Println("❌ Failed to parse suggestions:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse suggestions"})
		return
	}

	if songs == nil {
		songs = []models.Song{}
	}

	log.Printf("✅ Found %d suggestions\n", len(songs))
	c.JSON(http.StatusOK, gin.H{"suggestions": songs})
}
