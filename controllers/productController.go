package controllers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/gin-gonic/gin"
)

func CreateProduct(ctx *gin.Context) {
	var product models.Product
	err := ctx.ShouldBindJSON(&product)
	if err != nil {
		fmt.Println(err)
		log.Fatal("Failed to parse request body")
		return
	}
	result := initializers.DB.Create(&product)
	if result.Error != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to create product"})
		return
	}
	ctx.JSON(http.StatusCreated, product)
}
