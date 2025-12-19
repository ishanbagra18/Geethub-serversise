package controllers

import (
	"context"
	"net/http"
	"time"
	"log"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/models"	
	"github.com/ishanbagra18/ecommerce-using-go/helpers"

)

var playlistCollection *mongo.Collection

func InitPlaylistController() {
	playlistCollection = database.OpenCollection(database.Client, "playlists")
}








// -------------------- CREATE PLAYLIST --------------------
// -------------------- CREATE PLAYLIST --------------------
func CreatePlaylist() gin.HandlerFunc {
	return func(c *gin.Context) {

		log.Println("🎶 CreatePlaylist endpoint hit")

		// Parse multipart form (max 20MB)
		if err := c.Request.ParseMultipartForm(20 << 20); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid form data"})
			return
		}

		// ---------- Read form fields ----------
		name := c.PostForm("name")
		description := c.PostForm("description")
		playlistType := c.PostForm("type") // "user" or "system"

		tags := c.PostFormArray("tags")       // optional
		songIDs := c.PostFormArray("song_ids") // optional

		// ---------- Manual validation ----------
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Playlist name is required"})
			return
		}

		if description == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Playlist description is required"})
			return
		}

		if playlistType != string(models.PlaylistTypeUser) &&
			playlistType != string(models.PlaylistTypeSystem) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist type"})
			return
		}

		// ---------- Auth handling ----------
		var creatorID *string

		userIDInterface, exists := c.Get("user_id")

		if playlistType == string(models.PlaylistTypeUser) {
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				return
			}
			uid := userIDInterface.(string)
			creatorID = &uid
		}

		// ---------- Optional cover image upload ----------
		var coverImageURL *string

		imageFile, imageHeader, err := c.Request.FormFile("cover_image")
		if err == nil {
			defer imageFile.Close()

			imgURL, err := helpers.UploadFile(imageFile, imageHeader, "playlist_images")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload cover image"})
				return
			}
			coverImageURL = &imgURL
		}

		// ---------- Create playlist object ----------
		now := time.Now()

		playlist := models.Playlist{
			ID:          primitive.NewObjectID(),
			Name:        &name,
			Description: &description,
			CoverImage:  coverImageURL,
			CreatorID:   creatorID,
			Type:        models.PlaylistType(playlistType),
			Tags:        tags,
			SongIDs:     songIDs,
			IsPublic:    true,
			PlayCount:   0,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		// ---------- System vs User flags ----------
		if playlist.Type == models.PlaylistTypeSystem {
			playlist.IsSeeded = true
		} else {
			playlist.IsSeeded = false
		}

		// ---------- Save to DB ----------
		_, err = playlistCollection.InsertOne(context.Background(), playlist)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create playlist"})
			return
		}

		// ---------- Response ----------
		c.JSON(http.StatusCreated, gin.H{
			"message":  "Playlist created successfully",
			"playlist": playlist,
		})
	}
}


















// -------------------- GET ALL PUBLIC PLAYLISTS --------------------
func GetAllPlaylists() gin.HandlerFunc {
	return func(c *gin.Context) {

		filter := bson.M{
			"is_public": true,
		}

		cursor, err := playlistCollection.Find(context.TODO(), filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching playlists"})
			return
		}
		defer cursor.Close(context.TODO())

		var playlists []models.Playlist
		if err := cursor.All(context.TODO(), &playlists); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Return empty array if no playlists found
		if playlists == nil {
			playlists = []models.Playlist{}
		}

		c.JSON(http.StatusOK, gin.H{"playlists": playlists})
	}
}
































// -------------------- GET USER'S PLAYLISTS --------------------
func GetMyPlaylists() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		uid := userID.(string)
		filter := bson.M{
			"creator_id": uid,
			"type":       models.PlaylistTypeUser,
		}

		cursor, err := playlistCollection.Find(context.TODO(), filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching playlists"})
			return
		}
		defer cursor.Close(context.TODO())

		var playlists []models.Playlist
		if err := cursor.All(context.TODO(), &playlists); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Return empty array if no playlists found
		if playlists == nil {
			playlists = []models.Playlist{}
		}

		c.JSON(http.StatusOK, gin.H{
			"playlists": playlists,
			"count":     len(playlists),
		})
	}
}














// -------------------- GET PLAYLIST BY ID --------------------
func GetPlaylistByID() gin.HandlerFunc {
	return func(c *gin.Context) {
		playlistID := c.Param("id")

		objID, err := primitive.ObjectIDFromHex(playlistID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
			return
		}

		var playlist models.Playlist
		err = playlistCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&playlist)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching playlist"})
			return
		}

		// Check if playlist is public or belongs to user
		userID, exists := c.Get("user_id")
		if !playlist.IsPublic {
			if !exists || (playlist.CreatorID != nil && *playlist.CreatorID != userID.(string)) {
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"playlist": playlist})
	}
}
















// -------------------- UPDATE PLAYLIST --------------------
func UpdatePlaylist() gin.HandlerFunc {
	return func(c *gin.Context) {
		playlistID := c.Param("id")
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		objID, err := primitive.ObjectIDFromHex(playlistID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
			return
		}

		// Check if playlist exists and belongs to user
		var existingPlaylist models.Playlist
		err = playlistCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingPlaylist)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching playlist"})
			return
		}

		uid := userID.(string)
		if existingPlaylist.CreatorID == nil || *existingPlaylist.CreatorID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to update this playlist"})
			return
		}

		var updateData map[string]interface{}
		if err := c.BindJSON(&updateData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Only allow updating certain fields
		allowedFields := map[string]bool{
			"name":        true,
			"description": true,
			"cover_image": true,
			"tags":        true,
			"is_public":   true,
		}

		updateFields := bson.M{}
		for key, value := range updateData {
			if allowedFields[key] {
				updateFields[key] = value
			}
		}

		if len(updateFields) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No valid fields to update"})
			return
		}

		updateFields["updated_at"] = time.Now()

		update := bson.M{"$set": updateFields}
		_, err = playlistCollection.UpdateOne(context.TODO(), bson.M{"_id": objID}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update playlist"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Playlist updated successfully"})
	}
}



















// -------------------- DELETE PLAYLIST --------------------
func DeletePlaylist() gin.HandlerFunc {
	return func(c *gin.Context) {
		playlistID := c.Param("id")
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		objID, err := primitive.ObjectIDFromHex(playlistID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
			return
		}

		// Check if playlist exists and belongs to user
		var existingPlaylist models.Playlist
		err = playlistCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingPlaylist)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching playlist"})
			return
		}

		uid := userID.(string)
		if existingPlaylist.CreatorID == nil || *existingPlaylist.CreatorID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to delete this playlist"})
			return
		}

		_, err = playlistCollection.DeleteOne(context.TODO(), bson.M{"_id": objID})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete playlist"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Playlist deleted successfully"})
	}
}































// -------------------- ADD SONG TO PLAYLIST --------------------
func AddSongToPlaylist() gin.HandlerFunc {
	return func(c *gin.Context) {
		playlistID := c.Param("id")
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		objID, err := primitive.ObjectIDFromHex(playlistID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
			return
		}

		var requestBody struct {
			SongID string `json:"song_id" binding:"required"`
		}

		if err := c.BindJSON(&requestBody); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Song ID is required"})
			return
		}

		// Check if playlist exists and belongs to user
		var existingPlaylist models.Playlist
		err = playlistCollection.FindOne(context.TODO(), bson.M{"_id": objID}).Decode(&existingPlaylist)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching playlist"})
			return
		}

		uid := userID.(string)
		if existingPlaylist.CreatorID == nil || *existingPlaylist.CreatorID != uid {
			c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to modify this playlist"})
			return
		}

		// Check if song already exists in playlist
		for _, songID := range existingPlaylist.SongIDs {
			if songID == requestBody.SongID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Song already exists in playlist"})
				return
			}
		}

		update := bson.M{
			"$push": bson.M{"song_ids": requestBody.SongID},
			"$set":  bson.M{"updated_at": time.Now()},
		}

		_, err = playlistCollection.UpdateOne(context.TODO(), bson.M{"_id": objID}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add song to playlist"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Song added to playlist successfully"})
	}
}




























// -------------------- REMOVE SONG FROM PLAYLIST --------------------
func RemoveSongFromPlaylist() gin.HandlerFunc {
	return func(c *gin.Context) {

		// ✅ playlist id from params
		playlistID := c.Param("id")

		// ✅ song id from request body
		var body struct {
			SongID string `json:"song_id"`
		}

		if err := c.ShouldBindJSON(&body); err != nil || body.SongID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "song_id is required",
			})
			return
		}

		songID := body.SongID

		// ✅ auth user
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// ✅ validate playlist id
		objID, err := primitive.ObjectIDFromHex(playlistID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid playlist ID"})
			return
		}

		// ✅ fetch playlist
		var existingPlaylist models.Playlist
		err = playlistCollection.FindOne(
			context.TODO(),
			bson.M{"_id": objID},
		).Decode(&existingPlaylist)

		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Playlist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error fetching playlist"})
			return
		}

		// ✅ ownership check
		uid := userID.(string)
		if existingPlaylist.CreatorID == nil || *existingPlaylist.CreatorID != uid {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "You don't have permission to modify this playlist",
			})
			return
		}

		// ✅ remove song
		update := bson.M{
			"$pull": bson.M{"song_ids": songID},
			"$set":  bson.M{"updated_at": time.Now()},
		}

		result, err := playlistCollection.UpdateOne(
			context.TODO(),
			bson.M{"_id": objID},
			update,
		)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to remove song from playlist",
			})
			return
		}

		if result.ModifiedCount == 0 {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "Song not found in playlist",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Song removed from playlist successfully",
		})
	}
}
