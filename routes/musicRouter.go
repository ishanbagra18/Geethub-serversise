package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func MusicRoute(router *gin.Engine) {
	// PUBLIC ROUTES (Now with Optional Auth to catch user_id for history)
	router.GET("/song/:song_id", middleware.OptionalAuthentication(), controller.GetSongByID())

	router.GET("/allsongs", controller.GetAllSongs)
	router.GET("/music/searchsong", controller.SearchSongs)
	router.GET("/music/allsongs", controller.GetAllSongs)
	router.GET("/music/topsongs", controller.MostLikedSongs())
	router.GET("/music/saved", controller.MostSavedSongs())
	router.GET("/music/trendingsongs", controller.TrendingSongs())

	// PROTECTED ROUTES
	musicGroup := router.Group("/music")
	musicGroup.Use(middleware.Authentication())
	{
		musicGroup.POST("/addsong", controller.UploadSong)
		musicGroup.GET("/mysongs", controller.MyuploadedSongs())
		musicGroup.PATCH("/like/:song_id", controller.ToggleLikeSong)
		musicGroup.PATCH("/save/:song_id", controller.ToggleSave)
		musicGroup.GET("/mylikedsongs", controller.MylikedSongs())
		musicGroup.GET("/mysavedsongs", controller.MysavedSongs())
		musicGroup.GET("/punjabisongs", controller.PunjabiSongs())
		musicGroup.GET("/hindisongs", controller.HindiSongs())
		musicGroup.GET("/latestreleased", controller.LatestRelaseSongs())
		musicGroup.GET("/mymostplayed", controller.TopSongsByUser())
	}
}
