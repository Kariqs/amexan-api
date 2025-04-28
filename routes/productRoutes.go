package routes

import (
	"github.com/Kariqs/amexan-api/controllers"
	"github.com/gin-gonic/gin"
)

func ProductRoutes(server *gin.Engine) {
	server.POST("/product", controllers.CreateProduct)
	server.POST("/product-specs", controllers.CreateProductSpecs)
	server.POST("/product-images", controllers.UploadProductImages)
	server.GET("/product", controllers.GetProducts)
	server.GET("/product/:id", controllers.GetProduct)
}
