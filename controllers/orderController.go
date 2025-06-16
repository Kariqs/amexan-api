package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/Kariqs/amexan-api/initializers"
	"github.com/Kariqs/amexan-api/models"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
)

func GetPesapalAccessToken() (string, error) {
	client := resty.New()
	resp, err := client.R().
		SetBasicAuth(os.Getenv("PESAPAL_CONSUMER_KEY"), os.Getenv("PESAPAL_CONSUMER_SECRET")).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		Post("https://pay.pesapal.com/v3/api/Auth/RequestToken")

	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}

	// Check HTTP status code
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	// Check if response body is empty
	if len(resp.Body()) == 0 {
		return "", fmt.Errorf("empty response body")
	}

	var data map[string]any
	if err := json.Unmarshal(resp.Body(), &data); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w, body: %s", err, string(resp.Body()))
	}

	// Check if the response contains an error
	if errorMsg, exists := data["error"]; exists {
		return "", fmt.Errorf("API error: %v", errorMsg)
	}

	// Check if token exists and is a string
	if token, ok := data["token"].(string); ok && token != "" {
		return token, nil
	}

	// Log the actual response structure for debugging
	return "", fmt.Errorf("token not found in response or is empty. Response: %+v", data)
}

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

	//Get pesapal access token
	token, err := GetPesapalAccessToken()
	if err != nil {
		log.Println(err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to authenticate with Pesapal")
		return
	}

	// 🧾 Build Pesapal Order Payload
	pesapalOrder := map[string]interface{}{
		"id":              fmt.Sprintf("ORDER-%d", order.ID),
		"currency":        "KES",
		"amount":          1, //should be order.Total
		"description":     "Payment for order #" + fmt.Sprint(order.ID),
		"callback_url":    "https://amexan.store/",
		"notification_id": os.Getenv("PESAPAL_NOTIFICATION_ID"),
		"billing_address": map[string]any{
			"email_address": order.Email,
			"phone_number":  order.Phone,
			"first_name":    order.FirstName,
			"last_name":     order.LastName,
			"city":          order.DeliveryLocation,
		},
	}

	// 📤 Submit Order to Pesapal
	client := resty.New()
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+token).
		SetHeader("Content-Type", "application/json").
		SetBody(pesapalOrder).
		Post("https://pay.pesapal.com/v3/api/Transactions/SubmitOrderRequest")

	if err != nil {
		log.Println("Pesapal error:", err)
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to submit order to Pesapal")
		return
	}

	var pesapalResp map[string]any
	json.Unmarshal(resp.Body(), &pesapalResp)

	redirectURL, ok := pesapalResp["redirect_url"].(string)
	if !ok {
		sendErrorResponse(ctx, http.StatusInternalServerError, "Failed to parse Pesapal response")
		return
	}

	sendJSONResponse(ctx, http.StatusOK, gin.H{
		"message":      "Order created. Redirect user to payment.",
		"redirect_url": redirectURL,
		"order_id":     order.ID,
	})
}

func HandlePesapalIPN(ctx *gin.Context) {
	orderTrackingId := ctx.Query("orderTrackingId")
	if orderTrackingId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Missing orderTrackingId"})
		return
	}

	token, err := GetPesapalAccessToken()
	if err != nil {
		log.Println("Token error:", err)
		ctx.Status(http.StatusInternalServerError)
		return
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Authorization", "Bearer "+token).
		Get("https://pay.pesapal.com/pesapalv3/api/Transactions/GetTransactionStatus?orderTrackingId=" + orderTrackingId)

	if err != nil {
		log.Println("Pesapal status error:", err)
		ctx.Status(http.StatusInternalServerError)
		return
	}

	var result map[string]interface{}
	json.Unmarshal(resp.Body(), &result)

	status, ok := result["payment_status"].(string)
	if !ok {
		log.Println("Invalid status from Pesapal:", resp.String())
		ctx.Status(http.StatusInternalServerError)
		return
	}

	// Update payment status
	if err := initializers.DB.Model(&models.Order{}).
		Where("pesapal_tracking_id = ?", orderTrackingId).
		Update("payment_status", status).Error; err != nil {
		log.Println("DB update error:", err)
		ctx.Status(http.StatusInternalServerError)
		return
	}

	ctx.Status(http.StatusOK)
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
