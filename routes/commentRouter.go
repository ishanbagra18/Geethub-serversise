package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
	"github.com/ishanbagra18/ecommerce-using-go/middleware"
)

func CommentRoutes(r *gin.Engine) {
	// Public - get comments for a song
	r.GET("/comments/:song_id", controller.GetComments())

	// Protected - add, delete, like comments
	commentGroup := r.Group("/comments")
	commentGroup.Use(middleware.Authentication())
	{
		commentGroup.POST("/:song_id", controller.AddComment())
		commentGroup.DELETE("/:comment_id", controller.DeleteComment())
		commentGroup.PATCH("/:comment_id/like", controller.LikeComment())
	}
}
