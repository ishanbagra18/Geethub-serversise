// artist backend added here

package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var artistCollection *mongo.Collection

func InitArtistController() {
	artistCollection = database.GetCollection("ecommerce", "artists")
	log.Println("✅ [InitArtistController] Artist collection initialized")
}

// GetAllArtists retrieves all artists with pagination					//done
func GetAllArtists() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var artists []models.Artist
		cursor, err := artistCollection.Find(ctx, bson.M{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch artists"})
			return
		}
		defer cursor.Close(ctx)

		if err = cursor.All(ctx, &artists); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode artists"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"artists": artists})
	}
}

// GetArtistByID retrieves a single artist by ID						//done
func GetArtistByID() gin.HandlerFunc {
	return func(c *gin.Context) {
		artistID := c.Param("artist_id")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var artist models.Artist
		err := artistCollection.FindOne(ctx, bson.M{"artist_id": artistID}).Decode(&artist)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Artist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch artist"})
			return
		}

		c.JSON(http.StatusOK, artist)
	}
}

// CreateArtist creates a new artist (Admin only)						//done
func CreateArtist() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var artist models.Artist

		if err := c.ShouldBindJSON(&artist); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// validation
		if err := validate.Struct(artist); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		artistCollection := database.GetCollection("ecommerce", "artists")

		artist.ID = primitive.NewObjectID()
		artist.Artist_id = artist.ID.Hex()
		artist.FollowerCount = 0
		artist.Followers = []string{}

		now := time.Now()
		artist.Created_at = &now
		artist.Updated_at = &now

		_, err := artistCollection.InsertOne(ctx, artist)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Artist created successfully",
			"artist":  artist,
		})
	}
}

// UpdateArtist updates artist information (Admin only)
func UpdateArtist() gin.HandlerFunc {
	return func(c *gin.Context) {
		artistID := c.Param("artist_id")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// First, check if artist exists
		var existingArtist models.Artist
		err := artistCollection.FindOne(ctx, bson.M{"artist_id": artistID}).Decode(&existingArtist)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Artist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch artist"})
			return
		}

		// Bind the update data to Artist struct
		var updateArtist models.Artist
		if err := c.BindJSON(&updateArtist); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Build update document with only provided fields
		updateData := bson.M{}

		if updateArtist.Name != nil {
			updateData["name"] = updateArtist.Name
		}

		if updateArtist.Bio != nil {
			updateData["bio"] = updateArtist.Bio
		}

		if updateArtist.Genre != nil && len(updateArtist.Genre) > 0 {
			updateData["genre"] = updateArtist.Genre
		}

		if updateArtist.ImageURL != nil {
			updateData["image_url"] = updateArtist.ImageURL
		}

		if updateArtist.SocialLinks != nil {
			updateData["social_links"] = updateArtist.SocialLinks
		}

		// Verified field can be updated
		updateData["verified"] = updateArtist.Verified

		// Always update the timestamp
		now := time.Now()
		updateData["updated_at"] = now

		// Perform the update
		update := bson.M{"$set": updateData}
		result, err := artistCollection.UpdateOne(ctx, bson.M{"artist_id": artistID}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update artist"})
			return
		}

		if result.ModifiedCount == 0 {
			c.JSON(http.StatusOK, gin.H{"message": "No changes made to artist"})
			return
		}

		// Fetch and return the updated artist
		var updatedArtist models.Artist
		err = artistCollection.FindOne(ctx, bson.M{"artist_id": artistID}).Decode(&updatedArtist)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"message": "Artist updated successfully"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Artist updated successfully",
			"artist":  updatedArtist,
		})
	}
}

// DeleteArtist deletes an artist (Admin only)   						//done
func DeleteArtist() gin.HandlerFunc {
	return func(c *gin.Context) {
		artistID := c.Param("artist_id")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := artistCollection.DeleteOne(ctx, bson.M{"artist_id": artistID})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete artist"})
			return
		}

		if result.DeletedCount == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Artist not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Artist deleted successfully"})
	}
}

// FollowArtist allows a user to follow an artist						//done
func FollowArtist() gin.HandlerFunc {
	return func(c *gin.Context) {

		// ✅ CORRECT way to read user_id from context
		uid, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			return
		}

		// ✅ Convert to string safely (works for ObjectID or string)
		userID := fmt.Sprint(uid)

		artistID := c.Param("artist_id")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// 1️⃣ Check if artist exists
		var artist models.Artist
		err := artistCollection.FindOne(
			ctx,
			bson.M{"artist_id": artistID},
		).Decode(&artist)

		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{
					"error": "Artist not found",
				})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to find artist",
			})
			return
		}

		// 2️⃣ Check if already following
		for _, followerID := range artist.Followers {
			if followerID == userID {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "Already following this artist",
				})
				return
			}
		}

		// 3️⃣ Update artist followers
		artistUpdate := bson.M{
			"$push": bson.M{
				"followers": userID,
			},
			"$inc": bson.M{
				"follower_count": 1,
			},
			"$set": bson.M{
				"updated_at": time.Now(),
			},
		}

		_, err = artistCollection.UpdateOne(
			ctx,
			bson.M{"artist_id": artistID},
			artistUpdate,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to follow artist",
			})
			return
		}

		// 4️⃣ Update user's followed artists
		userCollection := database.GetCollection("ecommerce", "users")

		userUpdate := bson.M{
			"$addToSet": bson.M{
				"followed_artists": artistID,
			},
			"$set": bson.M{
				"updated_at": time.Now(),
			},
		}

		_, err = userCollection.UpdateOne(
			ctx,
			bson.M{"user_id": userID},
			userUpdate,
		)

		if err != nil {
			log.Printf("⚠️ Failed to update user's followed_artists: %v", err)
		}

		// 5️⃣ Success response
		c.JSON(http.StatusOK, gin.H{
			"message": "Successfully followed artist",
		})
	}
}

// UnfollowArtist allows a user to unfollow an artist					//done
func UnfollowArtist() gin.HandlerFunc {
	return func(c *gin.Context) {
		artistID := c.Param("artist_id")
		userID := c.GetString("user_id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Remove user from artist's followers
		update := bson.M{
			"$pull": bson.M{"followers": userID},
			"$inc":  bson.M{"follower_count": -1},
			"$set":  bson.M{"updated_at": time.Now()},
		}

		result, err := artistCollection.UpdateOne(ctx, bson.M{"artist_id": artistID}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unfollow artist"})
			return
		}

		if result.ModifiedCount == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Artist not found or not following"})
			return
		}

		// Update user's followed_artists array
		userCollection := database.GetCollection("ecommerce", "users")
		userUpdate := bson.M{
			"$pull": bson.M{"followed_artists": artistID},
			"$set":  bson.M{"updated_at": time.Now()},
		}
		_, err = userCollection.UpdateOne(ctx, bson.M{"user_id": userID}, userUpdate)
		if err != nil {
			log.Printf("⚠️ Failed to update user's followed_artists: %v", err)
		}

		c.JSON(http.StatusOK, gin.H{"message": "Successfully unfollowed artist"})
	}
}

// GetFollowedArtists retrieves all artists followed by the current user
func GetFollowedArtists() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get user's followed artists
		userCollection := database.GetCollection("ecommerce", "users")
		var followedArtistIDs []string
		var userBson bson.M
		err := userCollection.FindOne(ctx, bson.M{"user_id": userID}).Decode(&userBson)
		if err == nil {
			if followed, ok := userBson["followed_artists"].(primitive.A); ok {
				for _, id := range followed {
					if strID, ok := id.(string); ok {
						followedArtistIDs = append(followedArtistIDs, strID)
					}
				}
			}
		}

		if len(followedArtistIDs) == 0 {
			c.JSON(http.StatusOK, gin.H{"artists": []models.Artist{}})
			return
		}

		// Fetch all followed artists
		filter := bson.M{"artist_id": bson.M{"$in": followedArtistIDs}}
		cursor, err := artistCollection.Find(ctx, filter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch artists"})
			return
		}
		defer cursor.Close(ctx)

		var artists []models.Artist
		if err = cursor.All(ctx, &artists); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode artists"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"artists": artists})
	}
}

// CheckIfFollowing checks if the current user is following a specific artist
func CheckIfFollowing() gin.HandlerFunc {
	return func(c *gin.Context) {
		artistID := c.Param("artist_id")
		userID := c.GetString("user_id")

		if userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var artist models.Artist
		err := artistCollection.FindOne(ctx, bson.M{"artist_id": artistID}).Decode(&artist)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Artist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch artist"})
			return
		}

		isFollowing := false
		for _, followerID := range artist.Followers {
			if followerID == userID {
				isFollowing = true
				break
			}
		}

		c.JSON(http.StatusOK, gin.H{"is_following": isFollowing})
	}
}

// GetArtistSongs retrieves all songs by a specific artist
func GetArtistSongs() gin.HandlerFunc {
	return func(c *gin.Context) {
		artistID := c.Param("artist_id")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		artistCollection := database.GetCollection("ecommerce", "artists")

		// 🔹 Fetch artist
		var artist models.Artist
		err := artistCollection.FindOne(ctx, bson.M{
			"artist_id": artistID,
		}).Decode(&artist)

		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusNotFound, gin.H{"error": "Artist not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch artist"})
			return
		}

		if artist.Name == nil || *artist.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Artist name is empty"})
			return
		}

		artistName := *artist.Name

		// 🔹 Fetch songs where artist exists in artists array
		songCollection := database.GetCollection("ecommerce", "songs")

		filter := bson.M{
			"artists": artistName, // ✅ MongoDB auto-matches array values
		}

		opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

		cursor, err := songCollection.Find(ctx, filter, opts)
		if err != nil {
			log.Println("Error fetching songs:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch songs"})
			return
		}
		defer cursor.Close(ctx)

		var songs []models.Song
		if err := cursor.All(ctx, &songs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode songs"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"artist": artistName,
			"count":  len(songs),
			"songs":  songs,
		})
	}
}
