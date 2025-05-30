package controllers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/gin-gonic/gin"
)

func CreateOrder(ctx *gin.Context) {
	var OrderInfo models.Order
	err := ctx.ShouldBindJSON(&OrderInfo)
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Error parsing request body")
		return
	}

	order := models.Order{
		UserID:           OrderInfo.UserID,
		FirstName:        OrderInfo.FirstName,
		LastName:         OrderInfo.LastName,
		Email:            OrderInfo.Email,
		Phone:            OrderInfo.Phone,
		DeliveryLocation: OrderInfo.DeliveryLocation,
		Total:            OrderInfo.Total,
		Status:           "Pending",
	}

	if result := initializers.DB.Create(&order); result.Error != nil {
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to  create order")
		return
	}

	for _, item := range OrderInfo.OrderItems {
		orderItem := models.OrderItem{
			OrderID:   int(order.ID),
			ProductId: item.ProductId,
			Name:      item.Name,
			Price:     item.Price,
			Quantity:  item.Quantity,
		}

		if result := initializers.DB.Create(&orderItem); result.Error != nil {
			sendErrorResponse(ctx, http.StatusBadRequest, "Failed to create order items")
			return
		}
	}

	sendJSONResponse(ctx, http.StatusCreated, gin.H{"message": "Order placed successfully."})
}

func GetOderByCustomerId(ctx *gin.Context) {
	userId, err := strconv.Atoi(ctx.Param("userId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse userId")
		return
	}

	var orders []models.Order
	if result := initializers.DB.Preload("OrderItems").Where("user_id = ?", userId).Find(&orders); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to fetch orders.")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"orders": orders,
	})
}
