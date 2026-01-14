package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func MessageRoutes(incomingRoutes *gin.Engine) {
	// Protected routes - authentication required
	incomingRoutes.POST("/messages/send/:receiver_id", middleware.Authentication(), controller.SendMessage())
	incomingRoutes.GET("/messages/conversation/:receiver_id", middleware.Authentication(), controller.GetMessagesBetweenUsers())
	incomingRoutes.DELETE("/messages/delete/:message_id", middleware.Authentication(), controller.DeleteMessage())
}
