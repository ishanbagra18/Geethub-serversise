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
	"go.mongodb.org/mongo-driver/mongo/options"
)

var commentCollection *mongo.Collection

func InitCommentController() {
	commentCollection = database.OpenCollection(database.Client, "comments")
}

// AddComment creates a new comment on a song
func AddComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Println("🔹 AddComment endpoint hit")

		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		songID := c.Param("song_id")
		if songID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "song_id is required"})
			return
		}

		var input struct {
			Text string `json:"text" binding:"required,min=1,max=500"`
		}

		if err := c.BindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Fetch user details for the comment
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var user bson.M
		err := usercollection.FindOne(ctx, bson.M{"user_id": userID.(string)}).Decode(&user)
		if err != nil {
			log.Println("❌ Failed to fetch user for comment:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user info"})
			return
		}

		userName := ""
		if fn, ok := user["first_name"].(*string); ok && fn != nil {
			userName = *fn
		} else if fn, ok := user["first_name"].(string); ok {
			userName = fn
		}
		if ln, ok := user["last_name"].(*string); ok && ln != nil {
			userName += " " + *ln
		} else if ln, ok := user["last_name"].(string); ok {
			userName += " " + ln
		}

		userEmoji := ""
		if e, ok := user["emoji"].(*string); ok && e != nil {
			userEmoji = *e
		} else if e, ok := user["emoji"].(string); ok {
			userEmoji = e
		}

		comment := models.Comment{
			ID:        primitive.NewObjectID(),
			SongID:    songID,
			UserID:    userID.(string),
			UserName:  userName,
			UserEmoji: userEmoji,
			Text:      input.Text,
			Likes:     []string{},
			CreatedAt: time.Now(),
		}

		_, err = commentCollection.InsertOne(ctx, comment)
		if err != nil {
			log.Println("❌ Failed to insert comment:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add comment"})
			return
		}

		log.Printf("✅ Comment added by %s on song %s\n", userID.(string), songID)
		c.JSON(http.StatusCreated, comment)
	}
}

// GetComments returns all comments for a song
func GetComments() gin.HandlerFunc {
	return func(c *gin.Context) {
		songID := c.Param("song_id")
		if songID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "song_id is required"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		opts := options.Find().SetSort(bson.M{"created_at": -1})

		cursor, err := commentCollection.Find(ctx, bson.M{"song_id": songID}, opts)
		if err != nil {
			log.Println("❌ Failed to fetch comments:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
			return
		}
		defer cursor.Close(ctx)

		var comments []models.Comment
		if err := cursor.All(ctx, &comments); err != nil {
			log.Println("❌ Failed to decode comments:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode comments"})
			return
		}

		if comments == nil {
			comments = []models.Comment{}
		}

		c.JSON(http.StatusOK, gin.H{
			"count":    len(comments),
			"comments": comments,
		})
	}
}

// DeleteComment deletes a comment (owner only)
func DeleteComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		commentID := c.Param("comment_id")
		if commentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "comment_id is required"})
			return
		}

		objID, err := primitive.ObjectIDFromHex(commentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Only allow deleting own comments
		result, err := commentCollection.DeleteOne(ctx, bson.M{
			"_id":     objID,
			"user_id": userID.(string),
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete comment"})
			return
		}

		if result.DeletedCount == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found or not authorized"})
			return
		}

		log.Printf("✅ Comment %s deleted by user %s\n", commentID, userID.(string))
		c.JSON(http.StatusOK, gin.H{"message": "Comment deleted successfully"})
	}
}

// LikeComment toggles like on a comment
func LikeComment() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		commentID := c.Param("comment_id")
		if commentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "comment_id is required"})
			return
		}

		objID, err := primitive.ObjectIDFromHex(commentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid comment ID"})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Check if user already liked this comment
		var comment models.Comment
		err = commentCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&comment)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
			return
		}

		uid := userID.(string)
		alreadyLiked := false
		for _, likeUID := range comment.Likes {
			if likeUID == uid {
				alreadyLiked = true
				break
			}
		}

		var update bson.M
		if alreadyLiked {
			update = bson.M{"$pull": bson.M{"likes": uid}}
		} else {
			update = bson.M{"$addToSet": bson.M{"likes": uid}}
		}

		_, err = commentCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle like"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Like toggled",
			"liked":   !alreadyLiked,
		})
	}
}
