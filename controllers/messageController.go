package controllers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/helpers"
	"github.com/ishanbagra18/ecommerce-using-go/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Helper function to create time pointer
func ptrTime(t time.Time) *time.Time {
	return &t
}

// Helper function to create string pointer
func ptrString(s string) *string {
	return &s
}

func SendMessage() gin.HandlerFunc {

	return func(c *gin.Context) {

		var message models.Message

		// Get sender_id from authenticated user
		senderID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Get receiver_id from URL params
		receiverID := c.Param("receiver_id")
		if receiverID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "receiver_id is required"})
			return
		}

		// Get message text from form data
		messageText := c.PostForm("message_text")

		// Get photo_url from form data (if sent as string)
		photoURLString := c.PostForm("photo_url")

		if messageText == "" && c.Request.MultipartForm == nil && photoURLString == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Either message_text, photo_url, or photo file is required"})
			return
		}

		// Set basic message fields
		message.ID = primitive.NewObjectID()
		message.SenderID = ptrString(senderID.(string))
		message.ReceiverID = ptrString(receiverID)
		message.Timestamp = ptrTime(time.Now())

		// Set message text if provided
		if messageText != "" {
			message.MessageText = ptrString(messageText)
		}

		// Handle photo_url if provided as a string (not a file upload)
		if photoURLString != "" {
			message.PhotoURL = ptrString(photoURLString)
			log.Println("✅ [SendMessage] Photo URL received:", photoURLString)
		} else {
			// Handle image upload if provided as a file
			file, fileHeader, err := c.Request.FormFile("photo")
			if err == nil && file != nil {
				defer file.Close()

				// Upload to Cloudinary
				photoURL, uploadErr := helpers.UploadFile(file, fileHeader, "chat_images")
				if uploadErr != nil {
					log.Println("❌ [SendMessage] Error uploading photo:", uploadErr)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload photo"})
					return
				}
				message.PhotoURL = ptrString(photoURL)
				log.Println("✅ [SendMessage] Photo uploaded successfully:", photoURL)
			}
		}

		// Insert message into messages collection
		messageCollection := database.GetCollection("ecommerce", "messages")
		_, err := messageCollection.InsertOne(context.TODO(), message)

		if err != nil {
			log.Println("❌ [SendMessage] Error inserting message:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
			return
		}

		// Find or create conversation between sender and receiver
		conversationCollection := database.GetCollection("ecommerce", "conversations")

		// Create participant IDs array (sorted for consistent lookup)
		participantIDs := []string{senderID.(string), receiverID}

		// Try to find existing conversation
		filter := bson.M{
			"participant_ids": bson.M{"$all": participantIDs},
		}

		var conversation models.Conversation
		err = conversationCollection.FindOne(context.TODO(), filter).Decode(&conversation)

		if err == mongo.ErrNoDocuments {
			// Create new conversation
			conversation = models.Conversation{
				ID:             primitive.NewObjectID(),
				ParticipantIDs: participantIDs,
				Messages:       []primitive.ObjectID{message.ID},
				CreatedAt:      ptrTime(time.Now()),
				UpdatedAt:      ptrTime(time.Now()),
			}

			_, err = conversationCollection.InsertOne(context.TODO(), conversation)
			if err != nil {
				log.Println("❌ [SendMessage] Error creating conversation:", err)
				// Message was sent but conversation wasn't created - still return success
			} else {
				log.Println("✅ [SendMessage] New conversation created:", conversation.ID.Hex())
			}
		} else if err != nil {
			log.Println("❌ [SendMessage] Error finding conversation:", err)
		} else {
			// Update existing conversation with new message
			update := bson.M{
				"$push": bson.M{"messages": message.ID},
				"$set":  bson.M{"updated_at": ptrTime(time.Now())},
			}

			_, err = conversationCollection.UpdateOne(context.TODO(), filter, update)
			if err != nil {
				log.Println("❌ [SendMessage] Error updating conversation:", err)
			} else {
				log.Println("✅ [SendMessage] Conversation updated with new message")
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Message sent successfully",
			"data":    message,
		})
	}
}

func GetMessagesBetweenUsers() gin.HandlerFunc {

	return func(c *gin.Context) {
		// Get sender_id from authenticated user
		senderID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}

		// Get receiver_id from URL params
		receiverID := c.Param("receiver_id")
		log.Println("🔍 [GetMessagesBetweenUsers] Sender ID:", senderID.(string))
		log.Println("🔍 [GetMessagesBetweenUsers] Receiver ID:", receiverID)

		if receiverID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "receiver_id is required"})
			return
		}

		// Find conversation between sender and receiver
		conversationCollection := database.GetCollection("ecommerce", "conversations")

		participantIDs := []string{senderID.(string), receiverID}

		filter := bson.M{
			"participant_ids": bson.M{"$all": participantIDs},
		}

		var conversation models.Conversation
		err := conversationCollection.FindOne(context.TODO(), filter).Decode(&conversation)

		if err == mongo.ErrNoDocuments {
			// No conversation found, return empty messages
			c.JSON(http.StatusOK, gin.H{
				"conversation_id": nil,
				"messages":        []models.Message{},
			})
			return
		} else if err != nil {
			log.Println("❌ [GetMessagesBetweenUsers] Error finding conversation:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve conversation"})
			return
		}

		// Get all messages from the conversation using message IDs
		messageCollection := database.GetCollection("ecommerce", "messages")

		if len(conversation.Messages) == 0 {
			// Conversation exists but no messages yet
			c.JSON(http.StatusOK, gin.H{
				"conversation_id": conversation.ID.Hex(),
				"messages":        []models.Message{},
			})
			return
		}

		// Fetch messages by their IDs
		messageFilter := bson.M{
			"_id": bson.M{"$in": conversation.Messages},
		}

		cursor, err := messageCollection.Find(context.TODO(), messageFilter)
		if err != nil {
			log.Println("❌ [GetMessagesBetweenUsers] Error finding messages:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve messages"})
			return
		}
		defer cursor.Close(context.TODO())

		var messages []models.Message
		if err = cursor.All(context.TODO(), &messages); err != nil {
			log.Println("❌ [GetMessagesBetweenUsers] Error decoding messages:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode messages"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"conversation_id": conversation.ID.Hex(),
			"messages":        messages,
		})
	}

}

func DeleteMessage() gin.HandlerFunc {

	return func(c *gin.Context) {
		messageID := c.Param("message_id")
		log.Println("🔍 [DeleteMessage] Received message_id:", messageID)

		if messageID == "" {
			log.Println("❌ [DeleteMessage] message_id is empty")
			c.JSON(http.StatusBadRequest, gin.H{"error": "message_id is required"})
			return
		}

		objID, err := primitive.ObjectIDFromHex(messageID)
		if err != nil {
			log.Println("❌ [DeleteMessage] Invalid message_id format:", messageID, "Error:", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message_id"})
			return
		}

		log.Println("✅ [DeleteMessage] Valid ObjectID:", objID.Hex())

		messageCollection := database.GetCollection("ecommerce", "messages")

		log.Println("🔍 [DeleteMessage] Attempting to delete message with ID:", objID.Hex())
		res, err := messageCollection.DeleteOne(context.TODO(), bson.M{"_id": objID})

		if err != nil {
			log.Println("❌ [DeleteMessage] Error deleting message:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete message"})
			return
		}

		log.Println("✅ [DeleteMessage] Delete operation completed. DeletedCount:", res.DeletedCount)
		if res.DeletedCount == 0 {
			log.Println("⚠️ [DeleteMessage] Message not found in database")
			c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
			return
		}

		log.Println("✅ [DeleteMessage] Message deleted successfully")
		c.JSON(http.StatusOK, gin.H{
			"message": "Message deleted successfully",
		})
	}

}
