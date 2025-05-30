package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func OrderRoutes(server *gin.Engine) {
	server.POST("/order", controllers.CreateOrder)
	server.GET("/order/:userId", controllers.GetOderByCustomerId)
}
