package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func OrderRoutes(server *gin.Engine) {
	server.POST("/order", controllers.CreateOrder)
	server.GET("/order", controllers.GetOrders)
	server.GET("/orders/:userId", controllers.GetOderByCustomerId)
	server.GET("/order/:orderId", controllers.GetOderById)
}
