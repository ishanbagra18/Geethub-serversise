package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

// CopilotRoutes registers all AI Copilot endpoints
func CopilotRoutes(incomingRoutes *gin.Engine) {
	copilotGroup := incomingRoutes.Group("/copilot")
	{
		// Query copilot with natural language prompt
		copilotGroup.POST("/query", middleware.OptionalAuthentication(), controllers.HandleCopilotQuery())

		// Execute controlled copilot actions (e.g. create playlist, queue tracks)
		copilotGroup.POST("/action", middleware.OptionalAuthentication(), controllers.HandleCopilotAction())
	}
}
