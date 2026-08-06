package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func PartyRoutes(incomingRoutes *gin.Engine) {
	// REST endpoints
	incomingRoutes.POST("/api/party/create", middleware.Authentication(), controllers.CreatePartyRoom())
	incomingRoutes.GET("/api/party/check/:room_id", controllers.CheckPartyRoom())

	// WebSocket sync endpoint
	incomingRoutes.GET("/ws/party/:room_id", controllers.ConnectPartyRoomWebSocket())
}
