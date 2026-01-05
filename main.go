package main

import (
	"log"
	"os"

	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/database"
	"github.com/ishanbagra18/ecommerce-using-go/helpers"
	"github.com/ishanbagra18/ecommerce-using-go/routes"
	"github.com/joho/godotenv"
)

func main() {
	log.Println("🔍 [main] Starting application...")

	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("❌ [main] Error loading .env file")
	}
	log.Println("✅ [main] .env file loaded successfully")

	log.Println("🔍 [main] Initializing MongoDB...")
	database.InitDB()
	log.Println("✅ [main] MongoDB initialized successfully")

	helpers.InitUserController()
	controllers.InitUserController()
	controllers.InitMusicController()
	controllers.InitPlaylistController()
	controllers.InitHistoryController()
	controllers.InitStatsController()

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}
	log.Printf("🔍 [main] Using port: %s\n", port)

	router := gin.New()
	router.Use(gin.Logger())

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	log.Println("✅ [main] Gin router initialized")

	log.Println("🔍 [main] Registering routes...")
	routes.UserRoute(router)
	routes.AuthRoute(router)
	routes.MusicRoute(router)
	routes.PlaylistRoute(router)
	routes.HistoryRoutes(router)
	routes.StatsRoutes(router)
	log.Println("✅ [main] Routes registered")

	router.GET("/api-1", func(c *gin.Context) {
		log.Println("🔍 [main] /api-1 endpoint hit")
		c.JSON(200, gin.H{"success": "Access granted for api-1"})
	})

	router.GET("/api-2", func(c *gin.Context) {
		log.Println("🔍 [main] /api-2 endpoint hit")
		c.JSON(200, gin.H{"success": "Access granted for api-2"})
	})

	router.GET("/api-3", func(c *gin.Context) {
		log.Println("🔍 [main] /api-3 endpoint hit")
		c.JSON(200, gin.H{"success": "Access granted for api-3"})
	})

	log.Println("🚀 [main] Server running on port", port)
	router.Run(":" + port)
}
