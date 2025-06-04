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

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "15"))
	offset := (page - 1) * limit

	sortOrder := ctx.DefaultQuery("sort", "desc")
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	query := initializers.DB.Preload("OrderItems")

	if search := ctx.Query("search"); search != "" {
		query = query.Where("ID LIKE ?", "%"+search+"%")
	}

	query = query.Order("created_at " + sortOrder)

	result := query.Limit(limit).Offset(offset).Find(&orders)
	if result.Error != nil {
		respondWithError(ctx, http.StatusInternalServerError, "Unable to fetch orders", result.Error)
		return
	}

	var count int64
	countQuery := initializers.DB.Model(&models.Order{})
	if search := ctx.Query("search"); search != "" {
		countQuery = countQuery.Where("id LIKE ?", "%"+search+"%")
	}
	countQuery.Count(&count)

	previousPage := page - 1
	nextPage := page + 1
	totalPages := math.Ceil(float64(count) / float64(limit))

	ctx.JSON(http.StatusOK, gin.H{
		"orders": orders,
		"metadata": gin.H{
			"total":        count,
			"currentPage":  page,
			"limit":        limit,
			"hasPrevPage":  previousPage > 0,
			"hasNextPage":  int(totalPages) > page,
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

	sortOrder := ctx.DefaultQuery("sort", "desc")
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	query := initializers.DB.Preload("OrderItems").Where("user_id = ?", userId)

	if search := ctx.Query("search"); search != "" {
		query = query.Where("id LIKE ?", "%"+search+"%")
	}

	var orders []models.Order
	if result := query.Order("created_at " + sortOrder).Find(&orders); result.Error != nil {
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
	if result := initializers.DB.Preload("OrderItems").Where("id = ?", orderId).Find(&order); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to fetch order.")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"order": order,
	})
}

func UpdateOrderStatus(ctx *gin.Context) {
	var orderStatusData struct {
		Status string `json:"status"`
	}
	err := ctx.ShouldBindJSON(&orderStatusData)
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse request body")
		return
	}

	orderId, err := strconv.Atoi(ctx.Param("orderId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse orderId")
		return
	}
	if result := initializers.DB.Model(&models.Order{}).Where("id = ?", orderId).Update("status", orderStatusData.Status); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to update order status")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Order status updated successfully.",
	})
}

func DeleteOrder(ctx *gin.Context) {
	orderId, err := strconv.Atoi(ctx.Param("orderId"))
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to parse order id.")
		return
	}

	if result := initializers.DB.Delete(&models.Order{}, orderId); result.Error != nil {
		log.Println(result.Error)
		sendErrorResponse(ctx, http.StatusBadRequest, "Failed to delete order.")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{"message": "Order deleted successfully."})
}

func GetUndeliveredOrders(ctx *gin.Context) {
	var count int64

	result := initializers.DB.
		Model(&models.Order{}).
		Where("status != ?", "Completed").
		Count(&count)

	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count undelivered orders"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"undeliveredOrderCount": count})
}
