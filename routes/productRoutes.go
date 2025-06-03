package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/Kariqs/amexan-api/middlewares"
	"github.com/gin-gonic/gin"
)

func ProductRoutes(server *gin.Engine) {
	server.POST("/product", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.CreateProduct)
	server.POST("/product-specs", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.CreateProductSpecs)
	server.POST("/product-images", middlewares.RequireAuth(), middlewares.RequireAdmin(), controllers.UploadProductImages)
	server.GET("/product", controllers.GetProducts)
	server.GET("/product/:id", controllers.GetProduct)
}
