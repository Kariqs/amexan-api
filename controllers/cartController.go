package controllers

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func createCart(userId int, ctx *gin.Context) {
	cart := models.Cart{UserID: userId}
	if result := initializers.DB.Create(&cart); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, msgFailedToCreateCart)
		return
	}
}

func CreateCartItem(ctx *gin.Context) {
	var cartItem models.CartItem
	if err := ctx.ShouldBindJSON(&cartItem); err != nil {
		log.Println("Bind error:", err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Invalid input")
		return
	}

	var existingCartItem models.CartItem
	err := initializers.DB.Where("product_id = ?", cartItem.ProductId).First(&existingCartItem).Error

	if err == nil {
		existingCartItem.ProductQuantity += cartItem.ProductQuantity

		if err := initializers.DB.Save(&existingCartItem).Error; err != nil {
			log.Println("Update error:", err)
			sendErrorResponse(ctx, 400, "Unable to update cart item quantity.")
			return
		}

		sendJSONResponse(ctx, http.StatusOK, gin.H{
			"message": "Cart item quantity updated",
			"id":      existingCartItem.ID,
		})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Println("Database error: ", err)
		sendErrorResponse(ctx, 500, "Unable to fetch cart item")
		return
	}

	if err := initializers.DB.Create(&cartItem).Error; err != nil {
		log.Println("Create error:", err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create cart item")
		return
	}

	sendJSONResponse(ctx, http.StatusCreated, gin.H{
		"message": cartItem.ProductName + " added to cart",
		"id":      cartItem.ID,
	})
}

func GetCart(ctx *gin.Context) {
	userId, err := strconv.Atoi(ctx.Param("userId"))
	if err != nil {
		log.Println(err)
		return
	}

	var cart models.Cart
	result := initializers.DB.
		Where("user_id = ?", userId).
		Preload("Items").
		First(&cart)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			sendErrorResponse(ctx, http.StatusNotFound, "Cart not found")
		} else {
			log.Println("Database error:", result.Error)
			sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to fetch cart")
		}
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{"cart": cart})
}
