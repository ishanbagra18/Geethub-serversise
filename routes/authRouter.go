package routes

import (
	"github.com/gin-gonic/gin"
	controller "github.com/ishanbagra18/ecommerce-using-go/controllers"
)

func AuthRoute(incomingRoutes *gin.Engine) {

	// Normal email/password routes
	incomingRoutes.POST("/login", controller.Login())
	incomingRoutes.POST("/register", controller.Signup())
	incomingRoutes.PUT("/forgotpassword", controller.ForgetPassword())
	incomingRoutes.PUT("/updateprofile/:user_id", controller.UpdateProfile())
	incomingRoutes.GET("/myprofile/:user_id", controller.MyProfile())
	incomingRoutes.POST("/logout/:user_id", controller.Logout())

}
