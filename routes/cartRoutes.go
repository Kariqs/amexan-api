package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func CartRoutes(server *gin.Engine) {
	server.POST("/cart", controllers.CreateCartItem)
	server.GET("/cart/:userId", controllers.GetCart)
}
