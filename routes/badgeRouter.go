package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func BadgeRoutes(r *gin.Engine) {
	r.GET("/badges/my", middleware.Authentication(), controller.GetMyBadges())
}
