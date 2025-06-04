package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/Kariqs/amexan-api/middlewares"
	"github.com/gin-gonic/gin"
)

func OrderRoutes(server *gin.Engine) {
	server.POST("/order", middlewares.RequireAuth(), controllers.CreateOrder)
	server.GET("/order", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.GetOrders)
	server.GET("/user/:userId/orders", middlewares.RequireAuth(), controllers.GetOderByCustomerId)
	server.GET("/order/:orderId", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.GetOderById)
	server.PATCH("/order/:orderId", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.UpdateOrderStatus)
	server.DELETE("/order/:orderId", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.DeleteOrder)
	server.GET("/orders/undelivered", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.GetUndeliveredOrders)
}
