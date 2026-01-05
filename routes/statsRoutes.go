package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware")

func StatsRoutes(r *gin.Engine) {
	r.GET("/stats/my", middleware.Authentication(), controller.GetMyListeningStats())
}
