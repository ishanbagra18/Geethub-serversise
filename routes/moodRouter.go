package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func MoodRoutes(router *gin.Engine) {
	ai := router.Group("/ai")
	ai.Use(middleware.Authentication())
	{
		ai.GET("/mood", controller.DetectUserMood())
	}
}
