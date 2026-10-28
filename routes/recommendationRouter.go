package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

// RecommendationRoutes registers all endpoints for the recommendation pipeline
func RecommendationRoutes(router *gin.Engine) {
	// Recommendations with Optional Auth (Personalized for logged-in, Cold-Start for guests)
	rec := router.Group("/recommendations")
	{
		rec.GET("", middleware.OptionalAuthentication(), controller.GetRecommendations())
		rec.GET("/feed", middleware.OptionalAuthentication(), controller.GetRecommendations())
		rec.GET("/similar/:song_id", controller.GetSimilarSongs())
		rec.GET("/evaluate", controller.EvaluateRecommendations())
		rec.POST("/interaction", middleware.OptionalAuthentication(), controller.RecordInteraction())
	}

	// Quick skip endpoint under /music/skip/:song_id
	router.POST("/music/skip/:song_id", middleware.OptionalAuthentication(), controller.SkipSong())
}
