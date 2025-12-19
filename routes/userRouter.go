package routes

import (
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
	"github.com/gin-gonic/gin"
)

func UserRoute(incomingRoutes *gin.Engine) {
	// Protected routes (require authentication)
	userGroup := incomingRoutes.Group("/users")
	userGroup.Use(middleware.Authentication())

	userGroup.GET("", controller.GetUsers())          // GET /users
	userGroup.GET("/:user_id", controller.GetUser()) // GET /users/:user_id
}
	