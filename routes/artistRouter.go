package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func ArtistRoutes(incomingRoutes *gin.Engine) {
	// Public routes - no authentication required
	incomingRoutes.GET("/artists", controllers.GetAllArtists())
	incomingRoutes.GET("/artists/:artist_id", controllers.GetArtistByID())
	incomingRoutes.GET("/artists/:artist_id/songs", controllers.GetArtistSongs())

	// Protected routes - authentication required
	incomingRoutes.POST("/artists/follow/:artist_id", middleware.Authentication(), controllers.FollowArtist())
	incomingRoutes.POST("/artists/unfollow/:artist_id", middleware.Authentication(), controllers.UnfollowArtist())
	incomingRoutes.GET("/artists/followed/me", middleware.Authentication(), controllers.GetFollowedArtists())
	incomingRoutes.GET("/artists/check-following/:artist_id", middleware.Authentication(), controllers.CheckIfFollowing())

	// Admin only routes - for creating/updating/deleting artists
	incomingRoutes.POST("/createartists", middleware.Authentication(), controllers.CreateArtist())
	incomingRoutes.PUT("/updateartists/:artist_id", middleware.Authentication(), controllers.UpdateArtist())
	incomingRoutes.DELETE("/artists/:artist_id", middleware.Authentication(), controllers.DeleteArtist())
}
