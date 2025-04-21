package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func DefaultRoutes(server *gin.Engine) {
	server.GET("/", controllers.GetHome)
}
