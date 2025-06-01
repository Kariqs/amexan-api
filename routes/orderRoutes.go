package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func OrderRoutes(server *gin.Engine) {
	server.POST("/order", controllers.CreateOrder)
	server.GET("/order", controllers.GetOrders)
	server.GET("/user/:userId/orders", controllers.GetOderByCustomerId)
	server.GET("/order/:orderId", controllers.GetOderById)
	server.PATCH("/order/:orderId", controllers.UpdateOrderStatus)
}
