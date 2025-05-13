package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func AuthRoutes(server *gin.Engine) {
	auth := server.Group("/auth")
	{
		auth.POST("/signup", controllers.Signup)
		auth.POST("/login", controllers.Login)
		auth.POST("/verify-email/:activationToken", controllers.ActivateAccount)
	}
}
