package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func AuthRoute(router *gin.Engine) {

	// 🌍 PUBLIC ROUTES
	router.POST("/login", controller.Login())
	router.POST("/register", controller.Signup())

	// 🔐 PROTECTED ROUTES
	authGroup := router.Group("/auth")
	authGroup.Use(middleware.Authentication())
	{
		authGroup.PUT("/changepassword", controller.ChangePassword())
		authGroup.PUT("/updateprofile/:user_id", controller.UpdateProfile())
		authGroup.GET("/myprofile/:user_id", controller.MyProfile())
		authGroup.POST("/logout", controller.Logout())
	}
}
