package routes

import (
    "github.com/gin-gonic/gin"
    controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
    "github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func PlaylistRoute(router *gin.Engine) {
    // 🌍 Public
    router.GET("/playlists", controller.GetAllPlaylists())

    // 🔐 Protected
    playlistGroup := router.Group("/playlist")
    playlistGroup.Use(middleware.Authentication())
    {
        playlistGroup.POST("/create", controller.CreatePlaylist())
        playlistGroup.GET("/playlists", controller.GetAllPlaylists())
        playlistGroup.GET("/myplaylists", controller.GetMyPlaylists())
        playlistGroup.GET("/:id", controller.GetPlaylistByID()) // Consider renaming to /:id
        playlistGroup.DELETE("/delete/:id", controller.DeletePlaylist())
        playlistGroup.PUT("/update/:id", controller.UpdatePlaylist())
		playlistGroup.POST("/:id/addsong", controller.AddSongToPlaylist())
        
        // FIXED: Removed redundant "/playlist" prefix. 
        // Full URL: DELETE http://localhost:9000/playlist/:id/remove-song
        playlistGroup.DELETE("/:id/remove-song", controller.RemoveSongFromPlaylist())
    }
}