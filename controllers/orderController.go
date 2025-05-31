package controllers

import (
	"log"
	"math"
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

func GetOrders(ctx *gin.Context) {
	var orders []models.Order

	// Add pagination
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "15"))
	offset := (page - 1) * limit

	query := initializers.DB.Preload("OrderItems")

	// Add search by name if provided
	if search := ctx.Query("search"); search != "" {
		query = query.Where("ID LIKE ?", "%"+search+"%")
	}

	// Execute the query with pagination
	result := query.Limit(limit).Offset(offset).Find(&orders)
	if result.Error != nil {
		respondWithError(ctx, http.StatusInternalServerError, "Unable to fetch products", result.Error)
		return
	}

	// Get total count for pagination
	var count int64
	initializers.DB.Model(&models.Product{}).Count(&count)

	previousPage := page - 1
	currentPage := page
	nextPage := page + 1

	var hasNextPage bool
	var hasPreviousPage bool

	totalPages := math.Ceil(float64(count) / float64(limit))
	if currentPage == int(totalPages) {
		hasNextPage = false
	} else {
		hasNextPage = true
	}

	if previousPage == 0 {
		hasPreviousPage = false
	} else {
		hasPreviousPage = true
	}

	ctx.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"metadata": gin.H{
			"total":        count,
			"currentPage":  currentPage,
			"limit":        limit,
			"hasPrevPage":  hasPreviousPage,
			"hasNextPage":  hasNextPage,
			"previousPage": previousPage,
			"nextPage":     nextPage,
		},
	})
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

func GetOderById(ctx *gin.Context) {
	orderId, err := strconv.Atoi(ctx.Param("orderId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse orderId")
		return
	}

	var order models.Order
	if result := initializers.DB.Preload("OrderItems").Where("ID = ?", orderId).Find(&order); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to fetch order.")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"order": order,
	})
}
