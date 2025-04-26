package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func ProductRoutes(server *gin.Engine) {
	server.POST("/product", controllers.CreateProduct)
}
