package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func HistoryRoutes(router *gin.Engine) {

	history := router.Group("/history")
	history.Use(middleware.Authentication())

	{
		history.GET("/my", controller.GetMyHistory());
		history.DELETE("/clear", controller.ClearHistory());
		history.GET("/lastplayed", controller.LastPlayedSong());
	}
}
