package controllers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)




var historyCollection *mongo.Collection

func InitHistoryController() {
	historyCollection = database.OpenCollection(database.Client, "history")
}






// controllers/history.go

func GetMyHistory() gin.HandlerFunc {
    return func(c *gin.Context) {
        userID, exists := c.Get("user_id")
        if !exists {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            return
        }

        // Use "played_at" to match the model's BSON tag
        opts := options.Find().
            SetSort(bson.M{"played_at": -1}). 
            SetLimit(50)

        cursor, err := historyCollection.Find(
            context.Background(),
            bson.M{"user_id": userID.(string)},
            opts,
        )
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch history"})
            return
        }

        var history []models.History
        if err := cursor.All(context.Background(), &history); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Error decoding history"})
            return
        }

        c.JSON(http.StatusOK, gin.H{
            "count":   len(history),
            "history": history,
        })
    }
}







func ClearHistory() gin.HandlerFunc {
    return func(c *gin.Context) {
        userID, exists := c.Get("user_id")  
        if !exists {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            return
        }


        _, err := historyCollection.DeleteMany(
            context.Background(),
            bson.M{"user_id": userID.(string)},
        )


        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to clear history"})
            return
        }

        c.JSON(http.StatusOK, gin.H{"message": "History cleared successfully"})
    }
}

