package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongo:

// Iske through MongoDB client create hota hai.

// Collections, DBs, operations sab isi se.

// mongo/options:

// Connection settings set karne ke liye.

// Jaise: ApplyURI(mongoURI)

var Client *mongo.Client

func InitDB() {
	mongoURI := os.Getenv("MONGODB_URL")
	if mongoURI == "" {
		log.Fatal("❌ MONGODB_URL not found in environment variables")
	}

	log.Println("🔍 [InitDB] MongoDB URI found in env")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	log.Println("🔍 [InitDB] Context with timeout created")

	// Add connection options with retry and timeout settings
	clientOptions := options.Client().
		ApplyURI(mongoURI).
		SetServerSelectionTimeout(60 * time.Second).
		SetConnectTimeout(60 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatalf("❌ [InitDB] Error connecting to MongoDB: %v", err)
	}
	log.Println("✅ [InitDB] Connected to MongoDB server (connection established)")

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatalf("❌ [InitDB] MongoDB ping failed: %v", err)
	}
	log.Println("✅ [InitDB] MongoDB ping successful")

	fmt.Println("🚀 MongoDB connected successfully")
	Client = client
}

// General method where you specify DB name
func GetCollection(dbName string, collectionName string) *mongo.Collection {
	if Client == nil {
		log.Fatal("❌ [GetCollection] MongoDB Client is not initialized. Call InitDB() first.")
	}
	log.Printf("🔍 [GetCollection] Returning collection '%s' from DB '%s'\n", collectionName, dbName)
	return Client.Database(dbName).Collection(collectionName)
}

// Shortcut always using "ecommerce" DB
func OpenCollection(client *mongo.Client, collectionName string) *mongo.Collection {
	if client == nil {
		log.Fatal("❌ [OpenCollection] MongoDB Client is not initialized. Call InitDB() first.")
	}
	log.Printf("🔍 [OpenCollection] Returning collection '%s' from DB 'ecommerce'\n", collectionName)
	return client.Database("ecommerce").Collection(collectionName)
}

// 🔹 GetCollection

// “Mujhe batao kaunsa database aur kaunsi collection chahiye — main de dunga.”

// Matlab:

// Aap custom DB ka naam doge

// Wo us DB ki collection return karega

// 🔹 OpenCollection

// “Database fix hai : ecommerce. Bas collection ka naam do.”

// Matlab:

// Jab app hamesha ek hi DB use karti ho

// Tab ye shortcut function helpful hai
