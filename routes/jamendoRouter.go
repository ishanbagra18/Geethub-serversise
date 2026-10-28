package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
)

// JamendoRoutes registers music search and external track endpoints
func JamendoRoutes(incomingRoutes *gin.Engine) {
	// GET /api/music/search?q=...&limit=...&source=...
	incomingRoutes.GET("/api/music/search", controllers.SearchMusic())

	// Backward-compatible alias GET /music/search
	incomingRoutes.GET("/music/search", controllers.SearchMusic())

	// GET /api/music/song/:id
	incomingRoutes.GET("/api/music/song/:id", controllers.GetExternalSong())
}
