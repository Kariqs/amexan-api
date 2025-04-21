package controllers

import (
	"log"
	"net/http"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/gin-gonic/gin"
)

func CreateProduct(ctx *gin.Context) {
	var productPayLoad models.Product
	err := ctx.ShouldBindJSON(&productPayLoad)
	if err != nil {
		log.Fatal("Failed to parse request body")
		return
	}
	result := initializers.DB.Create(&productPayLoad)
	if result.Error != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to create product"})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{"message": "product created successfully"})
}
